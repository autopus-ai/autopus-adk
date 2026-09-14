package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubReactExec installs deterministic gh/git seams for the duration of the test.
func stubReactExec(t *testing.T, lookPath func(string) (string, error), output func(string, ...string) ([]byte, error)) {
	t.Helper()
	t.Cleanup(func() {
		reactLookPath = execLookPath
		reactOutput = execOutput
	})
	reactLookPath = lookPath
	reactOutput = output
}

func ghInstalled(string) (string, error) { return "/usr/bin/gh", nil }

// reactCheckCmd returns a command whose output is captured in buf.
func reactCheckCmd(buf *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	return cmd
}

// Guards against the diagnostic for a missing gh CLI degrading into a generic failure.
func TestRunReactCheck_MissingGhCLIReportsInstallHint(t *testing.T) {
	stubReactExec(t,
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) {
			t.Fatal("no command may run when gh is absent")
			return nil, nil
		})

	var out bytes.Buffer
	err := runReactCheck(reactCheckCmd(&out), nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gh CLI not found")
	assert.Contains(t, err.Error(), "https://cli.github.com/")
	assert.Empty(t, out.String(), "gh absence must be reported as an error, not as progress output")
}

// Guards against a failing `gh run list` being silently treated as "no failures".
func TestRunReactCheck_RunListFailureSurfacesError(t *testing.T) {
	stubReactExec(t, ghInstalled, func(name string, _ ...string) ([]byte, error) {
		if name == "git" {
			return []byte("origin\n"), nil
		}
		return nil, errors.New("gh exploded")
	})

	var out bytes.Buffer
	err := runReactCheck(reactCheckCmd(&out), nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list CI runs")
	assert.Contains(t, err.Error(), "gh exploded", "underlying cause must stay wrapped")
}

// Guards against unparsable gh JSON being mistaken for an empty run list.
func TestRunReactCheck_MalformedJSONSurfacesParseError(t *testing.T) {
	stubReactExec(t, ghInstalled, func(name string, _ ...string) ([]byte, error) {
		if name == "git" {
			return []byte("origin\n"), nil
		}
		return []byte("{not json"), nil
	})

	var out bytes.Buffer
	err := runReactCheck(reactCheckCmd(&out), nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse CI runs")
}

// Guards the quiet/verbose precedence for the empty-run-list outcome.
func TestRunReactCheck_NoFailuresQuietStaysSilent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		quiet bool
		want  string
	}{
		{name: "verbose announces clean CI", quiet: false, want: "No recent CI failures found."},
		{name: "quiet prints nothing", quiet: true, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubReactExec(t, ghInstalled, func(name string, _ ...string) ([]byte, error) {
				if name == "git" {
					return []byte("origin\n"), nil
				}
				return []byte("[]"), nil
			})

			var out bytes.Buffer
			require.NoError(t, runReactCheck(reactCheckCmd(&out), nil, tc.quiet))
			if tc.want == "" {
				assert.Empty(t, out.String())
				return
			}
			assert.Contains(t, out.String(), tc.want)
		})
	}
}

const twoFailedRunsJSON = `[
 {"databaseId":111,"name":"ci","conclusion":"failure","headBranch":"main","updatedAt":"2026-01-01T00:00:00Z"},
 {"databaseId":222,"name":"lint","conclusion":"failure","headBranch":"dev","updatedAt":"2026-01-02T00:00:00Z"}
]`

// Quiet mode must stop after the summary line: no report directory, no per-run detail.
func TestRunReactCheck_QuietStopsAfterSummaryWithoutWritingReports(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	stubReactExec(t, ghInstalled, func(name string, args ...string) ([]byte, error) {
		if name == "git" {
			return []byte("origin\n"), nil
		}
		if len(args) > 1 && args[1] == "view" {
			t.Fatal("quiet mode must not fetch per-run logs")
		}
		return []byte(twoFailedRunsJSON), nil
	})

	var out bytes.Buffer
	require.NoError(t, runReactCheck(reactCheckCmd(&out), nil, true))

	assert.Equal(t, "Found 2 failed run(s):\n", out.String())
	_, statErr := os.Stat(filepath.Join(dir, ".autopus", "react"))
	assert.True(t, os.IsNotExist(statErr), "quiet mode must not create the report directory")
}

// Verbose mode must persist one report per run carrying the run metadata and logs.
func TestRunReactCheck_VerboseWritesReportPerRun(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	stubReactExec(t, ghInstalled, func(name string, args ...string) ([]byte, error) {
		if name == "git" {
			return []byte("origin\n"), nil
		}
		if len(args) > 1 && args[1] == "view" {
			return []byte("FAIL: build step\n"), nil
		}
		return []byte(twoFailedRunsJSON), nil
	})

	var out bytes.Buffer
	require.NoError(t, runReactCheck(reactCheckCmd(&out), nil, false))

	assert.Contains(t, out.String(), "[111] ci (branch: main, updated: 2026-01-01T00:00:00Z)")
	assert.Contains(t, out.String(), "Report saved:")

	for _, id := range []string{"111", "222"} {
		data, err := os.ReadFile(filepath.Join(dir, ".autopus", "react", id+".md"))
		require.NoError(t, err, "report for run %s must exist", id)
		body := string(data)
		assert.Contains(t, body, "**Run ID**: "+id)
		assert.Contains(t, body, "FAIL: build step")
	}
}

// A log-fetch failure must degrade to a warning plus placeholder logs, not abort the sweep.
func TestRunReactCheck_LogFetchFailureDegradesToPlaceholder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	stubReactExec(t, ghInstalled, func(name string, args ...string) ([]byte, error) {
		if name == "git" {
			return []byte("origin\n"), nil
		}
		if len(args) > 1 && args[1] == "view" {
			return nil, errors.New("log stream gone")
		}
		return []byte(twoFailedRunsJSON), nil
	})

	var out bytes.Buffer
	require.NoError(t, runReactCheck(reactCheckCmd(&out), nil, false))

	assert.Contains(t, out.String(), "could not fetch logs for run 111")
	assert.Equal(t, 2, strings.Count(out.String(), "Report saved:"), "both runs must still get reports")

	data, err := os.ReadFile(filepath.Join(dir, ".autopus", "react", "111.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Log fetch failed.")
}

// writeReactReport must surface an unusable destination instead of reporting success.
func TestWriteReactReport_UnwritablePathErrors(t *testing.T) {
	t.Parallel()

	err := writeReactReport(filepath.Join(t.TempDir(), "missing-dir", "1.md"), ciRun{DatabaseID: 1}, "logs")
	require.Error(t, err)
}

// hasGitRemote must treat a git failure as "no remote" rather than propagating an error.
func TestHasGitRemote_ClassifiesOutput(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  []byte
		err  error
		want bool
	}{
		{name: "git failure means no remote", err: errors.New("not a repo")},
		{name: "blank output means no remote", out: []byte("  \n")},
		{name: "named remote means configured", out: []byte("origin\n"), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubReactExec(t, ghInstalled, func(string, ...string) ([]byte, error) { return tc.out, tc.err })
			got, err := hasGitRemote()
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
