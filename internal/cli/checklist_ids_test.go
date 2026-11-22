package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/checklist"
)

// A candidate saved with a pre-suffix id and one with a suffixed id are both
// addressable by refute, approve, reject and promote on the command line.
func TestChecklistCommands_OldAndNewIDs(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "init")
	t.Chdir(dir)

	prevP, prevA := checklistProviders, checklistAsker
	checklistProviders = func() ([]string, error) { return []string{"claude", "codex"}, nil }
	checklistAsker = func(string) checklist.Asker {
		return func(context.Context, string, string) (string, error) {
			return "VERDICT: survived\nREASON: specific", nil
		}
	}
	t.Cleanup(func() { checklistProviders, checklistAsker = prevP, prevA })

	ids := []string{"CHK-001", "CHK-002-0a1b2c", "CHK-003", "CHK-004-abcdef"}
	for i, id := range ids {
		c := checklist.Candidate{ID: id, State: checklist.StatePendingRefutation, Fingerprint: fmt.Sprintf("%012d", i),
			DraftedBy: "claude", Lessons: []string{"L-001"}, Created: time.Now(),
			Check: checklist.Check{ID: id, Mode: checklist.ModeObserve, When: checklist.When{Paths: []string{"src/**"}},
				Sources: []string{"L-001"}, Created: "2026-11-22", Body: "criterion"}}
		require.NoError(t, checklist.SaveCandidate(dir, c))
	}

	out, err := runAuto(t, "checklist", "refute", "CHK-002-0a1b2c")
	require.NoError(t, err, out)
	assert.Contains(t, out, "CHK-002-0a1b2c 승인 대기")
	out, err = runAuto(t, "checklist", "refute", "--all")
	require.NoError(t, err, out)
	for _, id := range []string{"CHK-001", "CHK-003", "CHK-004-abcdef"} {
		assert.Contains(t, out, id+" 승인 대기")
	}

	for _, id := range []string{"CHK-001", "CHK-002-0a1b2c"} {
		out, err = runAuto(t, "checklist", "approve", id)
		require.NoError(t, err, out)
		assert.Contains(t, out, id+" 승인됨")
	}
	for _, id := range []string{"CHK-003", "CHK-004-abcdef"} {
		out, err = runAuto(t, "checklist", "reject", id)
		require.NoError(t, err, out)
		assert.Contains(t, out, id+" 거절됨")
	}
	out, err = runAuto(t, "checklist", "promote", "CHK-002-0a1b2c")
	require.NoError(t, err, out)
	_, err = runAuto(t, "checklist", "approve", "CHK-002-0A1B2C")
	assert.Error(t, err, "an id outside both forms is refused")
}
