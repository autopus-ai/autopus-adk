package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimedetect "github.com/insajin/autopus-adk/pkg/detect"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/terminal"
)

// fakeWiringTerminal is a minimal terminal.Terminal double whose Name() is
// controllable. SelectBackend/paneCapable only exercise Name(), so the other
// methods are inert no-ops.
type fakeWiringTerminal struct{ name string }

func (f fakeWiringTerminal) Name() string { return f.name }
func (fakeWiringTerminal) CreateWorkspace(context.Context, string) error {
	return nil
}
func (fakeWiringTerminal) SplitPane(context.Context, terminal.Direction) (terminal.PaneID, error) {
	return "", nil
}
func (fakeWiringTerminal) SendCommand(context.Context, terminal.PaneID, string) error {
	return nil
}
func (fakeWiringTerminal) SendLongText(context.Context, terminal.PaneID, string) error {
	return nil
}
func (fakeWiringTerminal) Notify(context.Context, string) error { return nil }
func (fakeWiringTerminal) ReadScreen(context.Context, terminal.PaneID, terminal.ReadScreenOpts) (string, error) {
	return "", nil
}
func (fakeWiringTerminal) PipePaneStart(context.Context, terminal.PaneID, string) error {
	return nil
}
func (fakeWiringTerminal) PipePaneStop(context.Context, terminal.PaneID) error {
	return nil
}
func (fakeWiringTerminal) Close(context.Context, string) error { return nil }

// TestBrainstormSubprocessFlagDefaultsToFalse verifies REQ-001: the brainstorm
// command now defaults to the interactive pane path (--subprocess=false).
func TestBrainstormSubprocessFlagDefaultsToFalse(t *testing.T) {
	t.Parallel()
	cmd := newOrchestraBrainstormCmd()
	flag := cmd.Flags().Lookup("subprocess")
	require.NotNil(t, flag, "brainstorm command must expose a --subprocess flag")
	assert.Equal(t, "false", flag.DefValue, "REQ-001: brainstorm default must be pane mode (--subprocess=false)")
}

// TestSpecReviewBackendFactoryRoutesThroughSelectBackend verifies REQ-002: the
// default specReviewBackendFactory is SelectBackend, so a pane-capable terminal
// yields the pane backend and a plain terminal yields the subprocess backend.
func TestSpecReviewBackendFactoryRoutesThroughSelectBackend(t *testing.T) {
	t.Parallel()

	paneCfg := orchestra.OrchestraConfig{Terminal: fakeWiringTerminal{name: "cmux"}}
	paneBackend := specReviewBackendFactory(paneCfg)
	require.NotNil(t, paneBackend)
	assert.Equal(t, "pane", paneBackend.Name(), "REQ-002: cmux terminal must route to the interactive pane backend")

	plainCfg := orchestra.OrchestraConfig{Terminal: fakeWiringTerminal{name: "plain"}}
	plainBackend := specReviewBackendFactory(plainCfg)
	require.NotNil(t, plainBackend)
	assert.Equal(t, "subprocess", plainBackend.Name(), "REQ-002: plain terminal must route to the subprocess backend")
}

// TestOrchestraRunBackendFactoryConsumesSelectBackend verifies REQ-003: the run
// pipeline builds its backend from SelectBackend with a populated terminal,
// rather than a hardcoded subprocess factory. A cmux terminal yields pane; a nil
// terminal yields subprocess.
func TestOrchestraRunBackendFactoryConsumesSelectBackend(t *testing.T) {
	t.Parallel()

	paneCfg := orchestra.OrchestraConfig{Terminal: fakeWiringTerminal{name: "cmux"}}
	paneBackend := orchestraRunBackendFactory(paneCfg)
	require.NotNil(t, paneBackend)
	assert.Equal(t, "pane", paneBackend.Name(), "REQ-003: cmux terminal must route to the interactive pane backend")

	subprocessCfg := orchestra.OrchestraConfig{Terminal: nil}
	subprocessBackend := orchestraRunBackendFactory(subprocessCfg)
	require.NotNil(t, subprocessBackend)
	assert.Equal(t, "subprocess", subprocessBackend.Name(), "REQ-003: nil terminal must route to the subprocess backend")
}

// TestPaneInteractiveContext verifies that pane execution is disabled in nested
// agent automation, CI, and any non-TTY stdio context, so structured orchestra
// falls back to the subprocess backend instead of spawning panes that time out.
// Also covers REQ-005/REQ-008 nested-agent relaxation and CI floor.
func TestPaneInteractiveContext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		claudeCode    string
		codex         string
		ci            string
		stdinTTY      bool
		stdoutTTY     bool
		hookAvailable bool
		muxInstalled  bool
		want          bool
	}{
		// Normal interactive TTY path (no env vars).
		{"interactive tty, no env", "", "", "", true, true, false, false, true},
		{"piped stdout", "", "", "", true, false, false, false, false},
		{"piped stdin", "", "", "", false, true, false, false, false},
		{"piped both", "", "", "", false, false, false, false, false},

		// CI floor: always false regardless of CLAUDECODE, hook, or mux.
		{"ci environment", "", "", "true", true, true, true, true, false},
		{"ci beats claudecode", "1", "", "1", true, true, true, true, false},

		// S5: CLAUDECODE + hook available + mux installed → true.
		{"S5: claudecode hook+mux ready", "1", "", "", false, false, true, true, true},
		// S5c: CODEX + hook available + mux installed → true.
		{"S5c: codex hook+mux ready", "", "1", "", false, false, true, true, true},
		// CI floor: CODEX cannot force pane mode in CI.
		{"ci beats codex", "", "1", "1", false, false, true, true, false},
		// S5b: CLAUDECODE + hook unavailable → false (floor preserved).
		{"S5b: claudecode no hook", "1", "", "", false, false, false, true, false},
		// S5b: CLAUDECODE + mux not installed → false (floor preserved).
		{"S5b: claudecode no mux", "1", "", "", false, false, true, false, false},
		// CODEX floor mirrors CLAUDECODE: both hook and mux are required.
		{"codex no hook", "", "1", "", false, false, false, true, false},
		{"codex no mux", "", "1", "", false, false, true, false, false},
		// CLAUDECODE present but both conditions false.
		{"claudecode no hook no mux", "1", "", "", false, false, false, false, false},

		// Legacy case: nested claude-code without relaxation flags — still false
		// when hook/mux unavailable, matching original behavior.
		{"nested claude-code no hook no mux", "1", "", "", true, true, false, false, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := paneInteractiveContext(tt.claudeCode, tt.codex, tt.ci, tt.stdinTTY, tt.stdoutTTY, tt.hookAvailable, tt.muxInstalled)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPaneInteractiveContextWithRuntime_ProcessTreeAgentUsesNestedGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		runtime       runtimedetect.AgentRuntime
		ci            string
		hookAvailable bool
		muxInstalled  bool
		want          bool
	}{
		{"claude ancestry", runtimedetect.AgentRuntimeClaudeCode, "", true, true, true},
		{"codex ancestry", runtimedetect.AgentRuntimeCodex, "", true, true, true},
		{"unrelated ancestry", runtimedetect.AgentRuntimeUnknown, "", true, true, false},
		{"ci floor beats claude ancestry", runtimedetect.AgentRuntimeClaudeCode, "1", true, true, false},
		{"ci floor beats codex ancestry", runtimedetect.AgentRuntimeCodex, "1", true, true, false},
		{"claude ancestry still requires hook", runtimedetect.AgentRuntimeClaudeCode, "", false, true, false},
		{"codex ancestry still requires mux", runtimedetect.AgentRuntimeCodex, "", true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := paneInteractiveContextWithRuntime(
				"", false, tt.runtime, tt.ci,
				false, false, tt.hookAvailable, tt.muxInstalled,
			)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDetectStructuredTerminal_NonInteractiveFallsBackToPlain verifies the
// non-interactive/CI floor without inheriting the host's agent markers, hook
// files, cwd, or terminal multiplexer state.
func TestDetectStructuredTerminal_NonInteractiveFallsBackToPlain(t *testing.T) {
	// Given: a fully isolated CI process. CI is the deterministic integration
	// oracle; the pure TestPaneInteractiveContext table covers individual TTY
	// combinations without consulting real file descriptors.
	for _, key := range []string{
		"CLAUDECODE",
		"CODEX",
		"CODEX_CI",
		"CODEX_THREAD_ID",
		"CODEX_MANAGED_BY_NPM",
		"TMUX",
		"TMUX_PANE",
		"CMUX_SOCKET_PATH",
		"CMUX_WORKSPACE_ID",
		"CMUX_SURFACE_ID",
		"CMUX_PANE_ID",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("CI", "1")
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	// When/Then: host hooks or installed cmux/tmux binaries cannot bypass CI.
	assert.Equal(t, "plain", detectStructuredTerminal().Name())
}

// TestSelectBackend_PlainAdapterYieldsSubprocess is the S8 oracle: when
// SelectBackend receives a PlainAdapter terminal (SubprocessMode=false), the
// returned backend must be named "subprocess". This is a regression guard on
// the existing pkg/orchestra/backend.go behaviour — no new code required.
func TestSelectBackend_PlainAdapterYieldsSubprocess(t *testing.T) {
	t.Parallel()
	cfg := orchestra.OrchestraConfig{
		SubprocessMode: false,
		Terminal:       &terminal.PlainAdapter{},
	}
	backend := orchestra.SelectBackend(cfg)
	require.NotNil(t, backend, "S8: SelectBackend must return a non-nil backend")
	assert.Equal(t, "subprocess", backend.Name(), "S8: PlainAdapter must route to subprocess backend")
}
