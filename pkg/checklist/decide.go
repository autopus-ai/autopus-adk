package checklist

import (
	"fmt"
	"path"
	"time"
)

// Approve adds an approved candidate to the checks in observe mode. It is a
// person's decision: nothing in the harness calls it on its own.
func Approve(root, id string, now time.Time) (Check, error) {
	c, err := LoadCandidate(root, id)
	if err != nil {
		return Check{}, err
	}
	if c.State != StateAwaitingApproval {
		return Check{}, fmt.Errorf("%s is %s; only a candidate in %s can be approved", id, c.State.Label(), StateAwaitingApproval.Label())
	}
	// The state is a field in a working-tree file anyone can edit, so it is
	// checked against the refutations that are supposed to have produced it.
	if !c.refutationsAllowApproval() {
		return Check{}, fmt.Errorf("%s has no surviving refutation by another provider, or one refuted it; refute it first", id)
	}
	rel := path.Join(ChecksDir, id+".md")
	if _, err := readFile(root, rel); err == nil {
		return Check{}, fmt.Errorf("%s already exists", rel)
	}
	check := c.Check
	check.Mode = ModeObserve
	data, err := check.Format()
	if err != nil {
		return Check{}, err
	}
	if err := writeFile(root, rel, data); err != nil {
		return Check{}, err
	}
	c.State, c.Decided = StateApproved, &now
	return check, SaveCandidate(root, c)
}

// Reject closes a candidate without changing any check.
func Reject(root, id string, now time.Time) error {
	c, err := LoadCandidate(root, id)
	if err != nil {
		return err
	}
	if c.State == StateApproved || c.State == StateRejected {
		return fmt.Errorf("%s is already %s", id, c.State.Label())
	}
	c.State, c.Decided = StateRejected, &now
	return SaveCandidate(root, c)
}

// SetMode changes an existing check's mode: enforce is a promotion and
// retired a retirement. Both are a person's decision.
func SetMode(root, id string, mode Mode) (Check, error) {
	if !ValidID(id) {
		return Check{}, fmt.Errorf("id %q is not CHK-NNN", id)
	}
	rel := path.Join(ChecksDir, id+".md")
	data, err := readFile(root, rel)
	if err != nil {
		return Check{}, fmt.Errorf("check %s: %w", id, err)
	}
	check, err := ParseCheck(data)
	if err != nil {
		return Check{}, err
	}
	if check.Mode == mode {
		return check, fmt.Errorf("%s is already %s", id, mode)
	}
	check.Mode = mode
	out, err := check.Format()
	if err != nil {
		return Check{}, err
	}
	return check, writeFile(root, rel, out)
}

// refutationsAllowApproval reports at least one survived verdict from a
// provider other than the drafter and no refuted verdict.
func (c Candidate) refutationsAllowApproval() bool {
	survived := false
	for _, r := range c.Refutations {
		switch {
		case r.Verdict == "refuted":
			return false
		case r.Verdict == "survived" && r.Provider != c.DraftedBy:
			survived = true
		}
	}
	return survived
}
