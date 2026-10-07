package cli

// SPEC-PANERM-001 T14 (REQ-13, REQ-14): the doctor report lists the group S
// members that the configured platforms' update retracts, the OpenCode
// scripts a rejected config still names, and user-level handlers as an
// advisory finding that update never edits. S13 (panerm_doctor_legacy_test.go)
// pins the end-to-end set equality with update on the binary O workspaces.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func writeDoctorFixture(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
}

// isolateDoctorEnv pins HOME and PATH to scratch dirs: no host settings file
// or opencode binary is reachable, so OpenCode reads as runtime major 1.
func isolateDoctorEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	return home
}

func doctorConfigFor(platforms ...string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("doctor-fixture")
	cfg.Platforms = platforms
	return cfg
}

const (
	claudeStopScript = ".claude/hooks/autopus/hook-claude-stop.sh"
	geminiStopScript = ".gemini/hooks/autopus/hook-gemini-stop.sh"
	staleTSScript    = ".claude/hooks/autopus/hook-opencode-complete.ts"
)

func TestCollectRetiredOrchestraReport_ListsWhatConfiguredUpdatesRetract(t *testing.T) {
	isolateDoctorEnv(t)
	root := t.TempDir()
	writeDoctorFixture(t, root, map[string]string{
		"autopus.yaml": "features:\n  cc21:\n    monitor_pattern_timeout_ms: 30000\n",
		".claude/settings.json": `{"hooks":{"Stop":[{"matcher":"mixed","hooks":[` +
			`{"type":"command","command":"\"${CLAUDE_PROJECT_DIR:-.}\"/` + claudeStopScript + `"},` +
			`{"type":"command","command":"./scripts/notify.sh"}]}],` +
			`"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"auto react check --quiet"}]}]}}`,
		claudeStopScript: "#!/bin/sh\n",
		".agents/hooks.json": `{"autopus":{"enabled":true,"Stop":[{"type":"command",` +
			`"command":"\"$(cd .. && pwd)/` + geminiStopScript + `\""}]},` +
			`"user-hooks":{"Stop":[{"type":"command","command":"$(pwd)/` + geminiStopScript + `"}]}}`,
		geminiStopScript: "#!/bin/sh\n",
		// codex is not configured, so its update never runs and its leftovers
		// are not listed.
		".codex/hooks.json":                       `{"hooks":{"Stop":[{"hooks":[{"command":".codex/hooks/autopus/hook-codex-stop.sh"}]}]}}`,
		".codex/hooks/autopus/hook-codex-stop.sh": "#!/bin/sh\n",
	})

	report := collectRetiredOrchestraReport(root, doctorConfigFor("claude-code", "antigravity-cli"))

	assert.Equal(t, []string{"features.cc21.monitor_pattern_timeout_ms"}, report.legacyKeys)
	assert.Equal(t, []string{
		".agents/hooks.json Stop " + geminiStopScript,
		".claude/settings.json Stop " + claudeStopScript,
		claudeStopScript,
		geminiStopScript,
	}, report.staleHooks, "handlers first, then script files; the user hook set and codex stay out")
	assert.Empty(t, report.userLevelHooks)
	assert.NoError(t, report.opencodeErr)
}

func TestCollectRetiredOrchestraReport_RejectedOpenCodeConfigStillNamesItsScript(t *testing.T) {
	isolateDoctorEnv(t)
	root := t.TempDir()
	writeDoctorFixture(t, root, map[string]string{
		"autopus.yaml":  "mode: full\n",
		"opencode.json": `{"plugins":[["` + staleTSScript + `",{}]]}`,
		staleTSScript:   "export default {}\n",
	})

	report := collectRetiredOrchestraReport(root, doctorConfigFor("opencode"))

	require.Error(t, report.opencodeErr, "update fails on this config, so doctor carries the rejection")
	assert.Equal(t, []string{"opencode.json plugin " + staleTSScript, staleTSScript}, report.staleHooks)
	assert.Empty(t, report.legacyKeys)
}

func TestRetiredOrchestraChecks_UserLevelHandlersAreAdvisory(t *testing.T) {
	home := isolateDoctorEnv(t)
	writeDoctorFixture(t, home, map[string]string{
		".claude/settings.json": `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command",` +
			`"command":"$HOME/` + claudeStopScript + `","timeout":10}]}]}}`,
	})
	root := t.TempDir()
	writeDoctorFixture(t, root, map[string]string{"autopus.yaml": "mode: full\n"})
	cfg := doctorConfigFor("claude-code")

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectRetiredOrchestraChecks(root, cfg)

	assert.Equal(t, jsonStatusOK, report.status, "a user-level finding never fails the run")
	byID := map[string]jsonCheck{}
	for _, check := range report.checks {
		byID[check.ID] = check
	}
	assert.Equal(t, "pass", byID[legacyOrchestraConfigCheckID].Status)
	assert.Equal(t, "pass", byID[staleCompletionHooksCheckID].Status)
	user := byID[userLevelStaleHooksCheckID]
	assert.Equal(t, "warn", user.Status)
	assert.Equal(t, "user-level stale completion hooks: ~/.claude/settings.json Stop "+claudeStopScript, user.Detail)
	assert.Equal(t, userLevelStaleHooksRemedy, user.Fields["remedy"])

	var text bytes.Buffer
	assert.True(t, checkRetiredOrchestraText(&text, root, cfg))
	assert.Contains(t, text.String(), "~/.claude/settings.json Stop "+claudeStopScript)
	assert.Contains(t, text.String(), "auto update never edits user-level settings")
	assert.NotContains(t, text.String(), "stale completion hooks: .claude", "the project itself is clean")
}

// Claude Code runs .claude/settings.local.json handlers like the shared ones,
// but update never edits that file: its stale handler is advisory, and the
// script it runs stays (pkg/adapter keeps a script a settings file names).
// Before this fix a stale binary deleted the script and doctor said pass while
// every Stop exited 127.
func TestRetiredOrchestraChecks_LocalSettingsHandlersAreAdvisory(t *testing.T) {
	isolateDoctorEnv(t)
	root := t.TempDir()
	writeDoctorFixture(t, root, map[string]string{
		"autopus.yaml": "mode: full\n",
		".claude/settings.local.json": `{"hooks":{"Stop":[{"hooks":[{"type":"command",` +
			`"command":"\"${CLAUDE_PROJECT_DIR:-.}\"/` + claudeStopScript + `","timeout":300}]}]}}`,
	})
	cfg := doctorConfigFor("claude-code")

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectRetiredOrchestraChecks(root, cfg)

	assert.Equal(t, jsonStatusOK, report.status, "a local finding never fails the run")
	byID := map[string]jsonCheck{}
	for _, check := range report.checks {
		byID[check.ID] = check
	}
	assert.Equal(t, "pass", byID[staleCompletionHooksCheckID].Status, "the script is already gone")
	local := byID[localStaleHooksCheckID]
	assert.Equal(t, "warn", local.Status)
	assert.Equal(t, "local stale completion hooks: .claude/settings.local.json Stop "+claudeStopScript, local.Detail)
	assert.Equal(t, localStaleHooksRemedy, local.Fields["remedy"])

	var text bytes.Buffer
	assert.True(t, checkRetiredOrchestraText(&text, root, cfg))
	assert.Contains(t, text.String(), ".claude/settings.local.json Stop "+claudeStopScript)
}

func TestRetiredOrchestraChecks_ProjectFindingsWarnWithTheUpdateRemedy(t *testing.T) {
	isolateDoctorEnv(t)
	root := t.TempDir()
	writeDoctorFixture(t, root, map[string]string{
		"autopus.yaml":   "orchestra:\n  subprocess:\n    enabled: true\n",
		claudeStopScript: "#!/bin/sh\n",
	})
	cfg := doctorConfigFor("claude-code")

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectRetiredOrchestraChecks(root, cfg)

	assert.Equal(t, jsonStatusWarn, report.status)
	require.Len(t, report.checks, 2, "no user-level check without a user-level finding")
	assert.Equal(t, jsonCheck{ID: legacyOrchestraConfigCheckID, Severity: "warning", Status: "warn",
		Detail: "legacy orchestra keys: orchestra.subprocess.enabled",
		Fields: map[string]string{"remedy": `run "auto update"`}}, report.checks[0])
	assert.Equal(t, "stale completion hooks: "+claudeStopScript, report.checks[1].Detail)

	var text bytes.Buffer
	assert.False(t, checkRetiredOrchestraText(&text, root, cfg))
	assert.Equal(t, 2, strings.Count(text.String(), `remedy: run "auto update"`))
}
