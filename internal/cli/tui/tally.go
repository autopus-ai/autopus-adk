package tui

import "io"

// Tally wraps a writer and counts the ERROR lines FAIL writes through it.
//
// A report that derives its verdict from scattered booleans can print a FAIL
// banner while every line above it says WARN, which leaves the reader with a
// failure nobody can locate. Counting what FAIL actually wrote ties the
// verdict to the lines the reader saw.
type Tally struct {
	io.Writer
	errors int
}

// NewTally returns a Tally that forwards every write to w.
func NewTally(w io.Writer) *Tally {
	return &Tally{Writer: w}
}

// Errors reports how many ERROR lines FAIL has written through the tally.
func (t *Tally) Errors() int {
	return t.errors
}

func countError(w io.Writer) {
	if t, ok := w.(*Tally); ok {
		t.errors++
	}
}
