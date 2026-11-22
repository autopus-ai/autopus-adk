//go:build unix

package verifygate

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubbornScript ignores SIGTERM for itself and everything it starts, so the
// kill has to wait out the whole grace before its SIGKILL.
const stubbornScript = "trap '' TERM; "

// The window a step is judged by is the time it had, not that plus the time
// the kill took: a tree that ignores SIGTERM costs the grace, and that must
// not show up as time the step ran.
func TestRun_KillGraceIsNotCountedAsTimeTheStepHad(t *testing.T) {
	dir := t.TempDir()
	step := Step{Name: "stubborn", Dir: dir, Argv: []string{"sh", "-c", stubbornScript + "sleep 30"}}

	failure, _ := Run(context.Background(), []Step{step}, 3*time.Second)

	require.NotNil(t, failure)
	assert.True(t, failure.TimedOut)
	assert.Contains(t, failure.Reason, "within the 3s it had of the 3s budget")
}

// A test idle for less than half the window when the deadline came is not a
// suspected hang, however long the kill takes afterwards: idle time and the
// window are both measured at the deadline.
func TestRun_IdleUnderHalfTheWindowStaysABudgetSkipWhenTheKillIsSlow(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Now().UTC()
	at := func(d time.Duration, ev string) string {
		return `{"Time":"` + t0.Add(d).Format(time.RFC3339Nano) + `",` + ev[1:]
	}
	first := at(0, `{"Action":"run","Package":"x","Test":"TestA"}`)
	second := at(2500*time.Millisecond, `{"Action":"pass","Package":"x","Test":"TestA"}`) + "\n" +
		at(2500*time.Millisecond, `{"Action":"run","Package":"x","Test":"TestB"}`)
	// TestB starts 2.5s in and is 1.5s idle at the 4s deadline: under half.
	script := stubbornScript + "printf '%s\\n' '" + first + "'; sleep 2.5; printf '%s\\n' '" + second + "'; sleep 30"
	step := Step{Name: "slow", Dir: dir, GoTestJSON: true, BudgetBound: true, Argv: []string{"sh", "-c", script}}

	failure, skipped := Run(context.Background(), []Step{step}, 4*time.Second)

	require.Nil(t, failure, "a 1.5s idle test in a 4s window is not a hang: %+v", failure)
	require.Len(t, skipped, 1)
	assert.True(t, skipped[0].Budget, skipped[0].Reason)
	assert.Contains(t, skipped[0].Reason, "within the 4s it had")
}
