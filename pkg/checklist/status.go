package checklist

import (
	"time"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// Defaults of the status report. A check is a retirement candidate after
// RetireAfter applications without a flag; an observe check that flagged
// PromoteAfter times is suggested for enforce.
const (
	DefaultRetireAfter  = 50
	DefaultPromoteAfter = 2
)

// CheckStatus is one check's line in the status report.
type CheckStatus struct {
	ID          string     `json:"id"`
	Mode        Mode       `json:"mode"`
	Applied     int        `json:"applied"`
	Flagged     int        `json:"flagged"`
	LastFlagged *time.Time `json:"last_flagged,omitempty"`
	Retire      bool       `json:"retire_candidate"`
	Promote     bool       `json:"promote_suggested"`
}

// Status is the whole loop at a glance.
type Status struct {
	Lessons    []LessonStatus    `json:"lessons"`
	Candidates []Candidate       `json:"candidates"`
	Checks     []CheckStatus     `json:"checks"`
	Invalid    map[string]string `json:"invalid,omitempty"`
}

// ApprovalQueue is the candidates a person can approve now.
func (s Status) ApprovalQueue() []Candidate {
	var q []Candidate
	for _, c := range s.Candidates {
		if c.State == StateAwaitingApproval {
			q = append(q, c)
		}
	}
	return q
}

// BuildStatus reads lessons, candidates, checks and hits without writing.
// retireAfter counts applications, so a rarely matched check is not retired
// early; the window is the most recent retireAfter applications.
func BuildStatus(root string, entries []learn.LearningEntry, retireAfter int) (Status, error) {
	if retireAfter <= 0 {
		retireAfter = DefaultRetireAfter
	}
	cands, err := LoadCandidates(root)
	if err != nil {
		return Status{}, err
	}
	byFP := map[string]string{}
	for _, c := range cands {
		byFP[c.Fingerprint] = c.ID
	}
	st := Status{Candidates: cands}
	for _, g := range groupEscapes(entries) {
		id, ok := byFP[g.fingerprint]
		for _, e := range g.entries {
			st.Lessons = append(st.Lessons, lessonStatus(e, g, id, ok))
		}
	}
	checks, invalid, err := WorkingChecks(root)
	if err != nil {
		return st, err
	}
	st.Invalid = invalid
	hits, err := ReadHits(root)
	if err != nil {
		return st, err
	}
	for _, c := range checks {
		st.Checks = append(st.Checks, checkStatus(c, hits, retireAfter))
	}
	return st, nil
}

func checkStatus(c Check, hits []Hit, retireAfter int) CheckStatus {
	s := CheckStatus{ID: c.ID, Mode: c.Mode}
	var mine []Hit
	for _, h := range hits {
		if h.ID == c.ID {
			mine = append(mine, h)
		}
	}
	s.Applied = len(mine)
	for _, h := range mine {
		if h.Result == ResultFlagged {
			s.Flagged++
			t := h.Time
			s.LastFlagged = &t
		}
	}
	window := mine
	if len(window) > retireAfter {
		window = window[len(window)-retireAfter:]
	}
	flaggedInWindow := 0
	for _, h := range window {
		if h.Result == ResultFlagged {
			flaggedInWindow++
		}
	}
	s.Retire = c.Mode != ModeRetired && len(window) >= retireAfter && flaggedInWindow == 0
	s.Promote = c.Mode == ModeObserve && s.Flagged >= DefaultPromoteAfter
	return s
}
