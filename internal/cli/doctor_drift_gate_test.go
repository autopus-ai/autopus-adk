package cli

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

func hasCheck(checks []jsonCheck, id string) bool {
	for _, c := range checks {
		if c.ID == id {
			return true
		}
	}
	return false
}

func getCheck(checks []jsonCheck, id string) (jsonCheck, bool) {
	for _, c := range checks {
		if c.ID == id {
			return c, true
		}
	}
	return jsonCheck{}, false
}

// driftInstallWithContentAndOrphan builds a claude-code install that has both a
// tampered deterministic file (content drift) and an unconfigured gemini-cli
// manifest (orphan), so a single fixture exercises two concurrent drift signals.
func driftInstallWithContentAndOrphan(t *testing.T) (string, *config.HarnessConfig) {
	t.Helper()
	dir := t.TempDir()
	cfg := claudeDriftConfig()
	generateClaudeInstall(t, dir, cfg)

	wf := filepath.Join(dir, ".claude", "agents", "autopus", "frontend-specialist.md")
	data, err := os.ReadFile(wf)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(wf,
		[]byte(strings.Replace(string(data), "claude-sonnet-5", "claude-sonnet-4-6", 1)), 0o644))

	writeManifestFile(t, dir, "gemini-cli-manifest.json")
	return dir, cfg
}

// TestCollectDriftGateChecks_Advisory_StatusUnchanged is the S7 oracle: with
// content drift and an orphan manifest both present, both drift checks report
// warn yet the envelope status stays OK, so overall_ok remains true. This
// isolates the advisory invariant from unrelated doctor checks.
func TestCollectDriftGateChecks_Advisory_StatusUnchanged(t *testing.T) {
	dir, cfg := driftInstallWithContentAndOrphan(t)

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectDriftGateChecks(dir, cfg)

	content, ok := getCheck(report.checks, "doctor.drift.content.claude-code")
	require.True(t, ok, "content drift check present")
	assert.Equal(t, "warn", content.Status)

	orphan, ok := getCheck(report.checks, "doctor.drift.orphan_manifest")
	require.True(t, ok, "orphan manifest check present")
	assert.Equal(t, "warn", orphan.Status)

	// Advisory: the warn checks must not flip the envelope status. overall_ok is
	// derived as (status == ok), so it stays true.
	assert.Equal(t, jsonStatusOK, report.status, "drift is advisory; envelope stays OK")
}

// TestDriftGate_JSONTextParity confirms the text and JSON surfaces mirror one
// another: when JSON emits content and orphan warn checks, the text Drift section
// names the same platform and orphan condition.
func TestDriftGate_JSONTextParity(t *testing.T) {
	dir, cfg := driftInstallWithContentAndOrphan(t)

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectDriftGateChecks(dir, cfg)
	require.True(t, hasCheck(report.checks, "doctor.drift.content.claude-code"))
	require.True(t, hasCheck(report.checks, "doctor.drift.orphan_manifest"))

	var buf bytes.Buffer
	renderDriftText(&buf, dir, cfg)
	out := buf.String()

	assert.Contains(t, out, "Drift", "text renders the Drift section header")
	assert.Contains(t, out, "claude-code", "text names the drifting platform")
	assert.Contains(t, out, "orphan manifest", "text names the orphan condition")
}

// TestRenderDriftText_CleanInstall_NoSection verifies the text section is silent
// when there is no drift subject: a bare dir with no manifests and no source repo
// emits no Drift section.
func TestRenderDriftText_CleanInstall_NoSection(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.HarnessConfig{Platforms: []string{"claude-code"}}
	renderDriftText(&buf, t.TempDir(), cfg)
	assert.Empty(t, buf.String(), "no manifests and no source repo → silent skip")
}
