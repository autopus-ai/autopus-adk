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
