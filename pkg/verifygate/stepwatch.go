package verifygate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"time"
)

// earlyCutAt is the fraction of a step's window after which a whole-package
// Go test step that has already reported a failed test is stopped. The
// failure is reported either way; ending the step early leaves the rest of
// the budget to rerun the failed tests on HEAD (see RunCharged), which is
// what tells a failure the change caused from one the tree already had.
const earlyCutAt = 2.0 / 3.0

// earlyCutPoll is how often the output is read again after earlyCutAt.
const earlyCutPoll = 2 * time.Second

// stepContext is a step's deadline that can be brought forward: once cut,
// it reports the cut as its deadline and DeadlineExceeded as its error, so
// Run judges the step exactly as one that ran out of its window.
type stepContext struct {
	context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	cutAt  time.Time
}

func newStepContext(parent context.Context, window time.Duration) (*stepContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, window)
	return &stepContext{Context: ctx, cancel: cancel}, cancel
}

func (c *stepContext) Deadline() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cutAt.IsZero() {
		return c.cutAt, true
	}
	return c.Context.Deadline()
}

func (c *stepContext) Err() error {
	c.mu.Lock()
	cut := !c.cutAt.IsZero()
	c.mu.Unlock()
	if cut {
		return context.DeadlineExceeded
	}
	return c.Context.Err()
}

// cut ends the step now, unless its context already ended.
func (c *stepContext) cut() {
	c.mu.Lock()
	if c.cutAt.IsZero() && c.Context.Err() == nil {
		c.cutAt = time.Now()
	}
	c.mu.Unlock()
	c.cancel()
}

// wasCut reports whether cut ended the step before its window did.
func (c *stepContext) wasCut() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.cutAt.IsZero()
}

// lockedBuffer is a bytes.Buffer the step's output can be read from while
// the step still writes it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *lockedBuffer) String() string { return string(b.Bytes()) }

// watchFailures cuts a whole-package Go test step short once earlyCutAt of
// its window has passed and a test has failed. Only BudgetBound go test
// -json steps that no later step covers are watched: a suite that finishes
// on its own is never cut, and a step with no failed test runs its full
// window. The watcher ends with the step's context. A cut that lands in the
// instant between the step exiting and Run reading the context only makes
// a finished failing step look cut off, which RunCharged treats as partial.
func watchFailures(ctx *stepContext, step Step, out *lockedBuffer, window time.Duration) {
	if !step.GoTestJSON || !step.BudgetBound || step.CoveredLater {
		return
	}
	go func() {
		timer := time.NewTimer(time.Duration(float64(window) * earlyCutAt))
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if _, failed := renderGoTestJSON(out.Bytes()); hasTestFailure(unknownFailures(failed, step.KnownFailing)) {
				ctx.cut()
				return
			}
			timer.Reset(earlyCutPoll)
		}
	}()
}

// withoutLatePackageFails drops, from a step cut off at its deadline, the
// package-level failures go reported after the deadline: those are the
// test binaries the kill took down, not packages that failed on their own.
func withoutLatePackageFails(failed []string, out []byte, deadline time.Time, cut bool) []string {
	if !cut {
		return failed
	}
	late := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev goTestEventTimed
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil {
			continue
		}
		if ev.Action == "fail" && ev.Test == "" && ev.Package != "" && ev.Time.After(deadline) {
			late[ev.Package] = true
		}
	}
	kept := make([]string, 0, len(failed))
	for _, key := range failed {
		if !late[key] {
			kept = append(kept, key)
		}
	}
	return kept
}

// unknownFailures drops the keys HEAD is known to fail.
func unknownFailures(failed []string, known map[string]bool) []string {
	if len(known) == 0 {
		return failed
	}
	var out []string
	for _, key := range failed {
		if !known[key] {
			out = append(out, key)
		}
	}
	return out
}

// hasTestFailure reports a failed test, not only a failed package: a
// package-level fail event can be the kill itself.
func hasTestFailure(failed []string) bool {
	for _, key := range failed {
		if _, _, ok := splitTestKey(key); ok {
			return true
		}
	}
	return false
}
