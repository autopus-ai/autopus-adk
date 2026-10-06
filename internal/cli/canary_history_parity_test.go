package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// canaryClockFields are the only wall-clock values canary prints; parity
// compares everything else byte for byte.
var canaryClockFields = regexp.MustCompile(`"(timestamp|generated_at)": ?"[^"]*"`)

// canaryRunOutput is what a canary run shows its caller.
type canaryRunOutput struct {
	stdout, stderr, exit, latest string
}

func runCanaryForParity(t *testing.T, dir string, args ...string) canaryRunOutput {
	t.Helper()
	cmd := NewRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"canary", "--project-dir", dir}, args...))
	run := canaryRunOutput{exit: "exit 0"}
	if err := cmd.Execute(); err != nil {
		run.exit = "error: " + err.Error()
	}
	latest, err := os.ReadFile(filepath.Join(dir, ".autopus", "canary", "latest.json"))
	require.NoError(t, err)
	run.stdout = canaryClockFields.ReplaceAllString(stdout.String(), `"$1":"<clock>"`)
	run.stderr = stderr.String()
	run.latest = canaryClockFields.ReplaceAllString(string(latest), `"$1":"<clock>"`)
	return run
}

// disableCanaryHistory swaps the append for a no-op until restore runs.
func disableCanaryHistory() (restore func()) {
	previous := canaryHistoryAppend
	canaryHistoryAppend = func(context.Context, string, string, float64) error { return nil }
	return func() { canaryHistoryAppend = previous }
}

// writeBrokenGoProject makes the build check fail at once, the build-failure
// early return of runCanary, before any harness or network check runs.
func writeBrokenGoProject(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package main\n\nfunc main() { undefinedSymbol() }\n"), 0o644))
}

// S18: latest.json, stdout, the JSON envelope, and the exit code are
// byte-identical with history enabled and disabled, in text and JSON mode;
// only the enabled run appends, and a dry run appends nothing anywhere.
// Not parallel: it swaps the package-level append seam.
func TestCanaryHistory_ParityWithHistoryEnabledAndDisabled(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	for _, tc := range []struct {
		name   string
		build  bool
		args   []string
		series string
	}{
		{"dry-run text", false, []string{"--dry-run", "--api-url", "https://API.Example.com:443/health"}, ""},
		{"dry-run json", false, []string{"--dry-run", "--format", "json"}, ""},
		{"build failure text", true, []string{"--api-url", "https://API.Example.com:443/health"}, "canary.failure_rate:api.example.com"},
		{"build failure json", true, []string{"--format", "json"}, "canary.failure_rate:local"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.build {
				writeBrokenGoProject(t, dir)
			}
			metrics := filepath.Join(dir, ".autopus", "metrics")

			enabled := runCanaryForParity(t, dir, tc.args...)
			if tc.series == "" {
				assert.NoDirExists(t, metrics, "a dry run creates no history, lock, or directory")
			} else {
				observations := canaryHistoryLines(t, dir)
				require.Len(t, observations, 1)
				assert.Equal(t, tc.series, observations[0].Series)
				assert.Equal(t, "c1", observations[0].SampleKey)
				assert.Equal(t, 1.0, observations[0].Value)
			}
			require.NoError(t, os.RemoveAll(metrics))

			restore := disableCanaryHistory()
			disabled := runCanaryForParity(t, dir, tc.args...)
			restore()
			assert.NoDirExists(t, metrics)
			assert.Equal(t, disabled, enabled)
			assert.Empty(t, enabled.stderr)
			if tc.build {
				assert.Contains(t, enabled.exit, "build:.:go failed")
			} else {
				assert.Equal(t, "exit 0", enabled.exit)
			}
		})
	}
}

// S18: with .autopus/metrics/ read-only the run prints a stderr warning and
// keeps its stdout, latest.json, and exit code.
func TestCanaryHistory_ReadOnlyHistoryKeepsTheExitCode(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not bind on this platform or user")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	writable, readOnly := t.TempDir(), t.TempDir()
	writeBrokenGoProject(t, writable)
	writeBrokenGoProject(t, readOnly)
	metrics := filepath.Join(readOnly, ".autopus", "metrics")
	require.NoError(t, os.MkdirAll(metrics, 0o700))
	require.NoError(t, os.Chmod(metrics, 0o500))
	t.Cleanup(func() { _ = os.Chmod(metrics, 0o700) })

	want := runCanaryForParity(t, writable)
	got := runCanaryForParity(t, readOnly)
	assert.Contains(t, got.stderr, "canary history append failed")
	assert.Equal(t, want.exit, got.exit)
	assert.Contains(t, got.exit, "build:.:go failed")
	assert.Equal(t, want.stdout, got.stdout)
	assert.NoFileExists(t, filepath.Join(metrics, healthband.CanaryRunsFile))
	assert.Len(t, canaryHistoryLines(t, writable), 1)
}
