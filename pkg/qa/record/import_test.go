package record_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/record"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

const guiPack = `id: browser-gui-explore
title: GUI exploration
surface: frontend
lanes: [gui-explore]
adapter:
  id: gui-explore
command:
  argv: ["npm", "exec", "playwright", "test"]
  cwd: .
  timeout: 120s
checks:
  - id: browser-gui-explore
    type: gui_exploration
    expected:
      exit_code: 0
gui:
  allowed_origins: ["http://127.0.0.1:4173"]
  forbidden_actions: [mutation]
  selector_strategy: role-first
  network_policy:
    mode: summary-only
source_refs:
  source_spec: SPEC-QAMESH-003
  acceptance_refs: [AC-1]
  owned_paths: ["tests/**"]
`

// projectWithPack builds a project whose only GUI Journey Pack allows
// http://127.0.0.1:4173.
func projectWithPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range map[string]string{
		".autopus/qa/journeys/browser-gui-explore.yaml":   guiPack,
		".autopus/qa/capture/autopus-capture.fixture.cjs": "module.exports = {};\n",
		"playwright.config.ts":                            "export default { testDir: './tests' };\n",
	} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return dir
}

func writeSource(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestQARecordImport_UnsupportedLineFailsWithoutAllowPartial(t *testing.T) {
	t.Parallel()
	project := projectWithPack(t)
	from := writeSource(t, "login.spec.ts", loginCodegen)

	result, err := record.Import(project, record.ImportOptions{From: from, ID: "login"})

	require.Error(t, err)
	code, setupGap := record.CodeOf(err)
	assert.Equal(t, record.CodeUnsupportedLines, code)
	assert.False(t, setupGap)
	assert.Contains(t, err.Error(), "line 8: await page.mouse.click(120, 48);")
	require.Len(t, result.Unsupported, 1)
	assert.NoFileExists(t, filepath.Join(scenario.CandidatesDir(project), "login.yaml"))
}

func TestQARecordImport_AllowPartialWritesValidRecordingCandidate(t *testing.T) {
	t.Parallel()
	project := projectWithPack(t)
	from := writeSource(t, "login.spec.ts", loginCodegen)

	result, err := record.Import(project, record.ImportOptions{From: from, ID: "login", AllowPartial: true})

	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.Equal(t, filepath.Join(scenario.CandidatesDir(project), "login.yaml"), result.Path)
	assert.Equal(t, "browser-gui-explore", result.Journey)
	assert.Equal(t, 5, result.Steps)
	loaded, err := scenario.LoadFile(result.Path)
	require.NoError(t, err)
	assert.Equal(t, scenario.SchemaVersionV2, loaded.SchemaVersion)
	assert.Equal(t, scenario.IntentRecording, loaded.IntentSource)
	assert.Equal(t, "http://127.0.0.1:4173", loaded.Origin)
	assert.True(t, strings.HasPrefix(loaded.RecordingRef, "login.spec.ts@sha256:"), loaded.RecordingRef)
	require.Len(t, loaded.Screens, 1)
	assert.Equal(t, scenario.Screen{ID: "home", Path: "/", Steps: []scenario.Step{
		{Click: &scenario.Target{Role: "link", Name: "Sign in"}, By: scenario.ByHuman},
		{Fill: &scenario.FillAction{Target: scenario.Target{Label: "Email"}, Value: "ada@example.com"}, By: scenario.ByHuman},
		{Press: &scenario.PressAction{Target: scenario.Target{Label: "Email"}, Key: "Enter"}, By: scenario.ByHuman},
		{ExpectText: "Welcome back", By: scenario.ByHuman},
		{ExpectURL: "/dashboard", By: scenario.ByHuman},
	}}, loaded.Screens[0])
	body, err := os.ReadFile(result.Path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(body), "# Recorded journey imported by `auto qa record` from login.spec.ts@sha256:"))
	assert.Contains(t, string(body), "# 1 unsupported line(s) were dropped by --allow-partial.")
}

func TestQARecordImport_AgentAssertionWithoutAcNeedsConfirmation(t *testing.T) {
	t.Parallel()
	project := projectWithPack(t)
	from := writeSource(t, "session.jsonl", agentLog)

	result, err := record.Import(project, record.ImportOptions{From: from, ID: "agent-login"})

	require.NoError(t, err)
	assert.Equal(t, record.FormatJSONL, result.Format)
	assert.Equal(t, 1, result.ConfirmRequired)
	loaded, err := scenario.LoadFile(result.Path)
	require.NoError(t, err)
	require.Len(t, loaded.Screens, 1)
	assert.Equal(t, "/login", loaded.Screens[0].Path)
	assert.Equal(t, []scenario.Step{
		{Fill: &scenario.FillAction{Target: scenario.Target{Label: "Email"}, Value: "ada@example.com"}, By: scenario.ByHuman},
		{Fill: &scenario.FillAction{Target: scenario.Target{Label: "Password"}, ValueEnv: "E2E_PASSWORD"}, By: scenario.ByHuman},
		{Click: &scenario.Target{Role: "button", Name: "Sign in"}, By: scenario.ByHuman},
		{ExpectText: "Welcome back", Ac: "AC-LOGIN-1", By: scenario.ByHuman},
		{ExpectRole: &scenario.RoleTarget{Role: "heading", Name: "Dashboard"}, By: scenario.ByAgent, Confirm: scenario.ConfirmRequired},
		{ExpectURL: "/dashboard", Ac: "AC-LOGIN-2", By: scenario.ByAgent},
	}, loaded.Screens[0].Steps)
}

func TestQARecordImport_NeverOverwritesADifferentCandidate(t *testing.T) {
	t.Parallel()
	project := projectWithPack(t)
	jsonl := writeSource(t, "session.jsonl", agentLog)
	first, err := record.Import(project, record.ImportOptions{From: jsonl, ID: "login"})
	require.NoError(t, err)
	before, err := os.ReadFile(first.Path)
	require.NoError(t, err)

	again, err := record.Import(project, record.ImportOptions{From: jsonl, ID: "login"})
	require.NoError(t, err)
	assert.False(t, again.Created, "re-importing the same recording is a no-op")

	codegen := writeSource(t, "login.spec.ts", loginCodegen)
	_, err = record.Import(project, record.ImportOptions{From: codegen, ID: "login", AllowPartial: true})
	code, _ := record.CodeOf(err)
	assert.Equal(t, record.CodeCandidateExists, code)
	after, err := os.ReadFile(first.Path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestQARecordToScenario_RefusesRecordingsThatCannotFormScreens(t *testing.T) {
	t.Parallel()
	opts := record.Options{ID: "x", Journey: "j", Origin: "http://127.0.0.1:4173", RecordingRef: "x.jsonl@sha256:0"}
	for name, tc := range map[string]struct{ log, code, mention string }{
		"first event is not goto": {`{"action":"click","target":{"role":"button","name":"Go"},"by":"human"}`,
			record.CodeGotoMissing, "line 1"},
		"goto leaves the origin": {`{"action":"goto","url":"https://other.example/","by":"agent"}` + "\n" +
			`{"action":"click","target":{"role":"button"},"by":"agent"}`, record.CodeOriginMismatch, "line 1"},
		"expect url leaves the origin": {`{"action":"goto","url":"/","by":"human"}` + "\n" +
			`{"expect":"url","url":"https://other.example/x","by":"human"}`, record.CodeOriginMismatch, "line 2"},
		"only navigation": {`{"action":"goto","url":"/","by":"human"}`, record.CodeEmpty, "no action"},
	} {
		rec, unsupported := record.ParseJSONL([]byte(tc.log))
		require.Empty(t, unsupported, name)
		_, err := record.ToScenario(rec, opts)
		code, _ := record.CodeOf(err)
		assert.Equal(t, tc.code, code, name)
		if err != nil {
			assert.Contains(t, err.Error(), tc.mention, name)
		}
	}
}

// fakeCodegen stands in for `npx playwright codegen -o <out> <origin>`: it
// writes body, retargeted to the origin it was given, to the output path.
func fakeCodegen(calls *[]string, body string) record.Runner {
	return func(_ context.Context, dir, name string, args ...string) error {
		*calls = append(append(*calls, dir, name), args...)
		origin, out := args[len(args)-1], args[len(args)-2]
		return os.WriteFile(out, []byte(strings.ReplaceAll(body, "http://127.0.0.1:4173", origin)), 0o644)
	}
}

func TestQARecordLive_RunsCodegenOnThePackOriginAndImports(t *testing.T) {
	t.Parallel()
	project := projectWithPack(t)
	var calls []string

	result, err := record.Live(context.Background(), project,
		record.LiveOptions{ID: "login", AllowPartial: true, Exec: fakeCodegen(&calls, loginCodegen)})

	require.NoError(t, err)
	require.Len(t, calls, 9)
	assert.Equal(t, []string{project, "npx", "playwright", "codegen", "--target", "javascript", "-o"}, calls[:7])
	assert.Equal(t, "http://127.0.0.1:4173", calls[8])
	assert.NoFileExists(t, calls[7], "a recording that imported cleanly is not kept")
	assert.True(t, result.Created)
	assert.Equal(t, 5, result.Steps)
}

func TestQARecordLive_ExplicitOriginNeedsNoPack(t *testing.T) {
	t.Parallel()
	var calls []string
	result, err := record.Live(context.Background(), t.TempDir(), record.LiveOptions{Origin: "http://localhost:3000/",
		ID: "login", Journey: "gui-journey", AllowPartial: true, Exec: fakeCodegen(&calls, loginCodegen)})

	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000", calls[8])
	assert.Equal(t, "http://localhost:3000", result.Origin)

	_, err = record.Live(context.Background(), t.TempDir(), record.LiveOptions{ID: "x", Exec: fakeCodegen(&calls, loginCodegen)})
	code, _ := record.CodeOf(err)
	assert.Equal(t, record.CodeOriginMissing, code, "no pack and no --origin leaves nothing to record against")
}

func TestQARecordLive_MissingNpxIsASetupGap(t *testing.T) {
	t.Parallel()
	missing := func(context.Context, string, string, ...string) error {
		return &exec.Error{Name: "npx", Err: exec.ErrNotFound}
	}
	_, err := record.Live(context.Background(), projectWithPack(t), record.LiveOptions{ID: "login", Exec: missing})

	code, setupGap := record.CodeOf(err)
	assert.Equal(t, record.CodePlaywrightMissing, code)
	assert.True(t, setupGap)
}

func TestQARecordLive_KeepsTheRecordingWhenImportFails(t *testing.T) {
	t.Parallel()
	var calls []string
	result, err := record.Live(context.Background(), projectWithPack(t),
		record.LiveOptions{ID: "login", Exec: fakeCodegen(&calls, loginCodegen)})

	require.Error(t, err)
	code, _ := record.CodeOf(err)
	assert.Equal(t, record.CodeUnsupportedLines, code)
	require.NotEmpty(t, result.Recording)
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(result.Recording)) })
	assert.FileExists(t, result.Recording)
	assert.Contains(t, err.Error(), result.Recording)
}
