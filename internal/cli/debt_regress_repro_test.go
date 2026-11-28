package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func rvGitC(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// AC-11: removing one of the blocked items (no item added) must not block again.
func TestDebtGateRegression_PartialFixBlocksAgain(t *testing.T) {
	dir := debtStopRepo(t)
	session := "rv-partial"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "debt")) })
	write := func(body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body), 0o644))
	}
	write("package stopgate\n\n// TODO handle overflow\n// TODO handle underflow\nfunc Add(a, b int) int { return a + b }\n")
	block, _ := debtGate(context.Background(), stopHookInput{SessionID: session}, dir, []string{"add.go"})
	require.NotNil(t, block)
	t.Logf("first stop blocks: %v", block.Decision)
	write("package stopgate\n\n// TODO handle overflow\nfunc Add(a, b int) int { return a + b }\n")
	block, warning := debtGate(context.Background(), stopHookInput{SessionID: session}, dir, []string{"add.go"})
	if block != nil {
		t.Errorf("AC-11: no new item was added (one was removed), yet the stop blocks again: %s", block.Reason)
	} else {
		t.Logf("warning only: %s", warning)
	}
}

// A debt item that this session never touched (pulled from main past the
// committed baseline) blocks a docs-only change.
func TestDebtGateRegression_UntouchedFileDebtBlocksDocsOnlyChange(t *testing.T) {
	dir := debtStopRepo(t)
	rvGitC(t, dir, "commit", "-qm", "baseline")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.go"), []byte("package stopgate\n\n// FIXME teammate's shortcut\nfunc Other() {}\n"), 0o644))
	rvGitC(t, dir, "add", "-A")
	rvGitC(t, dir, "commit", "-qm", "teammate change merged without updating the baseline")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# docs\n"), 0o644))
	out := runStop(t, dir, "rv-untouched")
	t.Cleanup(func() { resetStopState("rv-untouched"); _ = os.Remove(stopStateFile("rv-untouched", "debt")) })
	t.Logf("stop output for a README-only change: decision=%q reason=%q system=%q", out.Decision, out.Reason, out.SystemMessage)
	if out.Decision == "block" {
		t.Errorf("a README-only change is blocked for debt in a file it never touched")
	}
}
