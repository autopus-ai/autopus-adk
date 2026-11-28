package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Debt blocks share the session's block budget with failing builds.
func TestDebtGateRegression_DebtBlocksEatBuildFailureBudget(t *testing.T) {
	dir := debtStopRepo(t)
	rvGitC(t, dir, "commit", "-qm", "baseline")
	session := "rv-budget"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "debt")) })
	write := func(body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body), 0o644))
	}
	write("package stopgate\n\n// TODO a\nfunc Add(a, b int) int { return a + b }\n")
	out := runStop(t, dir, session)
	t.Logf("stop 1 (new TODO a): %s", out.Decision)
	write("package stopgate\n\n// TODO a\n// TODO b\nfunc Add(a, b int) int { return a + b }\n")
	out = runStop(t, dir, session)
	t.Logf("stop 2 (new TODO b): %s", out.Decision)
	write("package stopgate\n\n// TODO a\n// TODO b\nfunc Add(a, b int) int { return a - b }\n")
	out = runStop(t, dir, session)
	t.Logf("stop 3 (test fails): decision=%q", out.Decision)
	write("package stopgate\n\n// TODO a\n// TODO b\nfunc Add(a, b int) int { return a * b }\n")
	out = runStop(t, dir, session)
	t.Logf("stop 4 (test still fails, different tree): decision=%q system=%.120q", out.Decision, out.SystemMessage)
	if out.Decision != "block" {
		t.Errorf("a failing test got one block instead of %d: the debt blocks used up the session's budget", verifyStopMaxBlocks)
	}
}
