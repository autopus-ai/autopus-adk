package cli

// SPEC-PANERM-001 T6 (REQ-04, REQ-05, REQ-06) through the cobra tree: retired
// subcommands fail with the retirement error before the root pre-run and stay
// out of help, retired flags parse and stay out of help, and --yield-rounds
// warns once. panerm_retired_cli_test.go checks the same contract on the built
// binary (stderr bytes and exit status).

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var retiredOrchestraNames = []string{"collect", "inject", "cleanup", "status", "wait", "result"}

var retiredFlagTokens = []string{"--no-detach", "--subprocess", "--plain", "--yield-rounds"}

const yieldRoundsNoticeLine = "auto: warning: --yield-rounds was retired with the orchestra pane backend (SPEC-PANERM-001); all rounds run synchronously"

func retiredOrchestraErrorText(name string) string {
	return "auto orchestra " + name + " was retired with the orchestra pane backend (SPEC-PANERM-001); " +
		"orchestra commands now run synchronously and print their result directly"
}

// executeAutoRoot runs one auto argv through a fresh root command and returns
// what cobra wrote to its stdout and stderr writers.
func executeAutoRoot(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestRetiredOrchestraSubcommands_FailWithRetirementErrorBeforeRootPreRun(t *testing.T) {
	t.Parallel()

	for _, name := range retiredOrchestraNames {
		forms := map[string][]string{
			"bare": {"orchestra", name},
			"args": {"orchestra", name, "job-123", "--timeout", "60"},
			// An unknown preset and a missing config fail the root pre-run, so
			// only a stub with its own pre-run reaches the retirement error.
			"root flags": {"--quality", "x", "--config", "/nonexistent/autopus.yaml", "orchestra", name, "job-123"},
		}
		for form, args := range forms {
			t.Run(name+"/"+form, func(t *testing.T) {
				t.Parallel()

				stdout, stderr, err := executeAutoRoot(t, args...)

				require.EqualError(t, err, retiredOrchestraErrorText(name))
				assert.Equal(t, 1, exitCodeForError(err))
				assert.Empty(t, stdout)
				assert.Empty(t, stderr)
			})
		}
	}
}

func TestOrchestraHelp_ListsActiveSubcommandsOnly(t *testing.T) {
	t.Parallel()

	stdout, _, err := executeAutoRoot(t, "orchestra", "--help")

	require.NoError(t, err)
	for _, name := range []string{"brainstorm", "plan", "review", "secure", "run"} {
		assert.Regexp(t, regexp.MustCompile(`(?m)^  `+name+` `), stdout)
	}
	for _, name := range retiredOrchestraNames {
		assert.NotRegexp(t, regexp.MustCompile(`(?m)^  `+name+`\b`), stdout)
	}
}

func TestRetiredOrchestraFlags_ParseButStayOutOfHelp(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		path    []string
		retired []string
	}{
		{path: []string{"orchestra", "brainstorm"}, retired: []string{"no-detach", "yield-rounds", "subprocess"}},
		{path: []string{"orchestra", "plan"}, retired: []string{"no-detach", "subprocess"}},
		{path: []string{"orchestra", "review"}, retired: []string{"no-detach"}},
		{path: []string{"orchestra", "secure"}, retired: []string{"no-detach"}},
		{path: []string{"orchestra", "run"}, retired: []string{"subprocess"}},
		{path: []string{"spec", "review"}, retired: []string{"subprocess", "plain"}},
	} {
		t.Run(strings.Join(tc.path, " "), func(t *testing.T) {
			t.Parallel()

			help, _, err := executeAutoRoot(t, append(append([]string(nil), tc.path...), "--help")...)
			require.NoError(t, err)
			require.Contains(t, help, "Flags:")
			for _, token := range retiredFlagTokens {
				assert.NotContains(t, help, token)
			}

			cmd, _, err := NewRootCmd().Find(tc.path)
			require.NoError(t, err)
			for _, name := range tc.retired {
				assert.NoError(t, cmd.ParseFlags([]string{"--" + name}), "--%s stays accepted", name)
			}
		})
	}
}

func TestOrchestraBrainstorm_YieldRoundsWarnsOnce(t *testing.T) {
	// An unsupported --format stops the run before any provider starts.
	t.Setenv("TMPDIR", t.TempDir())
	for _, tc := range []struct {
		name    string
		args    []string
		notices int
	}{
		{name: "flagged", args: []string{"orchestra", "brainstorm", "x", "--yield-rounds", "--format", "yaml"}, notices: 1},
		{name: "unflagged", args: []string{"orchestra", "brainstorm", "x", "--format", "yaml"}, notices: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := executeAutoRoot(t, tc.args...)

			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.Equal(t, tc.notices, strings.Count(stderr, yieldRoundsNoticeLine+"\n"), stderr)
		})
	}
}
