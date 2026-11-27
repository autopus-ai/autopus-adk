package harnessfb

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/checklist"
)

// State of a lesson candidate.
type State string

const (
	StatePendingRefutation State = "pending_refutation"
	StateRefuted           State = "refuted"
	StateAwaitingApproval  State = "awaiting_approval"
	StateApproved          State = "approved"
	StateRejected          State = "rejected"
	StateRetired           State = "retired"
)

// Label is the state as reports print it.
func (s State) Label() string {
	switch s {
	case StatePendingRefutation:
		return "반증 대기"
	case StateRefuted:
		return "반증됨"
	case StateAwaitingApproval:
		return "승인 대기"
	case StateApproved:
		return "승인됨"
	case StateRejected:
		return "거절됨"
	case StateRetired:
		return "은퇴함"
	}
	return string(s)
}

// Call is one model call made for a candidate.
type Call struct {
	Provider string  `json:"provider"`
	Purpose  string  `json:"purpose"` // draft | refute
	Seconds  float64 `json:"seconds"`
	Tokens   int64   `json:"tokens"` // input plus output; -1 when the provider did not report it
}

// Candidate is a proposed harness lesson with the evidence and review
// behind it.
type Candidate struct {
	ID          string                 `json:"id"`
	State       State                  `json:"state"`
	Pattern     string                 `json:"pattern"`
	Title       string                 `json:"title"`
	Evidence    []Hit                  `json:"evidence"`
	Lesson      string                 `json:"lesson"`
	Rationale   string                 `json:"rationale"`
	Target      string                 `json:"target"`
	DraftedBy   string                 `json:"drafted_by"`
	Refutations []checklist.Refutation `json:"refutations,omitempty"`
	Calls       []Call                 `json:"calls,omitempty"`
	Created     time.Time              `json:"created"`
	Decided     *time.Time             `json:"decided,omitempty"`
}

// Cost sums the candidate's model calls: their count, seconds and tokens
// (known is false when a provider did not report tokens).
func (c Candidate) Cost() (calls int, seconds float64, tokens int64, known bool) {
	known = true
	for _, call := range c.Calls {
		seconds += call.Seconds
		if call.Tokens < 0 {
			known = false
			continue
		}
		tokens += call.Tokens
	}
	return len(c.Calls), seconds, tokens, known
}

var candidateID = regexp.MustCompile(`^HL-\d{3,}$`)

// ValidID reports a candidate id of the form HL-NNN.
func ValidID(id string) bool { return candidateID.MatchString(id) }

func candidateRel(id string) string { return path.Join(CandidatesDir, id+".json") }

// LoadCandidates reads every candidate under root, sorted by id.
func LoadCandidates(root string) ([]Candidate, error) {
	names, err := listFiles(root, CandidatesDir, ".json")
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, name := range names {
		id := strings.TrimSuffix(name, ".json")
		if !ValidID(id) {
			continue
		}
		c, err := LoadCandidate(root, id)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// LoadCandidate reads one candidate.
func LoadCandidate(root, id string) (Candidate, error) {
	if !ValidID(id) {
		return Candidate{}, fmt.Errorf("잘못된 후보 id %q: HL-NNN 형식이어야 합니다", id)
	}
	data, err := readFile(root, candidateRel(id))
	if err != nil {
		return Candidate{}, err
	}
	var c Candidate
	if err := json.Unmarshal(data, &c); err != nil {
		return Candidate{}, fmt.Errorf("%s: %w", id, err)
	}
	return c, nil
}

// SaveCandidate writes c.
func SaveCandidate(root string, c Candidate) error {
	if !ValidID(c.ID) {
		return fmt.Errorf("잘못된 후보 id %q", c.ID)
	}
	return writeJSON(root, candidateRel(c.ID), c)
}

func nextID(cands []Candidate) string {
	n := 0
	for _, c := range cands {
		var k int
		if _, err := fmt.Sscanf(c.ID, "HL-%d", &k); err == nil && k > n {
			n = k
		}
	}
	return fmt.Sprintf("HL-%03d", n+1)
}

// Unproposed are the repeated patterns no candidate exists for, in any
// state: a refuted or rejected pattern is not proposed again.
func Unproposed(patterns []Pattern, cands []Candidate) []Pattern {
	seen := map[string]bool{}
	for _, c := range cands {
		seen[c.Pattern] = true
	}
	var out []Pattern
	for _, p := range patterns {
		if p.Repeated && !seen[p.ID] {
			out = append(out, p)
		}
	}
	return out
}
