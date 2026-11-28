package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/debt"
)

// R2: a teammate's merge adds a second marker with the same generic text
// elsewhere; the session edits a file that has an OLD marker with that text
// and adds no debt. The count excess is pinned on the touched file.
func TestDebtGateRegression2_IdenticalTextExcessPinnedOnTouchedFile(t *testing.T) {
	isolateStop(t)
	dir := newStopRepo(t, "")
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	write("add.go", "package stopgate\n\n// TODO: handle error\nfunc Add(a, b int) int { return a + b }\n")
	rvGitC(t, dir, "add", "-A")
	rvGitC(t, dir, "commit", "-qm", "add")
	items, err := debt.Scan(context.Background(), dir, 0)
	require.NoError(t, err)
	_, _, err = debt.WriteBaseline(dir, items, false)
	require.NoError(t, err)
	rvGitC(t, dir, "add", "-A")
	rvGitC(t, dir, "commit", "-qm", "baseline")
	write("other.go", "package stopgate\n\n// TODO: handle error\nfunc Other() {}\n")
	rvGitC(t, dir, "add", "-A")
	rvGitC(t, dir, "commit", "-qm", "teammate merge without a baseline update")
	// The session's own change: a code edit in add.go, no new marker.
	write("add.go", "package stopgate\n\n// TODO: handle error\nfunc Add(a, b int) int { return b + a }\n")
	session := "rv2-excess"
	t.Cleanup(func() {
		resetStopState(session)
		_ = os.Remove(stopStateFile(session, "debt"))
		_ = os.Remove(stopStateFile(session, "debt-blocks"))
	})
	block, warning := debtGate(context.Background(), stopHookInput{SessionID: session}, dir, []string{"add.go"})
	if block != nil {
		t.Errorf("the session added no debt, yet it is blocked on add.go's pre-existing marker: %s", block.Reason)
	} else {
		t.Logf("no block; warning=%q", warning)
	}
}
