// Package intake turns learning entries into quarantined golden-task
// candidates (SPEC-HARNEVAL-002). It owns the learning fingerprint, the
// candidate and record wire formats, and the path-confined file access every
// intake-area operation goes through. Nothing here executes a repro value.
package intake

// Entry is the part of a learning entry that intake reads. The caller maps a
// learn store entry onto it, so the store format stays owned by pkg/learn.
type Entry struct {
	ID       string
	Type     string
	Pattern  string
	Files    []string
	Packages []string
	Expected string
	Actual   string
	Repro    string
}
