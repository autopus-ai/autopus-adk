package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runStopWithin(t *testing.T, dir, session string, budget time.Duration) (stopHookOutput, time.Duration) {
	t.Helper()
	isolateStop(t)
	cmd := newVerifyStopCmd()
	payload, _ := json.Marshal(stopHookInput{SessionID: session, Cwd: dir})
	cmd.SetIn(bytes.NewReader(payload))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--budget", budget.String()})
	start := time.Now()
	require.NoError(t, cmd.Execute())
	var got stopHookOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	return got, time.Since(start)
}

// A package whose suite outlasts the budget, with nothing failing, is not a
// failing change: the stop goes through once, and the same tree is not run
// again at the next stop.
func TestVerifyStop_SuiteLongerThanTheBudgetDoesNotBlock(t *testing.T) {
	session := "budget-long"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")
	slow := "package stopgate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSlow(t *testing.T) { time.Sleep(2 * time.Minute) }\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(slow), 0o644))

	got, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Empty(t, got.Decision, got.Reason)
	assert.Contains(t, got.SystemMessage, "nothing failed within the budget")

	again, took := runStopWithin(t, dir, session, 30*time.Second)
	assert.Equal(t, stopHookOutput{}, again)
	assert.Less(t, took, 10*time.Second, "the same tree is not run again")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte("package stopgate\n\nfunc Add(a, b int) int { return a - b }\n"), 0o644))
	edited, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Equal(t, "block", edited.Decision, "an edit is verified again, and TestAdd fails inside the budget")
	assert.Contains(t, edited.Reason, "Add(1, 2) != 3")
}
