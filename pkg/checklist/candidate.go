package checklist

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// State of a candidate on its way to becoming a check.
type State string

const (
	StatePendingRefutation State = "pending_refutation"
	StateRefuted           State = "refuted"
	StateAwaitingApproval  State = "awaiting_approval"
	StateApproved          State = "approved"
	StateRejected          State = "rejected"
)

// Label is the state as the status report prints it.
func (s State) Label() string {
	switch s {
	case StatePendingRefutation:
		return "반증 검토 대기"
	case StateRefuted:
		return "반증됨"
	case StateAwaitingApproval:
		return "승인 대기"
	case StateApproved:
		return "승인됨"
	case StateRejected:
		return "거절됨"
	}
	return string(s)
}

// Repro is the reproduction behind a single-occurrence candidate.
type Repro struct {
	Command    string `json:"command"`
	BeforeExit int    `json:"before_exit"`
	AfterExit  int    `json:"after_exit"`
}

// Refutation is one provider's verdict on a candidate.
type Refutation struct {
	Provider string `json:"provider"`
	Verdict  string `json:"verdict"` // refuted | survived | unparsed | error
	Reason   string `json:"reason"`
}

// Candidate is a proposed check with the evidence and review behind it.
type Candidate struct {
	ID          string       `json:"id"`
	State       State        `json:"state"`
	Fingerprint string       `json:"fingerprint"`
	Check       Check        `json:"check"`
	DraftedBy   string       `json:"drafted_by"`
	Lessons     []string     `json:"lessons"`
	Commits     []string     `json:"commits,omitempty"`
	Repeats     int          `json:"repeats"`
	Repro       *Repro       `json:"repro,omitempty"`
	Refutations []Refutation `json:"refutations,omitempty"`
	Created     time.Time    `json:"created"`
	Decided     *time.Time   `json:"decided,omitempty"`
}

func candidatePath(id string) string { return path.Join(CandidatesDir, id+".json") }

// LoadCandidates reads every candidate, sorted by id.
func LoadCandidates(root string) ([]Candidate, error) {
	names, err := listDir(root, CandidatesDir, ".json")
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, n := range names {
		data, err := readFile(root, path.Join(CandidatesDir, n))
		if err != nil {
			return nil, err
		}
		var c Candidate
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// LoadCandidate reads one candidate by id.
func LoadCandidate(root, id string) (Candidate, error) {
	if !ValidID(id) {
		return Candidate{}, fmt.Errorf("id %q is not CHK-NNN", id)
	}
	data, err := readFile(root, candidatePath(id))
	if err != nil {
		return Candidate{}, fmt.Errorf("candidate %s: %w", id, err)
	}
	var c Candidate
	return c, json.Unmarshal(data, &c)
}

// SaveCandidate writes c.
func SaveCandidate(root string, c Candidate) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(root, candidatePath(c.ID), append(data, '\n'))
}

// WorkingChecks reads the checks in the working tree, sorted by id. Unlike
// BaseChecks, this is what a person sees and edits, not what a run applies.
func WorkingChecks(root string) ([]Check, map[string]string, error) {
	invalid := map[string]string{}
	names, err := listDir(root, ChecksDir, ".md")
	if err != nil {
		return nil, invalid, err
	}
	var out []Check
	for _, n := range names {
		data, err := readFile(root, path.Join(ChecksDir, n))
		if err != nil {
			invalid[n] = err.Error()
			continue
		}
		c, err := ParseCheck(data)
		if err != nil {
			invalid[n] = err.Error()
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, invalid, nil
}

// nextID is one past the highest id used by any check or candidate.
func nextID(checks []Check, cands []Candidate) string {
	maxN := 0
	bump := func(id string) {
		if n, err := strconv.Atoi(strings.TrimPrefix(id, "CHK-")); err == nil && n > maxN {
			maxN = n
		}
	}
	for _, c := range checks {
		bump(c.ID)
	}
	for _, c := range cands {
		bump(c.ID)
	}
	return fmt.Sprintf("CHK-%03d", maxN+1)
}
