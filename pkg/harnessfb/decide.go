package harnessfb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/checklist"
)

// The refutations this machine ran and the approvals a person gave live
// inside the git directory, which no commit writes and no clone copies, so
// a candidate file cannot claim a verdict or an approval nobody gave.
const (
	refutationsGitPath = "autopus/harness-refutations"
	approvalsGitPath   = "autopus/harness-approvals"
)

// ErrNoGit means root is not in a git repository: refutations and
// approvals cannot be recorded, so no model is asked.
var ErrNoGit = errors.New("git 저장소가 아니어서 반증·승인 기록을 남길 수 없습니다")

type verdictRecord struct {
	ID          string                 `json:"id"`
	Digest      string                 `json:"digest"`
	Refutations []checklist.Refutation `json:"refutations,omitempty"`
}

// digest pins what was refuted and approved: the rule text and its drafter.
func digest(c Candidate) string {
	sum := sha256.Sum256([]byte(c.Lesson + "\x00" + c.DraftedBy))
	return hex.EncodeToString(sum[:])
}

// GitDir is root's common git directory, shared by its linked worktrees.
func GitDir(root string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", ErrNoGit
	}
	return strings.TrimSpace(string(out)), nil
}

func saveRecord(root, dir string, rec verdictRecord) error {
	git, err := GitDir(root)
	if err != nil {
		return err
	}
	return writeJSON(git, path.Join(dir, rec.ID+".json"), rec)
}

func loadRecord(root, dir string, c Candidate) (verdictRecord, error) {
	var rec verdictRecord
	git, err := GitDir(root)
	if err != nil {
		return rec, err
	}
	data, err := readFile(git, path.Join(dir, c.ID+".json"))
	if err != nil {
		return rec, err
	}
	if json.Unmarshal(data, &rec) != nil || rec.Digest != digest(c) {
		return rec, fmt.Errorf("%s의 규칙이 기록 뒤에 바뀌었습니다", c.ID)
	}
	return rec, nil
}

// SaveRefutationRecord records c's verdicts.
func SaveRefutationRecord(root string, c Candidate) error {
	return saveRecord(root, refutationsGitPath, verdictRecord{ID: c.ID, Digest: digest(c), Refutations: c.Refutations})
}

// allowsApproval reports at least one survived verdict by a provider other
// than the drafter and no refuted verdict, from this machine's record of
// the candidate as it is now.
func allowsApproval(root string, c Candidate) error {
	rec, err := loadRecord(root, refutationsGitPath, c)
	if err != nil {
		return fmt.Errorf("%s의 반증 기록이 없거나 맞지 않습니다: 먼저 `auto harness refute %s`를 실행하세요 (%v)", c.ID, c.ID, err)
	}
	survived := false
	for _, r := range rec.Refutations {
		switch {
		case r.Verdict == "refuted":
			return fmt.Errorf("%s는 %s가 반증했습니다", c.ID, r.Provider)
		case r.Verdict == "survived" && r.Provider != c.DraftedBy:
			survived = true
		}
	}
	if !survived {
		return fmt.Errorf("%s는 다른 provider의 반증을 통과하지 않았습니다", c.ID)
	}
	return nil
}

// Approved are the candidates a person approved on this machine, checked
// against the approval record: a file that only says "approved" is not.
func Approved(root string, cands []Candidate) []Candidate {
	var out []Candidate
	for _, c := range cands {
		if c.State != StateApproved {
			continue
		}
		if _, err := loadRecord(root, approvalsGitPath, c); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// Approve is a person's decision: the lesson is recorded as approved and
// written into the rule file of every worktree. Nothing in the harness
// calls it on its own. It returns the rule files written.
func Approve(root, id string, now time.Time) (Candidate, []string, error) {
	c, err := LoadCandidate(root, id)
	if err != nil {
		return c, nil, err
	}
	if c.State != StateAwaitingApproval {
		return c, nil, fmt.Errorf("%s는 %s 상태입니다: %s 후보만 승인합니다", id, c.State.Label(), StateAwaitingApproval.Label())
	}
	if err := allowsApproval(root, c); err != nil {
		return c, nil, err
	}
	if err := saveRecord(root, approvalsGitPath, verdictRecord{ID: c.ID, Digest: digest(c)}); err != nil {
		return c, nil, err
	}
	c.State, c.Decided = StateApproved, &now
	if err := SaveCandidate(root, c); err != nil {
		return c, nil, err
	}
	written, err := RenderRules(root)
	return c, written, err
}

// Reject closes a candidate; no rule file is touched.
func Reject(root, id string, now time.Time) (Candidate, error) {
	return decide(root, id, now, StateRejected, StatePendingRefutation, StateAwaitingApproval)
}

// Retire takes an approved lesson back out of the rule files.
func Retire(root, id string, now time.Time) (Candidate, []string, error) {
	c, err := decide(root, id, now, StateRetired, StateApproved)
	if err != nil {
		return c, nil, err
	}
	if git, err := GitDir(root); err == nil {
		_ = removeFile(git, path.Join(approvalsGitPath, id+".json"))
	}
	removeEncoding(root, id)
	written, err := RenderRules(root)
	return c, written, err
}

func decide(root, id string, now time.Time, to State, from ...State) (Candidate, error) {
	c, err := LoadCandidate(root, id)
	if err != nil {
		return c, err
	}
	ok := false
	for _, s := range from {
		ok = ok || c.State == s
	}
	if !ok {
		return c, fmt.Errorf("%s는 %s 상태라 %s으로 바꿀 수 없습니다", id, c.State.Label(), to.Label())
	}
	c.State, c.Decided = to, &now
	return c, SaveCandidate(root, c)
}

const ruleHeader = `---
name: harness-lessons
description: Rules approved from this project's own session measurements (auto harness)
---

# Harness lessons

` + "`auto harness`" + ` writes this file from approved lessons; change them with ` + "`auto harness approve|retire`" + `, not here.
Each rule came from waste measured in several sessions of this project, survived another provider's
refutation, and was approved by a person. A lesson leaves this file once ` + "`auto harness encode`" + ` has
proven a check in the repository that enforces it.

`

// rules is the rule file's content, or "" when nothing is approved.
func rules(root string) (string, error) {
	cands, err := LoadCandidates(root)
	if err != nil {
		return "", err
	}
	approved := Approved(root, cands)
	if len(approved) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString(ruleHeader)
	kept := 0
	for _, c := range approved {
		note := "temporary until a check enforces it"
		if e, ok := LoadEncoding(root, c); ok {
			if e.Structural() {
				continue // a proven check enforces it; the text would only repeat it
			}
			note = "no check fits: " + e.Reason
		}
		kept++
		fmt.Fprintf(&b, "- %s (%s, %d sessions; %s): %s\n", c.ID, c.Pattern, len(c.Evidence), note, strings.Join(strings.Fields(c.Lesson), " "))
	}
	if kept == 0 {
		return "", nil
	}
	return b.String(), nil
}

// RenderRules writes the approved lessons into RuleFile in every worktree
// of root's repository (the rule file is usually gitignored, so a worktree
// only has it when it is written there), or removes it when none is
// approved. It returns the absolute paths written or removed.
func RenderRules(root string) ([]string, error) {
	content, err := rules(root)
	if err != nil {
		return nil, err
	}
	var done []string
	var errs []error
	for _, wt := range worktrees(root) {
		if _, err := os.Stat(wt); err != nil {
			continue // a worktree git still lists but whose directory is gone
		}
		if err := syncRule(wt, content); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", wt, err))
			continue
		}
		done = append(done, filepath.Join(wt, filepath.FromSlash(RuleFile)))
	}
	return done, errors.Join(errs...)
}

// SyncRules brings one worktree's rule file in line with the approved
// lessons, for a worktree made after the last approval.
func SyncRules(root, worktree string) error {
	content, err := rules(root)
	if err != nil {
		return err
	}
	return syncRule(worktree, content)
}

// ownRule reports whether the worktree's rule file is one this loop may
// change: absent, or untracked and written by it. A file git tracks (a
// team committed it) or one a person wrote is left alone, since approvals
// are recorded per machine.
func ownRule(worktree string) bool {
	data, err := readFile(worktree, RuleFile)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	if !strings.HasPrefix(string(data), ruleHeader[:strings.Index(ruleHeader, "description")]) {
		return false
	}
	err = exec.CommandContext(context.Background(), "git", "-C", worktree, "ls-files", "--error-unmatch", "--", RuleFile).Run()
	return err != nil
}

func syncRule(worktree, content string) error {
	if !ownRule(worktree) {
		return nil
	}
	if content == "" {
		return removeFile(worktree, RuleFile)
	}
	if old, err := readFile(worktree, RuleFile); err == nil && string(old) == content {
		return nil
	}
	return writeFile(worktree, RuleFile, []byte(content))
}

// worktrees lists the work trees of root's repository; root alone when git
// cannot say.
func worktrees(root string) []string {
	out, err := exec.CommandContext(context.Background(), "git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return []string{root}
	}
	var list []string
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			list = append(list, p)
		}
	}
	if len(list) == 0 {
		return []string{root}
	}
	return list
}
