package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestRunDoctorJSON_ConfigLoadFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte("mode:\n\tbroken"), 0o644))

	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)

	err := runDoctorJSON(cmd, doctorOptions{dir: dir})
	require.NoError(t, err)

	payload := decodeJSONMap(t, stdout.Bytes())
	assert.Equal(t, string(jsonStatusWarn), payload["status"])

	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	configPayload, ok := data["config"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, configPayload["loaded"])

	warnings, ok := payload["warnings"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, warnings)
}

func TestDoctorJSONReportCollectHookChecks_Configured(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(settingsPath), 0o755))
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{
  "hooks": {
    "SessionStart": [],
    "Stop": []
  },
  "permissions": {
    "allow": ["Read(*)", "Write(*)"]
  }
}`), 0o644))

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectHookChecks(dir)

	assert.Equal(t, jsonStatusOK, report.status)
	require.Len(t, report.checks, 2)
	assert.Equal(t, "doctor.hooks.configured", report.checks[0].ID)
	assert.Equal(t, "pass", report.checks[0].Status)
	assert.Equal(t, "doctor.permissions.allow", report.checks[1].ID)
	assert.Equal(t, "pass", report.checks[1].Status)
}

func TestDoctorJSONReportCollectQualityGateChecks_WarnsOnInvalidPresetAndMissingProviders(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	cfg := config.DefaultFullConfig("doctor-json")
	cfg.Quality.Default = "missing-preset"
	cfg.Spec.ReviewGate.Enabled = true
	cfg.Spec.ReviewGate.Providers = []string{"claude"}
	cfg.Methodology.Mode = "tdd"
	cfg.Methodology.Enforce = true

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectQualityGateChecks(cfg)

	assert.Equal(t, jsonStatusWarn, report.status)
	require.Len(t, report.checks, 5)
	assert.Equal(t, "fail", report.checks[0].Status)
	assert.Equal(t, "pass", report.checks[1].Status)
	assert.Equal(t, "fail", report.checks[2].Status)
	assert.Equal(t, "warn", report.checks[3].Status)
	// A fresh config claims no methodology, so the methodology gate skips.
	assert.Equal(t, "skip", report.checks[4].Status)
}

func TestDoctorJSONReportCollectPlatformChecks_UnknownPlatform(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("doctor-json")
	cfg.Platforms = []string{"mystery-platform"}

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectPlatformChecks(context.Background(), t.TempDir(), cfg)

	assert.Equal(t, jsonStatusWarn, report.status)
	require.Len(t, report.data.Platforms, 1)
	assert.Equal(t, "mystery-platform", report.data.Platforms[0].Name)
	assert.False(t, report.data.Platforms[0].Valid)
	require.Len(t, report.data.Platforms[0].Messages, 1)
	assert.Contains(t, report.data.Platforms[0].Messages[0].Message, "unknown platform")
	require.Len(t, report.checks, 1)
	assert.Equal(t, "fail", report.checks[0].Status)
}

func TestDoctorJSONReportCollectCLIChecks_NoCodingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectCLIChecks()

	assert.Equal(t, jsonStatusWarn, report.status)
	assert.Empty(t, report.data.InstalledCLIs)
	require.Len(t, report.warnings, 1)
	assert.Equal(t, "coding_clis_missing", report.warnings[0].Code)
	require.Len(t, report.checks, 1)
	assert.Equal(t, "doctor.cli.detect", report.checks[0].ID)
	assert.Equal(t, "warn", report.checks[0].Status)
}

func TestCollectDoctorJSONReport_LoadedConfigAggregatesChecks(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("doctor-json")
	require.NoError(t, config.Save(dir, cfg))

	report := collectDoctorJSONReport(&cobra.Command{}, doctorOptions{dir: dir})

	require.NotNil(t, report.data.Config)
	assert.True(t, report.data.Config.Loaded)
	assert.Equal(t, string(cfg.Mode), report.data.Config.Mode)
	assert.NotEmpty(t, report.data.Dependencies)
	assert.NotEmpty(t, report.checks)
	assert.Equal(t, jsonStatusWarn, report.status)
}
