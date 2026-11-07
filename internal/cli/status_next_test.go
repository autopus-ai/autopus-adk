package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// S7: Desktop is optional. Without it status still names valid commands; an
// installed but unconnected Desktop adds only an optional connect action.
func TestStatusNext_DesktopIsOptional(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	f := newStatusFixture(t, "claude-code")
	f.commit("baseline")

	f.probe = stubStatusProbe([]string{"claude"}, false, home)
	text := f.runStatus()
	assert.Contains(t, text, "Desktop: not installed (optional; the ADK works standalone)")
	assert.NotContains(t, text, "auto connect")
	assert.Contains(t, text, `/auto review (claude-code)`)
	data := f.statusJSON()
	assert.Equal(t, "not_installed", jsonPath(data, "environment", "desktop", "state"))
	assert.NotContains(t, statusActions(t, data), "desktop_connect")

	f.probe = stubStatusProbe([]string{"claude"}, true, home)
	text = f.runStatus()
	assert.Contains(t, text, "Desktop: installed, not connected (optional; the ADK works standalone)")
	assert.Contains(t, text, "cd "+f.root+" && auto connect\n")
	assert.Contains(t, text, "optional · in "+f.root+" · runs in terminal · provider: none · calls: Autopus server sign-in")
	connect := statusActions(t, f.statusJSON())["desktop_connect"]
	require.NotNil(t, connect)
	assert.Equal(t, "auto connect", connect["command"])
	assert.Equal(t, true, connect["optional"])
	assert.Equal(t, false, connect["auto_run"])
	assert.Contains(t, connect["external_calls"], "Autopus server")

	workerConfig := filepath.Join(home, ".config", "autopus", "worker.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(workerConfig), 0o700))
	require.NoError(t, os.WriteFile(workerConfig, []byte("server: fixture\n"), 0o600))
	text = f.runStatus()
	assert.Contains(t, text, "Desktop: connected (worker config found)")
	assert.NotContains(t, text, "auto connect")
}

// S8: installed CLIs, configured platforms, and the platform pipeline
// commands use are reported apart; omp runs only when named, and with no
// pipeline platform no pipeline command is offered.
func TestStatusNext_PipelinePlatformFollowsWhatIsInstalled(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "codex")
	specDir := f.writeSpec("SPEC-RUN-004", "approved")
	f.commit("baseline")
	// A run on another commit cannot resume, so status offers a new run.
	snapshot, err := pipeline.SpecSnapshotHash(specDir)
	require.NoError(t, err)
	receipt := pipeline.NewBlockedRunReceipt("SPEC-RUN-004", pipeline.StrategySequential, "provider failed")
	f.saveCheckpoint("SPEC-RUN-004", pipeline.Checkpoint{
		Version: pipeline.CheckpointVersion, RouteVersion: pipeline.PipelineRouteVersion, SpecID: "SPEC-RUN-004",
		SnapshotHash: snapshot, GitCommitHash: strings.Repeat("0", 40), TaskStatus: map[string]pipeline.CheckpointStatus{},
		Receipt: &receipt,
	}, false)

	cases := []struct {
		installed                    []string
		line, note, rerun, firstNext string
	}{
		{installed: []string{"claude", "codex", "agy"},
			line:  "auto pipeline run platform: claude — first installed of claude → codex → agy",
			note:  "note: claude is not a configured platform here; pass --platform codex to use codex",
			rerun: "auto pipeline run SPEC-RUN-004 --platform claude"},
		{installed: []string{"omp"},
			line:  "auto pipeline run platform: omp — claude, codex, and agy are not installed; omp is, and runs only with --platform omp",
			note:  "note: omp is not a configured platform here, and no configured platform's CLI is installed",
			rerun: "auto pipeline run SPEC-RUN-004 --platform omp"},
		{installed: []string{"opencode"},
			line:      "auto pipeline run platform: none — no pipeline platform is installed (claude, codex, agy, or omp)",
			firstNext: "auto pipeline run needs claude, codex, agy, or omp, and none is installed"},
		{installed: nil,
			line:      "auto pipeline run platform: none — no pipeline platform is installed (claude, codex, agy, or omp)",
			firstNext: "no AI CLI (claude, codex, agy, opencode, omp) is on PATH"},
	}
	for _, tc := range cases {
		name := statusListOrNone(tc.installed)
		f.probe = stubStatusProbe(tc.installed, false, t.TempDir())
		text := f.runStatus()
		assert.Contains(t, text, "AI CLIs on PATH: "+name, name)
		assert.Contains(t, text, "Configured platforms: codex", name)
		assert.Contains(t, text, tc.line, name)
		if tc.note != "" {
			assert.Contains(t, text, tc.note, name)
		}
		actions := statusActions(t, f.statusJSON())
		if tc.rerun != "" {
			require.Contains(t, actions, "rerun", name)
			assert.Equal(t, tc.rerun, actions["rerun"]["command"], name)
			continue
		}
		assert.NotContains(t, actions, "rerun", "%s: no pipeline command without a pipeline platform", name)
		require.Contains(t, actions, "doctor", name)
		assert.Contains(t, actions["doctor"]["purpose"], tc.firstNext, name)
		assert.Contains(t, text, "  1. cd "+f.root+" && auto doctor\n", name)
	}
}

// Values from receipts and SPEC names never reach a command unvalidated: a
// platform with shell syntax yields no resume command, a SPEC ID the pipeline
// rejects yields no command at all, and control characters never reach the
// screen. Directories are quoted (REQ-SEV-NEXT-04).
func TestStatusNext_CommandsCarryOnlyValidatedValues(t *testing.T) {
	t.Parallel()
	parent := filepath.Join(t.TempDir(), "it's a project")
	require.NoError(t, os.MkdirAll(parent, 0o755))
	f := newStatusFixture(t, "codex")
	f.root = parent
	initTestGitRepo(t, f.root)
	f.probe = stubStatusProbe([]string{"codex"}, false, t.TempDir())
	f.write("autopus.yaml", "mode: full\nproject_name: fixture\nplatforms:\n  - codex\n")
	f.writeSpec("SPEC-INJECT-001", "approved")
	f.writeSpec("SPEC-X;touch PWN", "approved")
	f.commit("baseline")
	f.stoppedRun("SPEC-INJECT-001")
	f.editCheckpoint("SPEC-INJECT-001", func(cp *pipeline.Checkpoint) {
		cp.Receipt.ResolvedProviders = []string{"codex; touch PWN"}
		cp.Receipt.Blocker = "evil\x1b[31m red\nsecond line"
	})

	text := f.runStatus()
	data := f.statusJSON()
	resume := jsonPath(jsonSpec(t, data, "SPEC-INJECT-001"), "evidence", "execution", "resume")
	assert.Equal(t, true, jsonPath(resume, "admitted"))
	assert.Nil(t, jsonPath(resume, "command"))
	assert.Equal(t, "the run's recorded platform is not one auto pipeline run accepts", jsonPath(resume, "command_note"))
	quoted := `cd '` + strings.ReplaceAll(f.root, "'", `'\''`) + `' && auto pipeline run SPEC-INJECT-001 --platform codex`
	assert.Contains(t, text, quoted+"\n", "the rerun uses the validated platform in a quoted directory")
	for _, raw := range data["next_actions"].([]any) {
		action := raw.(map[string]any)
		command, _ := action["command"].(string)
		shell, _ := action["shell"].(string)
		assert.NotContains(t, command+shell, "PWN")
		assert.NotEqual(t, "SPEC-X;touch PWN", action["spec_id"])
	}
	assert.NotContains(t, text, "\x1b")
	assert.Contains(t, text, "blocker: evil [31m red second line\n")

	assert.Equal(t, "/tmp/plain-dir", shellQuote("/tmp/plain-dir"))
	assert.Equal(t, `'/tmp/a b'`, shellQuote("/tmp/a b"))
	assert.Equal(t, `'/tmp/it'\''s'`, shellQuote("/tmp/it's"))
	assert.Equal(t, `'$(touch PWN)'`, shellQuote("$(touch PWN)"))
}

// Every action names the absolute directory it runs in, so a command copied
// from `auto status --dir <project>` works from anywhere; a module SPEC's
// commands run in its module.
func TestStatusNext_EveryCommandNamesItsDirectory(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t, "claude-code")
	f.write("mod/autopus.yaml", "mode: full\nproject_name: mod\nplatforms:\n  - codex\n")
	f.write("mod/.autopus/specs/SPEC-MOD-001/spec.md", "# SPEC-MOD-001: module feature\n\n**Status**: approved\n")
	f.commit("baseline")

	actions := statusActions(t, f.statusJSON())
	require.Contains(t, actions, "implement")
	assert.Equal(t, filepath.Join(f.root, "mod"), actions["implement"]["cwd"])
	assert.Equal(t, "$codex-auto-go SPEC-MOD-001", actions["implement"]["command"], "the module routes by its own platforms")
	assert.Equal(t, `/auto qa`, actions["qa"]["command"], "the workspace routes by its own")
	for id, action := range actions {
		cwd, _ := action["cwd"].(string)
		assert.True(t, filepath.IsAbs(cwd), "%s: cwd %q", id, cwd)
		if action["runs_in"] == "terminal" {
			assert.True(t, strings.HasPrefix(action["shell"].(string), "cd "+cwd+" && "), id)
		}
	}
	assert.Contains(t, f.runStatus(), "in "+filepath.Join(f.root, "mod")+" · runs in agent_session")
}
