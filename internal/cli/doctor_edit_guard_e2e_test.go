package cli_test

// SPEC-EDITGUARD-001 S15, doctor half, through the real commands: after
// `auto init` for every platform with a matrix lane, `auto doctor` reads each
// generated hook surface and prints the registration state and the matrix
// state per installed platform, and `--json` carries the same rows as checks.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Not parallel: t.Setenv pins CODEX_HOME so Codex generation stays in the
// test's own directory.
func TestDoctorCmd_ReportsEditGuardRegistrationPerInstalledPlatform(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "codex-home"))
	runEditGuardCLI(t, "init", "--dir", dir, "--project", "guard",
		"--platforms", "claude-code,codex,opencode,antigravity-cli,omp")

	var text bytes.Buffer
	doctor := newTestRootCmd()
	doctor.SetOut(&text)
	doctor.SetArgs([]string{"doctor", "--dir", dir})
	require.NoError(t, doctor.Execute())
	for _, line := range []string{
		"Claude Code: guard registered in .claude/settings.json (matrix: enforced)",
		"Codex: guard registered in .codex/hooks.json (matrix: enforced)",
		"OpenCode: guard registered in .opencode/plugins/autopus-hooks.js (matrix: enforced)",
		"Gemini CLI: guard registered in .gemini/settings.json (matrix: enforced)",
		"Antigravity: guard not registered by design (matrix: advisory-only)",
		"OMP: guard not registered by design (matrix: none)",
	} {
		assert.Contains(t, text.String(), line)
	}

	var raw bytes.Buffer
	doctorJSON := newTestRootCmd()
	doctorJSON.SetOut(&raw)
	doctorJSON.SetArgs([]string{"doctor", "--dir", dir, "--json"})
	require.NoError(t, doctorJSON.Execute())
	var envelope struct {
		Checks []struct {
			ID     string            `json:"id"`
			Status string            `json:"status"`
			Fields map[string]string `json:"fields"`
		} `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(raw.Bytes(), &envelope))
	got := map[string]string{}
	for _, check := range envelope.Checks {
		if strings.HasPrefix(check.ID, "doctor.edit_guard.") {
			got[check.ID] = check.Status + " " + check.Fields["matrix_state"] + " registered=" + check.Fields["registered"]
		}
	}
	assert.Equal(t, map[string]string{
		"doctor.edit_guard.claude-code":     "pass enforced registered=true",
		"doctor.edit_guard.codex":           "pass enforced registered=true",
		"doctor.edit_guard.opencode":        "pass enforced registered=true",
		"doctor.edit_guard.gemini":          "pass enforced registered=true",
		"doctor.edit_guard.antigravity-cli": "skip advisory-only registered=false",
		"doctor.edit_guard.omp":             "skip none registered=false",
	}, got)
}
