package cli

// SPEC-PANERM-001 T12 (REQ-09, S6): the config notice names the retired
// orchestra keys that config loads ignore, appears at most once per process on
// the executing command's stderr, and stays silent for --quiet or a stderr that
// is not a terminal. Tests that install the process-wide retired key reporter
// do not run in parallel.

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const legacyPaneFixtures = "../../pkg/config/testdata/legacy_pane"

// c2NoticeLine is the S6 notice for C2: P2 of acceptance.md in byte order.
const c2NoticeLine = "auto: warning: ignored removed autopus.yaml keys: " +
	"features.cc21.monitor_pattern_timeout_ms, orchestra.providers.claude.pane_args, " +
	"orchestra.providers.claude.working_patterns, orchestra.providers.codex.interactive_input, " +
	"orchestra.providers.my-local.interactive_input, orchestra.providers.my-local.pane_args, " +
	"orchestra.providers.my-local.working_patterns, orchestra.subprocess.enabled" +
	`; the orchestra pane backend was retired (SPEC-PANERM-001); run "auto update" or delete the keys` + "\n"

func readLegacyPane(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(legacyPaneFixtures, name))
	require.NoError(t, err)
	return data
}

// legacyPaneDir writes data as autopus.yaml into a fresh directory.
func legacyPaneDir(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), data, 0o644))
	return dir
}

func alwaysTerminal(io.Writer) bool { return true }

func noticeProbeCommand(t *testing.T, quiet bool) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmd := &cobra.Command{Use: "probe"}
	cmd.Flags().Bool("quiet", false, "")
	if quiet {
		require.NoError(t, cmd.Flags().Set("quiet", "true"))
	}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	return cmd, &stdout, &stderr
}

func installNotice(t *testing.T, notice *configNotice) {
	t.Helper()
	previous := config.SetRetiredKeyReporter(notice.report)
	t.Cleanup(func() { config.SetRetiredKeyReporter(previous) })
}

func TestConfigNotice_TwoLoadsThroughOneNotifierWriteOneLine(t *testing.T) {
	notice := newConfigNotice(alwaysTerminal)
	installNotice(t, notice)
	cmd, stdout, stderr := noticeProbeCommand(t, false)
	notice.bind(cmd)
	dir := legacyPaneDir(t, readLegacyPane(t, "c2.yaml"))

	_, err := config.Load(dir)
	require.NoError(t, err)
	_, err = config.LoadPreview(dir)
	require.NoError(t, err)

	assert.Equal(t, c2NoticeLine, stderr.String())
	assert.Empty(t, stdout.String(), "the notice never reaches stdout")
}

func TestConfigNotice_HoldsReportsUntilTheCommandIsKnown(t *testing.T) {
	t.Parallel()
	notice := newConfigNotice(alwaysTerminal)
	notice.report([]string{"orchestra.subprocess.enabled", "orchestra.providers.codex.pane_args"})
	notice.report([]string{"features.cc21.monitor_pattern_timeout_ms", "orchestra.subprocess.enabled"})
	cmd, _, stderr := noticeProbeCommand(t, false)

	notice.bind(cmd)
	notice.report([]string{"orchestra.providers.later.pane_args"})
	notice.bind(cmd)

	assert.Equal(t, "auto: warning: ignored removed autopus.yaml keys: features.cc21.monitor_pattern_timeout_ms, "+
		"orchestra.providers.codex.pane_args, orchestra.subprocess.enabled"+
		`; the orchestra pane backend was retired (SPEC-PANERM-001); run "auto update" or delete the keys`+"\n",
		stderr.String(), "held paths merge once in byte order; a later load adds no second line")
}

func TestConfigNotice_QuietOrNonTerminalStderrSuppressesTheLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		quiet    bool
		terminal func(io.Writer) bool
	}{
		{name: "quiet", quiet: true, terminal: alwaysTerminal},
		{name: "not a terminal", terminal: stderrIsTerminal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notice := newConfigNotice(tc.terminal)
			notice.report([]string{"orchestra.subprocess.enabled"})
			cmd, stdout, stderr := noticeProbeCommand(t, tc.quiet)
			notice.bind(cmd)
			notice.report([]string{"features.cc21.monitor_pattern_timeout_ms"})
			assert.Empty(t, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func TestStderrIsTerminal_RejectsBuffersAndPipes(t *testing.T) {
	t.Parallel()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	assert.False(t, stderrIsTerminal(writer), "a pipe is no terminal")
	assert.False(t, stderrIsTerminal(&bytes.Buffer{}), "a buffer is no terminal")
}

func TestConfigNotice_ContextCarriesTheNotifierAndNilIsInert(t *testing.T) {
	t.Parallel()
	var unset context.Context // a command executed without a context
	assert.Nil(t, configNoticeFromContext(unset))
	assert.Nil(t, configNoticeFromContext(context.Background()))
	notice := newConfigNotice(alwaysTerminal)
	assert.Same(t, notice, configNoticeFromContext(withConfigNotice(context.Background(), notice)))

	var none *configNotice
	cmd, _, stderr := noticeProbeCommand(t, false)
	none.bind(cmd)
	assert.Empty(t, stderr.String())
}

// The root pre-run binds the notifier that the process entry point put into
// the command context, so `auto check --arch` on C2 writes the line once even
// though it loads the config twice, and `--quiet` silences it.
func TestRootPreRun_BindsTheConfigNoticeOfItsContext(t *testing.T) {
	dir := legacyPaneDir(t, readLegacyPane(t, "c2.yaml"))
	for _, tc := range []struct {
		name    string
		args    []string
		notices int
	}{
		{name: "terminal", args: []string{"check", "--arch", "--dir", dir}, notices: 1},
		{name: "quiet", args: []string{"check", "--arch", "--dir", dir, "--quiet"}, notices: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notice := newConfigNotice(alwaysTerminal)
			installNotice(t, notice)
			root := NewRootCmd()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(tc.args)
			require.NoError(t, root.ExecuteContext(withConfigNotice(context.Background(), notice)))
			assert.Equal(t, tc.notices, strings.Count(stderr.String(), c2NoticeLine), "stderr:\n%s", stderr.String())
			assert.NotContains(t, stdout.String(), "ignored removed autopus.yaml keys")
		})
	}
}
