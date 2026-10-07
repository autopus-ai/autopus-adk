package cli

// SPEC-EDITGUARD-001 REQ-EG-23 and S15: `auto doctor` reports, per installed
// platform, whether the edit guard is registered and the platform's state in
// the enforcement matrix that docs/edit-guard.md publishes.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	claudeGuardSettings = `{"hooks":{"PreToolUse":[{"matcher":"Edit|Write|MultiEdit","hooks":[{"type":"command",` +
		`"command":"out=$(auto guard edit --platform claude-code) && [ -n \"$out\" ] && printf '%s\\n' \"$out\"; exit 0",` +
		`"timeout":5}]}]}}`
	geminiGuardSettings = `{"hooks":{"BeforeTool":[{"matcher":"^(write_file|replace)$","hooks":[{"type":"command",` +
		`"command":"out=$(auto guard edit --platform gemini) && [ -n \"$out\" ] && printf '%s\\n' \"$out\"; exit 0",` +
		`"timeout":5000}]}]}}`
	// codexUserHooks has a user handler and a line that only mentions the
	// guard, so neither counts as the registration.
	codexUserHooks = `{"hooks":{"PreToolUse":[{"matcher":"apply_patch","hooks":[` +
		`{"type":"command","command":"./user-patch-check.sh"},` +
		`{"type":"command","command":"echo out=$(auto guard edit --platform codex)"}]}]}}`
	openCodeGuardPlugin = "export const AutopusHooks = {}\n" +
		`const EDIT_GUARD = {"command":"auto","args":["guard","edit","--platform","opencode"],"timeout":5,` +
		`"tools":["edit","write","patch"],"patchTools":["patch"]}` + "\n"
	openCodeNoGuardPlugin = "const EDIT_GUARD = null\n"
)

type editGuardRow struct{ id, status, detail string }

func writeEditGuardSurface(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func editGuardRows(diagnoses []editGuardDiagnosis) []editGuardRow {
	rows := make([]editGuardRow, 0, len(diagnoses))
	for _, d := range diagnoses {
		rows = append(rows, editGuardRow{d.id(), d.status, d.detail})
	}
	return rows
}

func editGuardConfig(enabled *bool, platforms ...string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("guard")
	cfg.Platforms = platforms
	cfg.Hooks.EditGuard = enabled
	return cfg
}

// Every installed platform gets a row per lane its adapter generates, in
// autopus.yaml order: antigravity-cli writes the Gemini CLI settings and the
// advisory-only Antigravity hooks, and a platform outside the matrix is none.
func TestDiagnoseEditGuard_ReportsEachInstalledPlatformLane(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEditGuardSurface(t, dir, ".claude/settings.json", claudeGuardSettings)
	writeEditGuardSurface(t, dir, ".codex/hooks.json", codexUserHooks)
	writeEditGuardSurface(t, dir, ".gemini/settings.json", geminiGuardSettings)
	writeEditGuardSurface(t, dir, ".opencode/plugins/autopus-hooks.js", openCodeGuardPlugin)
	cfg := editGuardConfig(nil, "claude-code", "codex", "opencode", "antigravity-cli", "omp", "cursor")

	assert.Equal(t, []editGuardRow{
		{"doctor.edit_guard.claude-code", "pass", "Claude Code: guard registered in .claude/settings.json (matrix: enforced)"},
		{"doctor.edit_guard.codex", "warn", "Codex: guard not registered in .codex/hooks.json; run 'auto update' (matrix: enforced)"},
		{"doctor.edit_guard.opencode", "pass",
			"OpenCode: guard registered in .opencode/plugins/autopus-hooks.js (matrix: enforced)"},
		{"doctor.edit_guard.gemini", "pass", "Gemini CLI: guard registered in .gemini/settings.json (matrix: enforced)"},
		{"doctor.edit_guard.antigravity-cli", "skip", "Antigravity: guard not registered by design (matrix: advisory-only)"},
		{"doctor.edit_guard.omp", "skip", "OMP: guard not registered by design (matrix: none)"},
		{"doctor.edit_guard.cursor", "skip", "cursor: guard not registered, no edit-guard lane (matrix: none)"},
	}, editGuardRows(diagnoseEditGuard(dir, cfg)))
}

// With hooks.edit_guard false a missing guard is the expected state, while a
// guard that is still registered needs an update to be retracted. A missing
// hook file reads as no registration.
func TestDiagnoseEditGuard_FollowsTheFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEditGuardSurface(t, dir, ".claude/settings.json", claudeGuardSettings)
	writeEditGuardSurface(t, dir, ".opencode/plugins/autopus-hooks.js", openCodeNoGuardPlugin)
	cfg := editGuardConfig(new(false), "claude-code", "opencode", "codex")

	assert.Equal(t, []editGuardRow{
		{"doctor.edit_guard.claude-code", "warn", "Claude Code: guard still registered in .claude/settings.json " +
			"although hooks.edit_guard is false; run 'auto update' (matrix: enforced)"},
		{"doctor.edit_guard.opencode", "skip", "OpenCode: guard not registered, hooks.edit_guard is false (matrix: enforced)"},
		{"doctor.edit_guard.codex", "skip", "Codex: guard not registered, hooks.edit_guard is false (matrix: enforced)"},
	}, editGuardRows(diagnoseEditGuard(dir, cfg)))

	enabled := editGuardRows(diagnoseEditGuard(dir, editGuardConfig(new(true), "opencode", "codex")))
	assert.Equal(t, []editGuardRow{
		{"doctor.edit_guard.opencode", "warn",
			"OpenCode: guard not registered in .opencode/plugins/autopus-hooks.js; run 'auto update' (matrix: enforced)"},
		{"doctor.edit_guard.codex", "warn", "Codex: guard not registered in .codex/hooks.json; run 'auto update' (matrix: enforced)"},
	}, enabled)
}

// The OpenCode row follows the plugin API of the plugin the adapter generated:
// the V1 plugin carries the guard, but no 1.x host was probed, so the doctor
// does not call it enforced.
func TestDiagnoseEditGuard_OpenCodeRowFollowsTheGeneratedPluginAPI(t *testing.T) {
	t.Parallel()

	cfg := editGuardConfig(nil, "opencode")
	for version, want := range map[string]editGuardRow{
		"1.14.0": {"doctor.edit_guard.opencode-v1", "skip", "OpenCode 1.x: guard registered in " +
			".opencode/plugins/autopus-hooks.js, but no probe confirmed that this host blocks a denied edit " +
			"(matrix: host-unverified)"},
		"2.0.10": {"doctor.edit_guard.opencode", "pass",
			"OpenCode: guard registered in .opencode/plugins/autopus-hooks.js (matrix: enforced)"},
	} {
		dir := t.TempDir()
		_, err := opencode.NewWithRoot(dir, opencode.WithCLIVersion(version)).Generate(context.Background(), cfg)
		require.NoError(t, err, version)
		assert.Equal(t, []editGuardRow{want}, editGuardRows(diagnoseEditGuard(dir, cfg)), version)
	}
}

// A surface the doctor cannot parse is reported as unreadable rather than as
// registered or missing, and a guard wired for another platform's dialect is
// not this lane's registration.
func TestDiagnoseEditGuard_ReportsSurfacesItCannotRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEditGuardSurface(t, dir, ".claude/settings.json", `{`)
	writeEditGuardSurface(t, dir, ".opencode/plugins/autopus-hooks.js", "const EDIT_GUARD = {oops\n")
	writeEditGuardSurface(t, dir, ".codex/hooks.json", `{"hooks":[]}`)
	writeEditGuardSurface(t, dir, ".gemini/settings.json", claudeGuardSettings)

	rows := editGuardRows(diagnoseEditGuard(dir, editGuardConfig(nil, "claude-code", "opencode", "codex", "antigravity-cli")))
	require.Len(t, rows, 5)
	for i, prefix := range []string{
		"Claude Code: cannot read .claude/settings.json: ",
		"OpenCode: cannot read .opencode/plugins/autopus-hooks.js: ",
		"Codex: cannot read .codex/hooks.json: ",
	} {
		assert.Equal(t, "warn", rows[i].status, rows[i].detail)
		assert.Regexp(t, `^`+regexp.QuoteMeta(prefix)+`.+ \(matrix: enforced\)$`, rows[i].detail)
	}
	assert.Equal(t, editGuardRow{"doctor.edit_guard.gemini", "warn",
		"Gemini CLI: guard not registered in .gemini/settings.json; run 'auto update' (matrix: enforced)"}, rows[3])
}

// The JSON report carries one check per row with structured fields, and a warn
// row marks the report warn with a coded warning; the text report prints the
// same detail under its own section.
func TestDoctorEditGuard_JSONChecksAndTextSection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEditGuardSurface(t, dir, ".claude/settings.json", claudeGuardSettings)
	cfg := editGuardConfig(nil, "claude-code", "codex", "omp")

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectEditGuardChecks(dir, cfg)
	require.Len(t, report.checks, 3)
	assert.Equal(t, jsonCheck{ID: "doctor.edit_guard.claude-code", Severity: "info", Status: "pass",
		Detail: "Claude Code: guard registered in .claude/settings.json (matrix: enforced)",
		Fields: map[string]string{"platform": "claude-code", "lane": "claude-code", "matrix_state": "enforced",
			"registered": "true", "surface": ".claude/settings.json", "edit_guard": "enabled"}}, report.checks[0])
	assert.Equal(t, "warning", report.checks[1].Severity)
	assert.Equal(t, map[string]string{"platform": "omp", "lane": "omp", "matrix_state": "none",
		"registered": "false", "edit_guard": "enabled"}, report.checks[2].Fields)
	assert.Equal(t, jsonStatusWarn, report.status)
	assert.Equal(t, []jsonMessage{{Code: "edit_guard_not_registered",
		Message: "Codex: guard not registered in .codex/hooks.json; run 'auto update' (matrix: enforced)"}}, report.warnings)

	var out bytes.Buffer
	checkEditGuardText(&out, dir, cfg)
	assert.Contains(t, out.String(), "Edit Guard")
	assert.Contains(t, out.String(), "Claude Code: guard registered in .claude/settings.json (matrix: enforced)")
	assert.Contains(t, out.String(), "OMP: guard not registered by design (matrix: none)")
}
