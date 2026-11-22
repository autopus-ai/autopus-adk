package checklist

import "sort"

// DuplicateDefect is one lesson fingerprint that more than one live (not
// rejected) candidate holds: two branches drafted the same defect under
// different sequence numbers. Keep is the candidate the lessons link to;
// Reject lists the other undecided candidates, and Retire the other approved
// ones, whose checks already exist and can only be retired.
type DuplicateDefect struct {
	Fingerprint string   `json:"fingerprint"`
	Keep        string   `json:"keep"`
	Reject      []string `json:"reject,omitempty"`
	Retire      []string `json:"retire,omitempty"`
}

// rank orders the candidates of one fingerprint for linking: an approved
// candidate first, then an undecided one, then a rejected one.
func rank(s State) int {
	switch s {
	case StateApproved:
		return 0
	case StateRejected:
		return 2
	}
	return 1
}

// representatives maps each fingerprint to the candidate its lessons link
// to: the best ranked, the lowest id among equals. cands is sorted by id.
func representatives(cands []Candidate) map[string]Candidate {
	out := map[string]Candidate{}
	for _, c := range cands {
		if cur, ok := out[c.Fingerprint]; !ok || rank(c.State) < rank(cur.State) {
			out[c.Fingerprint] = c
		}
	}
	return out
}

// duplicateDefects reports every fingerprint with more than one live
// candidate, sorted by fingerprint. A rejected candidate is already settled,
// so rejecting the advised one makes the report go away.
func duplicateDefects(cands []Candidate, reps map[string]Candidate) []DuplicateDefect {
	live := map[string][]Candidate{}
	for _, c := range cands {
		if c.State != StateRejected {
			live[c.Fingerprint] = append(live[c.Fingerprint], c)
		}
	}
	var out []DuplicateDefect
	for fp, cs := range live {
		if len(cs) < 2 {
			continue
		}
		d := DuplicateDefect{Fingerprint: fp, Keep: reps[fp].ID}
		for _, c := range cs {
			switch {
			case c.ID == d.Keep:
			case c.State == StateApproved:
				d.Retire = append(d.Retire, c.ID)
			default:
				d.Reject = append(d.Reject, c.ID)
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out
}
