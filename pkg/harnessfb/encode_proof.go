package harnessfb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProofTimeout bounds each of the three check runs in a proof.
var ProofTimeout = 10 * time.Minute

// maxRepro bounds the reproduction patch.
const maxRepro = 256 << 10

// ProofStage names the run that refused a proof.
type ProofStage string

const (
	StageCurrent ProofStage = "current" // the check fails on the current tree
	StageCopy    ProofStage = "copy"    // the check fails on the snapshot copy
	StageApply   ProofStage = "apply"   // the patch does not apply
	StageSelf    ProofStage = "self"    // the patch edits a file the check command names
	StageBroken  ProofStage = "broken"  // with the patch, the check command cannot run at all
	StageCatch   ProofStage = "catch"   // the check passes with the mistake in place
)

// ProofError is a proof the harness ran and refused.
type ProofError struct {
	Stage  ProofStage
	Output string
}

func (e *ProofError) Error() string {
	switch e.Stage {
	case StageCurrent:
		return "증명 실패: 검사가 지금 트리에서 통과하지 않습니다"
	case StageCopy:
		return "증명 실패: 검사가 지금 트리의 복사본에서 통과하지 않습니다. 복사본에는 git이 무시하는 파일(node_modules, venv, 빌드 산출물), submodule 내용, 저장소 로컬 git 설정이 없습니다. 검사 명령이 필요한 것을 준비하게 하세요"
	case StageApply:
		return "증명 실패: 재현 patch를 지금 트리에 적용할 수 없습니다"
	case StageSelf:
		return "증명 실패: 재현 patch가 검사 명령이 가리키는 파일을 고칩니다. patch는 교훈의 실수만 넣어야 합니다"
	case StageBroken:
		return "증명 실패: 재현 patch를 적용하자 검사 명령 자체가 실행되지 않습니다 (exit 126/127). 실수를 잡은 것이 아닙니다"
	case StageCatch:
		return "증명 실패: 재현 patch를 적용해도 검사가 통과합니다. 이 검사는 교훈의 실수를 잡지 못합니다"
	}
	return "증명 실패"
}

// Proof is one submitted check for an approved lesson.
type Proof struct {
	ID        string
	Tree      string // the work tree to prove on; the caller's worktree, not necessarily root
	Layer     Layer
	Check     string
	ReproPath string
}

// Prove runs the check on the current tree, on a snapshot copy of it, and
// on the copy with the reproduction patch committed. Only a pass, a pass
// and a failure prove the lesson; the copy run ties the failure to the
// patch rather than to the copy. The current tree is never changed.
func Prove(ctx context.Context, root string, p Proof, now time.Time) (Encoding, []string, error) {
	c, err := ApprovedCandidate(root, p.ID)
	if err != nil {
		return Encoding{}, nil, err
	}
	if p.Layer == LayerText || p.Layer == "" {
		return Encoding{}, nil, errors.New("--layer text는 --reason과 함께 쓰고, 검사가 있는 층은 api, lint, test, hook 중 하나입니다")
	}
	if strings.TrimSpace(p.Check) == "" || p.ReproPath == "" {
		return Encoding{}, nil, errors.New("--check와 --repro가 필요합니다")
	}
	repro, err := readRepro(p.ReproPath)
	if err != nil {
		return Encoding{}, nil, err
	}
	if p.Tree == "" {
		p.Tree = root
	}
	paths, failOut, err := proveTrees(ctx, p, repro)
	if err != nil {
		return Encoding{}, nil, err
	}
	sum := sha256.Sum256(repro)
	e := Encoding{
		ID: c.ID, Digest: digest(c), Layer: p.Layer, Check: p.Check,
		Repro: string(repro), ReproSHA256: hex.EncodeToString(sum[:]), ReproPaths: paths,
		FailOutput: tail(failOut), Proved: now,
	}
	if head, err := gitIn(ctx, p.Tree, nil, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		e.Head = strings.TrimSpace(head)
	}
	if branch, err := gitIn(ctx, p.Tree, nil, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		e.Branch = strings.TrimSpace(branch)
	}
	if err := saveEncoding(root, e); err != nil {
		return e, nil, err
	}
	written, err := RenderRules(root)
	return e, written, err
}

func readRepro(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("재현 patch를 읽을 수 없습니다: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRepro+1))
	if err != nil {
		return nil, fmt.Errorf("재현 patch를 읽을 수 없습니다: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 || len(data) > maxRepro {
		return nil, fmt.Errorf("재현 patch는 비어 있지 않고 %d KiB 이하여야 합니다", maxRepro>>10)
	}
	return data, nil
}

// proveTrees runs the three checks; the copy is gone when it returns. It
// returns the paths the patch touches and the check's output on the
// patched copy.
func proveTrees(ctx context.Context, p Proof, repro []byte) ([]string, string, error) {
	if exit, out, err := runCheck(ctx, p.Tree, p.Check); err != nil && !errors.Is(err, errCannotRun) {
		return nil, "", err
	} else if exit != 0 {
		return nil, "", &ProofError{Stage: StageCurrent, Output: out}
	}
	cp, cleanup, err := snapshot(ctx, p.Tree)
	if err != nil {
		return nil, "", fmt.Errorf("지금 트리의 복사본을 만들지 못했습니다: %w", err)
	}
	defer cleanup()
	if exit, out, err := runCheck(ctx, cp.dir, p.Check); err != nil && !errors.Is(err, errCannotRun) {
		return nil, "", err
	} else if exit != 0 {
		return nil, "", &ProofError{Stage: StageCopy, Output: out}
	}
	patch := filepath.Join(cp.tmp, "repro.patch")
	if err := os.WriteFile(patch, repro, 0o600); err != nil {
		return nil, "", err
	}
	numstat, err := cp.git(ctx, "apply", "--numstat", patch)
	if err != nil {
		return nil, "", &ProofError{Stage: StageApply, Output: numstat}
	}
	paths := patchPaths(numstat)
	if hit := checkNames(p.Check, paths); hit != "" {
		return paths, "", &ProofError{Stage: StageSelf, Output: hit}
	}
	if out, err := cp.git(ctx, "apply", "--index", "--whitespace=nowarn", patch); err != nil {
		return paths, "", &ProofError{Stage: StageApply, Output: out}
	}
	// Unsigned: the user's signing key is not asked to sign a throwaway
	// commit. Committed, so a check that only asks whether the tree is clean
	// (git diff --quiet) sees the same clean state as before.
	if out, err := cp.git(ctx, "commit", "-q", "--no-verify", "--no-gpg-sign", "-m", "repro"); err != nil {
		return paths, "", fmt.Errorf("재현 patch를 복사본에 커밋하지 못했습니다: %w", errors.Join(err, errors.New(out)))
	}
	exit, failOut, err := runCheck(ctx, cp.dir, p.Check)
	switch {
	case errors.Is(err, errCannotRun):
		return paths, "", &ProofError{Stage: StageBroken, Output: failOut}
	case err != nil:
		return paths, "", err
	case exit == 0:
		return paths, "", &ProofError{Stage: StageCatch, Output: failOut}
	}
	return paths, failOut, nil
}

// patchPaths reads the file names from `git apply --numstat`.
func patchPaths(numstat string) []string {
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(numstat), "\n") {
		if f := strings.SplitN(line, "\t", 3); len(f) == 3 {
			paths = append(paths, f[2])
		}
	}
	return paths
}

// interpreters run the script named after them; wrappers run the
// command that follows them.
var (
	interpreters = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "node": true, "python": true,
		"python3": true, "ruby": true, "perl": true, "source": true, ".": true}
	wrappers = map[string]bool{"env": true, "time": true, "nice": true, "command": true, "exec": true,
		"xargs": true, "nohup": true, "!": true, "timeout": true, "stdbuf": true}
)

// executed is the program a command segment runs and, for an interpreter,
// the script it runs, with wrappers, VAR=value prefixes and options
// stripped (a bare number or duration is taken as an option's value, as in
// nice -n 5 or timeout 60s).
func executed(seg string) []string {
	words := strings.Fields(seg)
	for len(words) > 0 && (wrappers[words[0]] || strings.Contains(words[0], "=") || strings.HasPrefix(words[0], "-") ||
		strings.Trim(words[0], "0123456789.smhd") == "") {
		words = words[1:]
	}
	if len(words) == 0 {
		return nil
	}
	run := []string{words[0]}
	if interpreters[filepath.Base(words[0])] {
		for _, w := range words[1:] {
			if !strings.HasPrefix(w, "-") {
				run = append(run, w)
				break
			}
		}
	}
	return run
}

// checkNames returns a patched path the check executes, such as the lint
// script itself: editing it would make the check fail for no reason
// related to the lesson. A file the check only reads, like the source it
// greps or a config file, is not detected; the patch's paths are shown.
func checkNames(check string, paths []string) string {
	for _, seg := range strings.FieldsFunc(check, func(r rune) bool { return r == '&' || r == '|' || r == ';' || r == '\n' }) {
		for _, word := range executed(seg) {
			word = filepath.ToSlash(filepath.Clean(strings.Trim(word, `'"`)))
			word = strings.TrimPrefix(word, "./")
			for _, p := range paths {
				if word == p {
					return p
				}
			}
		}
	}
	return ""
}
