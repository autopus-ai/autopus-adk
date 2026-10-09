package evidence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestValidate_AcceptsBrowserFailure(t *testing.T) {
	t.Parallel()

	manifest := fixtureManifest(t, "browser", "failed")

	require.NoError(t, manifest.Validate())
}

func TestManifestValidate_AcceptsDesktopMappedBundle(t *testing.T) {
	t.Parallel()

	manifest := fixtureManifest(t, "desktop", "passed")

	require.NoError(t, manifest.Validate())
}

func TestManifestValidate_RejectsInvalidSurfaceAndMissingRequiredFields(t *testing.T) {
	t.Parallel()

	invalidSurface := fixtureManifest(t, "mobile", "failed")
	require.ErrorContains(t, invalidSurface.Validate(), "unsupported surface")

	missingID := fixtureManifest(t, "browser", "failed")
	missingID.QAResultID = ""
	require.ErrorContains(t, missingID.Validate(), "qa_result_id")
}

func TestResolveArtifactPaths_RejectsPathsOutsideInputRoot(t *testing.T) {
	t.Parallel()

	inputRoot := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"access_token":"`+fakeProjectKey+`"}`), 0o644))
	manifest := fixtureManifest(t, "browser", "failed")
	manifest.Artifacts = []ArtifactRef{{
		Kind:        "console",
		Path:        outside,
		Publishable: true,
		Redaction:   "text_redacted_and_scanned",
	}}

	_, err := ResolveArtifactPaths(manifest, inputRoot)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "qameshfake")
}

func TestWriteFinalManifest_RejectsUnsafeArtifactBeforePublication(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	raw := filepath.Join(dir, "trace.zip")
	require.NoError(t, os.WriteFile(raw, []byte("Authorization: Bearer "+fakeProjectKey), 0o644))
	manifest := fixtureManifest(t, "browser", "failed")
	manifest.Artifacts[0] = ArtifactRef{
		Kind:        "trace_sanitized",
		Path:        raw,
		Publishable: true,
		Redaction:   "text_redacted_and_scanned",
	}

	_, err := WriteFinalManifest(manifest, filepath.Join(dir, "final"))
	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(dir, "final"))
}

func TestWriteFinalManifest_CopiesNonPublishableQuarantineSummary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	summary := filepath.Join(dir, "screenshot-quarantined.json")
	require.NoError(t, os.WriteFile(summary, []byte(`{"publishable":false}`), 0o644))
	manifest := fixtureManifest(t, "browser", "failed")
	replaceArtifactPath(&manifest, "screenshot_quarantined", summary)

	manifestPath, err := WriteFinalManifest(manifest, filepath.Join(dir, "final"))

	require.NoError(t, err)
	body, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "artifacts/screenshot_quarantined/screenshot-quarantined.json")
	assert.FileExists(t, filepath.Join(dir, "final", "artifacts", "screenshot_quarantined", "screenshot-quarantined.json"))
}

func TestManifestValidate_RejectsBrowserGoldenMissingArtifactContract(t *testing.T) {
	t.Parallel()

	manifest := fixtureManifest(t, "browser", "failed")
	manifest.Artifacts = []ArtifactRef{manifest.Artifacts[0]}

	require.ErrorContains(t, manifest.Validate(), "screenshot_masked")
}

func TestManifestValidate_RejectsBrowserGoldenArtifactSuffixDrift(t *testing.T) {
	t.Parallel()

	manifest := fixtureManifest(t, "browser", "failed")
	replaceArtifactPath(&manifest, "a11y_snapshot", filepath.Join(t.TempDir(), "a11y.txt"))

	require.ErrorContains(t, manifest.Validate(), "a11y_snapshot")
}

func TestManifestValidate_RejectsDesktopMissingSourceAndCategories(t *testing.T) {
	t.Parallel()

	manifest := fixtureManifest(t, "desktop", "failed")
	manifest.SourceRefs.SourceSpec = "SPEC-QAMESH-001"
	require.ErrorContains(t, manifest.Validate(), "SPEC-DESKTOP-017")

	manifest = fixtureManifest(t, "desktop", "failed")
	manifest.OracleResults.Desktop.TimeoutClassification = ""
	require.ErrorContains(t, manifest.Validate(), "timeout_classification")

	manifest = fixtureManifest(t, "desktop", "failed")
	manifest.Artifacts = manifest.Artifacts[:1]
	require.ErrorContains(t, manifest.Validate(), "app_log")
}

func TestWriteFinalManifest_DoesNotLeavePartialOutputWhenLaterArtifactFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	raw := filepath.Join(dir, "screenshot.zip")
	require.NoError(t, os.WriteFile(raw, []byte("binary screenshot"), 0o644))
	manifest := fixtureManifest(t, "browser", "failed")
	replaceArtifactPath(&manifest, "screenshot_quarantined", raw)
	output := filepath.Join(dir, "final")

	_, err := WriteFinalManifest(manifest, output)

	require.Error(t, err)
	assert.NoDirExists(t, output)
}

func TestNormalizeManifest_A11yCriticalFailureWinsOverRunnerStatus(t *testing.T) {
	t.Parallel()

	manifest := fixtureManifest(t, "browser", "passed")
	manifest.OracleResults.A11y.CriticalCount = 1

	normalized := NormalizeManifest(manifest)

	assert.Equal(t, "failed", normalized.Status)
}

func TestValidateLocatorConvention_RequiresFallbackReasonForCSS(t *testing.T) {
	t.Parallel()

	result := ValidateLocatorConvention([]LocatorContract{
		{Strategy: "role", Value: `button[name="Continue"]`},
		{Strategy: "label", Value: "Email"},
		{Strategy: "css", Value: "#app > div:nth-child(2) > button"},
	})

	assert.ElementsMatch(t, []string{"role", "label"}, result.Accepted)
	assert.Contains(t, result.Rejected, "css")
	assert.True(t, result.FallbackReasonRequired)
	assert.True(t, result.StableTestIDRequired)
}

func fixtureManifest(t *testing.T, surface, status string) Manifest {
	t.Helper()
	dir := t.TempDir()
	writeArtifact := func(name, body string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		return path
	}
	artifacts := []ArtifactRef{
		{Kind: "trace_summary", Path: writeArtifact("trace-summary.json", `{"trace_mode":"summary_only"}`), Publishable: true, Redaction: "text_redacted_and_scanned"},
		{Kind: "screenshot_quarantined", Path: writeArtifact("screenshot-quarantined.json", `{"publishable":false}`), Publishable: false, Redaction: "local_only_quarantine_ref"},
		{Kind: "console", Path: writeArtifact("console.json", `{"messages":["ok"]}`), Publishable: true, Redaction: "text_redacted_and_scanned"},
		{Kind: "network_summary", Path: writeArtifact("network-summary.json", `{"events":[]}`), Publishable: true, Redaction: "text_redacted_and_scanned"},
		{Kind: "a11y_snapshot", Path: writeArtifact("a11y-snapshot.aria.yml", "- main\n"), Publishable: true, Redaction: "text_redacted_and_scanned"},
		{Kind: "oracle_summary", Path: writeArtifact("oracle-summary.json", `{"critical_count":1}`), Publishable: true, Redaction: "text_redacted_and_scanned"},
	}
	oracleResults := OracleResults{
		A11y: &A11yOracle{CriticalCount: 1, FailedTargets: []string{`button[name="Pay"]`}},
	}
	sourceRefs := SourceRefs{
		SourceSpec:       "SPEC-QAMESH-001",
		AcceptanceRefs:   []string{"AC-QAMESH-001", "AC-QAMESH-003"},
		OwnedPaths:       []string{"sample-app/frontend"},
		DoNotModifyPaths: []string{".codex/**"},
	}
	scenarioRef := "browser:login"
	runner := Runner{Name: "playwright", Command: "npx playwright test e2e/tests/qamesh-golden.spec.ts --project=chromium"}
	if surface == "desktop" {
		artifacts = []ArtifactRef{
			{Kind: "screenshot", Path: writeArtifact("screenshots/failure.json", `{"ref":"failure.png"}`), Publishable: true, Redaction: "binary_retained_explicit_safe_mode"},
			{Kind: "app_log", Path: writeArtifact("logs/app.log", "desktop app log\n"), Publishable: true, Redaction: "text_redacted_and_scanned"},
			{Kind: "driver_log", Path: writeArtifact("logs/driver.log", "driver log\n"), Publishable: true, Redaction: "text_redacted_and_scanned"},
			{Kind: "command_output", Path: writeArtifact("commands/smoke.stdout.log", "stdout\n"), Publishable: true, Redaction: "text_redacted_and_scanned"},
		}
		oracleResults = OracleResults{Desktop: &DesktopOracle{TimeoutClassification: "driver_timeout"}}
		sourceRefs = SourceRefs{
			SourceSpec:       "SPEC-DESKTOP-017",
			AcceptanceRefs:   []string{"AC-QAMESH-004"},
			OwnedPaths:       []string{"sample-desktop"},
			DoNotModifyPaths: []string{".codex/**"},
		}
		scenarioRef = "desktop:macos-smoke"
		runner = Runner{Name: "appium-mac2", Command: "npm --prefix e2e-macos test"}
	}

	return Manifest{
		SchemaVersion:       SchemaVersion,
		QAResultID:          "qa-browser-login-001",
		Surface:             surface,
		Lane:                "golden",
		ScenarioRef:         scenarioRef,
		Runner:              runner,
		Status:              status,
		StartedAt:           "2026-05-02T00:00:00Z",
		EndedAt:             "2026-05-02T00:00:01Z",
		DurationMS:          1000,
		RetentionClass:      "local-redacted",
		ReproductionCommand: "PLAYWRIGHT_SKIP_GLOBAL_SETUP=true npx playwright test e2e/tests/qamesh-golden.spec.ts --project=chromium",
		Artifacts:           artifacts,
		OracleResults:       oracleResults,
		RedactionStatus:     RedactionStatus{Status: "passed"},
		SourceRefs:          sourceRefs,
	}
}

func replaceArtifactPath(manifest *Manifest, kind, path string) {
	for index, artifact := range manifest.Artifacts {
		if artifact.Kind == kind {
			manifest.Artifacts[index].Path = path
			return
		}
	}
}
