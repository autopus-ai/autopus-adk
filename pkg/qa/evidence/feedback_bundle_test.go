package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A bundle is handed to another agent, so a consumer holding only the bundle
// directory must be able to open every artifact the bundle references.
func TestWriteFeedbackBundle_EveryReferencedArtifactResolvesInsideBundle(t *testing.T) {
	t.Parallel()

	manifest := publishedFailedManifest(t, "console", "console failure detail\n")

	result, err := WriteFeedbackBundle(manifest, "codex", filepath.Join(t.TempDir(), "out"))

	require.NoError(t, err)
	entries := bundleArtifactEntries(t, result.BundlePath)
	require.NotEmpty(t, entries)
	bundled := 0
	for _, entry := range entries {
		path, ok := entry["path"].(string)
		if !ok {
			assert.Equal(t, false, entry["bundled"], "an entry without a path must not claim to be bundled")
			assert.NotEmpty(t, entry["withheld"], "a withheld artifact must say why")
			continue
		}
		assert.Equal(t, true, entry["bundled"])
		assert.FileExists(t, filepath.Join(result.BundlePath, filepath.FromSlash(path)))
		bundled++
	}
	assert.Positive(t, bundled, "publishable text artifacts must travel with the bundle")
}

func TestWriteFeedbackBundle_LeavesLocalOnlyArtifactAsMetadataOnly(t *testing.T) {
	t.Parallel()

	manifest := publishedFailedManifest(t, "console", "console failure detail\n")

	result, err := WriteFeedbackBundle(manifest, "codex", filepath.Join(t.TempDir(), "out"))

	require.NoError(t, err)
	entry := bundleArtifactEntry(t, result.BundlePath, "screenshot_quarantined")
	assert.Equal(t, false, entry["bundled"])
	assert.Equal(t, withheldLocalOnly, entry["withheld"])
	assert.NotContains(t, entry, "path")
	assert.NoDirExists(t, filepath.Join(result.BundlePath, bundleArtifactsDir, "screenshot_quarantined"))
	assert.NoDirExists(t, filepath.Join(result.BundlePath, bundleArtifactsDir, "screenshot-quarantined"))
}

// Without the recorded output a repair agent has to re-run the reproduction
// command just to learn which assertion failed.
func TestWriteFeedbackBundle_PromptQuotesRedactedFailureOutput(t *testing.T) {
	t.Parallel()

	body := "main_test.go:9: Add(2,3) = 5, want 6\nAWS_SECRET_ACCESS_KEY=" + fakeAWSSecretKey + "\ndb=postgres://admin:hunter2@db.internal:5432/prod\n"
	manifest := publishedFailedManifest(t, "console", body)

	result, err := WriteFeedbackBundle(manifest, "codex", filepath.Join(t.TempDir(), "out"))

	require.NoError(t, err)
	prompt := readFile(t, result.PromptPath)
	assert.Contains(t, prompt, "## Recorded Failure Output")
	assert.Contains(t, prompt, "main_test.go:9: Add(2,3) = 5, want 6")
	assert.Contains(t, prompt, RedactedSecret)
	assert.NotContains(t, prompt, fakeAWSSecretKey)
	assert.NotContains(t, prompt, "hunter2")
}

func TestWriteFeedbackBundle_RendersRealPerTargetFacts(t *testing.T) {
	t.Parallel()

	for flag, target := range supportedFeedbackTargets {
		t.Run(flag, func(t *testing.T) {
			manifest := publishedFailedManifest(t, "console", "detail\n")

			result, err := WriteFeedbackBundle(manifest, flag, filepath.Join(t.TempDir(), "out"))

			require.NoError(t, err)
			prompt := readFile(t, result.PromptPath)
			assert.Contains(t, prompt, "--to "+flag)
			assert.Contains(t, prompt, target.Adapter)
			assert.Contains(t, prompt, target.CLIBinary)
			assert.Contains(t, prompt, target.InstructionDoc)
			metadata := bundleMetadata(t, result.BundlePath)
			assert.Equal(t, target.Adapter, metadata["target_adapter"])
			assert.Equal(t, target.CLIBinary, metadata["target_cli"])
			assert.Equal(t, target.InstructionDoc, metadata["target_instructions"])
		})
	}
}

// The gemini target used to render a title that never mentioned Gemini.
func TestWriteFeedbackBundle_GeminiTargetNamesItself(t *testing.T) {
	t.Parallel()

	manifest := publishedFailedManifest(t, "console", "detail\n")

	result, err := WriteFeedbackBundle(manifest, "gemini", filepath.Join(t.TempDir(), "out"))

	require.NoError(t, err)
	assert.Contains(t, readFile(t, result.PromptPath), "Gemini")
}

func TestWriteFeedbackBundle_RejectsArtifactOutsideManifestDirectory(t *testing.T) {
	t.Parallel()

	manifest := publishedFailedManifest(t, "console", "detail\n")
	outside := filepath.Join(t.TempDir(), "outside.log")
	require.NoError(t, os.WriteFile(outside, []byte("escaped\n"), 0o644))
	for index := range manifest.Artifacts {
		if manifest.Artifacts[index].Kind == "console" {
			manifest.Artifacts[index].Path = outside
		}
	}

	result, err := WriteFeedbackBundle(manifest, "codex", filepath.Join(t.TempDir(), "out"))

	require.NoError(t, err)
	entry := bundleArtifactEntry(t, result.BundlePath, "console")
	assert.Equal(t, false, entry["bundled"])
	assert.Equal(t, withheldUnresolved, entry["withheld"])
}

func TestNormalizeManifest_PreservesSourceDirAnchor(t *testing.T) {
	t.Parallel()

	manifest := publishedFailedManifest(t, "console", "detail\n")
	require.NotEmpty(t, manifest.sourceDir)

	assert.Equal(t, manifest.sourceDir, NormalizeManifest(manifest).sourceDir)
}

// publishedFailedManifest walks the real publish path: write final evidence,
// then load it back the way the CLI and the runner do, so artifact paths are
// manifest-relative and the anchor is whatever LoadManifest recorded.
func publishedFailedManifest(t *testing.T, failingKind, failingBody string) Manifest {
	t.Helper()
	manifest := fixtureManifest(t, "browser", "failed")
	manifest.OracleResults.Checks = []CheckResult{{
		ID: "console-clean", Type: "deterministic", Status: "failed",
		Expected: "exit_code=0", Actual: "exit_code=1",
		FailureSummary: "expected exit_code=0, got 1",
		ArtifactRefs:   []string{failingKind},
	}}
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind == failingKind {
			require.NoError(t, os.WriteFile(artifact.Path, []byte(failingBody), 0o644))
		}
	}
	manifestPath, err := WriteFinalManifest(manifest, filepath.Join(t.TempDir(), "evidence"))
	require.NoError(t, err)
	loaded, err := LoadManifest(manifestPath)
	require.NoError(t, err)
	return loaded
}

func bundleMetadata(t *testing.T, bundlePath string) map[string]any {
	t.Helper()
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(readFile(t, filepath.Join(bundlePath, "bundle.json"))), &metadata))
	return metadata
}

func bundleArtifactEntries(t *testing.T, bundlePath string) []map[string]any {
	t.Helper()
	raw, ok := bundleMetadata(t, bundlePath)["evidence_artifacts"].([]any)
	require.True(t, ok, "bundle.json must list evidence artifacts")
	entries := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		require.True(t, ok)
		entries = append(entries, entry)
	}
	return entries
}

func bundleArtifactEntry(t *testing.T, bundlePath, kind string) map[string]any {
	t.Helper()
	for _, entry := range bundleArtifactEntries(t, bundlePath) {
		if entry["kind"] == kind {
			return entry
		}
	}
	t.Fatalf("bundle.json has no %s artifact entry", kind)
	return nil
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}
