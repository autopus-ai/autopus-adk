package checklist

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// LessonStatus says why an escape lesson is or is not a candidate yet.
type LessonStatus struct {
	ID        string `json:"id"`
	Pattern   string `json:"pattern"`
	Status    string `json:"status"`
	Candidate string `json:"candidate,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// Lesson status labels.
const (
	LessonNotEligible = "후보 조건 미충족"
	LessonReproFailed = "재현 실패"
	LessonCandidate   = "후보"
)

type lessonGroup struct {
	fingerprint string
	entries     []learn.LearningEntry
	commits     []string
}

// fingerprintOf groups lessons that describe the same defect: the pattern
// with case and spacing folded, and the sorted diff conditions.
func fingerprintOf(e learn.LearningEntry) string {
	pattern := strings.Join(strings.Fields(strings.ToLower(e.Pattern)), " ")
	paths := append([]string(nil), e.Escape.WhenPaths...)
	keys := append([]string(nil), e.Escape.WhenKeywords...)
	sort.Strings(paths)
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(pattern + "\x00" + strings.Join(paths, "\x01") + "\x00" + strings.Join(keys, "\x01")))
	return hex.EncodeToString(sum[:6])
}

func groupEscapes(entries []learn.LearningEntry) []*lessonGroup {
	byFP := map[string]*lessonGroup{}
	var order []*lessonGroup
	for _, e := range entries {
		if e.Type != learn.EntryTypeEscape || e.Escape == nil {
			continue
		}
		fp := fingerprintOf(e)
		g := byFP[fp]
		if g == nil {
			g = &lessonGroup{fingerprint: fp}
			byFP[fp] = g
			order = append(order, g)
		}
		g.entries = append(g.entries, e)
		key := e.Escape.Commit
		if key == "" {
			key = e.ID
		}
		if !contains(g.commits, key) {
			g.commits = append(g.commits, key)
		}
	}
	return order
}

// repro is the group's reproduced evidence: a command that failed before
// the fix and passed after it.
func (g *lessonGroup) repro() *Repro {
	for _, e := range g.entries {
		if e.Evidence != nil && e.EvidenceKindOf() == learn.EvidenceDeterministic {
			return &Repro{Command: e.Evidence.Command, BeforeExit: *e.Evidence.BeforeExit, AfterExit: *e.Evidence.AfterExit}
		}
	}
	return nil
}

// failedRepro describes a recorded repro that did not reproduce, or "".
func (g *lessonGroup) failedRepro() string {
	for _, e := range g.entries {
		ev := e.Evidence
		if ev != nil && ev.Command != "" && e.EvidenceKindOf() == learn.EvidenceLLMOnly {
			return describeExits(ev)
		}
	}
	return ""
}

// Propose turns eligible escape lessons into observe-mode candidates. A
// group is eligible when it repeats across distinct commits or carries a
// reproduced repro; a group that already has a candidate is skipped.
func Propose(root string, entries []learn.LearningEntry, now time.Time) ([]Candidate, []LessonStatus, error) {
	cands, err := LoadCandidates(root)
	if err != nil {
		return nil, nil, err
	}
	checks, _, err := WorkingChecks(root)
	if err != nil {
		return nil, nil, err
	}
	byFP := map[string]string{}
	for _, c := range cands {
		byFP[c.Fingerprint] = c.ID
	}
	var created []Candidate
	var statuses []LessonStatus
	for _, g := range groupEscapes(entries) {
		repro := g.repro()
		id, exists := byFP[g.fingerprint]
		eligible := len(g.commits) >= 2 || repro != nil
		if eligible && !exists {
			c := draftCandidate(g, nextID(checks, append(cands, created...)), repro, now)
			if err := SaveCandidate(root, c); err != nil {
				return created, statuses, err
			}
			created = append(created, c)
			id, exists = c.ID, true
		}
		for _, e := range g.entries {
			statuses = append(statuses, lessonStatus(e, g, id, exists))
		}
	}
	return created, statuses, nil
}

func lessonStatus(e learn.LearningEntry, g *lessonGroup, id string, isCandidate bool) LessonStatus {
	s := LessonStatus{ID: e.ID, Pattern: e.Pattern}
	switch {
	case isCandidate:
		s.Status, s.Candidate = LessonCandidate, id
	case g.failedRepro() != "":
		s.Status, s.Detail = LessonReproFailed, g.failedRepro()
	default:
		s.Status = LessonNotEligible
		s.Detail = "한 번만 기록되었고 재현으로 확인되지 않았습니다"
	}
	return s
}

func draftCandidate(g *lessonGroup, id string, repro *Repro, now time.Time) Candidate {
	first := g.entries[0]
	when := When{}
	var lessons []string
	for _, e := range g.entries {
		lessons = append(lessons, e.ID)
		when.Paths = union(when.Paths, e.Escape.WhenPaths)
		when.Keywords = union(when.Keywords, e.Escape.WhenKeywords)
	}
	check := Check{
		ID: id, Mode: ModeObserve, When: when, Sources: lessons,
		Created: now.Format("2006-01-02"),
		Body:    first.Pattern + "\n\n놓친 검사: " + first.Escape.MissedBy,
	}
	if repro != nil {
		check.Verify = repro.Command
	}
	return Candidate{
		ID: id, State: StatePendingRefutation, Fingerprint: g.fingerprint, Check: check,
		DraftedBy: first.Escape.Provider, Lessons: lessons, Commits: g.commits,
		Repeats: len(g.commits), Repro: repro, Created: now.UTC(),
	}
}

func union(a, b []string) []string {
	for _, s := range b {
		if !contains(a, s) {
			a = append(a, s)
		}
	}
	return a
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
