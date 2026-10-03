package evidence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	traceSentinel      = "TRACE-ZIP-BYTES-0xA1"
	screenshotSentinel = "PNG-PIXEL-BYTES-0xB2"
)

func writeUnder(t *testing.T, root, rel string, body []byte) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, body, 0o644))
	return path
}

// replayManifest is a failed browser manifest whose artifacts live under
// projectDir, plus the binary captures a Playwright run leaves behind and one
// capture that was pruned before the bundle was written.
func replayManifest(t *testing.T, projectDir string) Manifest {
	t.Helper()
	manifest := fixtureManifest(t, "browser", "failed")
	for i, artifact := range manifest.Artifacts {
		body, err := os.ReadFile(artifact.Path)
		require.NoError(t, err)
		manifest.Artifacts[i].Path = writeUnder(t, projectDir, "test-results/login/"+filepath.Base(artifact.Path), body)
	}
	manifest.Artifacts = append(manifest.Artifacts,
		ArtifactRef{Kind: "trace", Path: writeUnder(t, projectDir, "test-results/login/trace.zip", []byte("PK\x03\x04"+traceSentinel)), Redaction: "local_only_quarantine_ref"},
		ArtifactRef{Kind: "screenshot", Path: writeUnder(t, projectDir, "test-results/login/test-failed-1.png", []byte("\x89PNG\r\n\x1a\n"+screenshotSentinel)), Redaction: "local_only_quarantine_ref"},
		ArtifactRef{Kind: "trace", Path: filepath.Join(projectDir, "test-results", "login", "pruned-trace.zip"), Redaction: "local_only_quarantine_ref"},
	)
	return manifest
}

func loginReplay(projectDir string) *Replay {
	return &Replay{
		ScenarioID: "login", SpecPath: "e2e/autopus-generated/login.spec.ts", Line: 6,
		Screen: "login", Index: 1, Kind: "action", Ac: "AC-LOGIN-001",
		Excerpt:    "Error: locator.click: Test timeout of 30000ms exceeded.\n    >  6 |   await page.getByRole('button', { name: 'Sign in' }).click();",
		ProjectDir: projectDir,
	}
}

func promptSection(prompt, heading string) string {
	start := strings.Index(prompt, heading+"\n")
	if start < 0 {
		return ""
	}
	rest := prompt[start+len(heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return heading + rest
}

// AC-QALOOP-014: the repair prompt names the failing step, its kind and ac,
// and where the trace and screenshot live, without inlining their bytes.
func TestWriteFeedbackBundleWithReplay_PromptCitesFailingStepAndCapturePaths(t *testing.T) {
	t.Parallel()
	projectDir := t.TempDir()
	manifest := replayManifest(t, projectDir)

	result, err := WriteFeedbackBundleWithReplay(manifest, "codex", filepath.Join(t.TempDir(), "out"), loginReplay(projectDir))

	require.NoError(t, err)
	prompt := readFile(t, result.PromptPath)
	section := promptSection(prompt, "## Replay")
	require.NotEmpty(t, section, "a located failing step must produce a replay section")
	assert.Contains(t, section, "- Failing step: screen `login`, step 1, kind `action`")
	assert.Contains(t, section, "- Acceptance criterion (`ac`): `AC-LOGIN-001`")
	assert.Contains(t, section, "- Generated spec line: `e2e/autopus-generated/login.spec.ts:6`")
	assert.Contains(t, section, "  - trace (`trace`): `test-results/login/trace.zip`")
	assert.Contains(t, section, "  - screenshot (`screenshot`): `test-results/login/test-failed-1.png`")
	assert.Contains(t, section, "  - console (`console`): `test-results/login/console.json`")
	assert.Contains(t, section, "  - network (`network_summary`): `test-results/login/network-summary.json`")
	assert.Contains(t, section, "Error: locator.click: Test timeout of 30000ms exceeded.")
	assert.NotContains(t, section, "pruned-trace.zip", "a capture that is not on disk must not be cited")
	assert.NotContains(t, section, `{"messages":["ok"]}`, "the replay cites capture paths, never their contents")
	assert.NotContains(t, section, "<run>", "every capture here is project-relative")
	assert.NotContains(t, prompt, traceSentinel)
	assert.NotContains(t, prompt, screenshotSentinel)
	assert.Less(t, strings.Index(prompt, "## Replay"), strings.Index(prompt, "## Reproduction"))
}

func TestWriteFeedbackBundleWithReplay_CitesRunDirectoryCapturesRelativeToTheRun(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	manifest := fixtureManifest(t, "browser", "failed")
	manifest.sourceDir = runDir
	writeUnder(t, runDir, "capture/trace.zip", []byte("PK\x03\x04"+traceSentinel))
	manifest.Artifacts = append(manifest.Artifacts, ArtifactRef{Kind: "trace", Path: "capture/trace.zip", Redaction: "local_only_quarantine_ref"})
	replay := loginReplay("")
	replay.Excerpt = ""

	result, err := WriteFeedbackBundleWithReplay(manifest, "claude", filepath.Join(t.TempDir(), "out"), replay)

	require.NoError(t, err)
	section := promptSection(readFile(t, result.PromptPath), "## Replay")
	assert.Contains(t, section, "  - trace (`trace`): `<run>/capture/trace.zip`")
	assert.Contains(t, section, "`<run>` is the run directory that holds the evidence manifest")
	assert.NotContains(t, section, "Failure excerpt")
}

func TestWriteFeedbackBundleWithReplay_SaysWhenNoCaptureIsOnDisk(t *testing.T) {
	t.Parallel()
	manifest := fixtureManifest(t, "browser", "failed")
	manifest.Artifacts = []ArtifactRef{manifest.Artifacts[len(manifest.Artifacts)-1]}
	manifest.Lane = "fast"

	result, err := WriteFeedbackBundleWithReplay(manifest, "codex", filepath.Join(t.TempDir(), "out"), loginReplay(t.TempDir()))

	require.NoError(t, err)
	assert.Contains(t, promptSection(readFile(t, result.PromptPath), "## Replay"), "- Captures: none recorded on disk for this run")
}

func TestWriteFeedbackBundle_HasNoReplayWithoutALocatedStep(t *testing.T) {
	t.Parallel()
	for name, replay := range map[string]*Replay{"nil": nil, "unlocated": {SpecPath: "e2e/x.spec.ts"}} {
		result, err := WriteFeedbackBundleWithReplay(fixtureManifest(t, "browser", "failed"), "codex", filepath.Join(t.TempDir(), name), replay)
		require.NoError(t, err)
		assert.NotContains(t, readFile(t, result.PromptPath), "## Replay", name)
	}
	result, err := WriteFeedbackBundle(fixtureManifest(t, "browser", "failed"), "codex", t.TempDir())
	require.NoError(t, err)
	assert.NotContains(t, readFile(t, result.PromptPath), "## Replay")
}

func TestFailureExcerpt_ReturnsTheRedactedTailOfRunnerOutput(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	var stdout strings.Builder
	stdout.WriteString(strings.Repeat("x", maxFailureReadBytes+1024) + "\n")
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&stdout, "runner line-%02d\n", i)
	}
	stdout.WriteString("Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789\n")
	writeUnder(t, runDir, "stdout.log", []byte(stdout.String()))
	writeUnder(t, runDir, "stderr.log", []byte("LOCAL-ONLY-STDERR\n"))
	manifest := fixtureManifest(t, "browser", "failed")
	manifest.Artifacts = append(manifest.Artifacts,
		ArtifactRef{Kind: "stdout", Path: "stdout.log", Publishable: true, Redaction: "text_redacted_and_scanned"},
		ArtifactRef{Kind: "stderr", Path: "stderr.log", Redaction: "local_only_quarantine_ref"},
	)

	excerpt := FailureExcerpt(manifest, runDir)

	assert.Contains(t, excerpt, "runner line-60")
	assert.NotContains(t, excerpt, "runner line-01", "only the bounded tail is quoted")
	assert.NotContains(t, excerpt, "xxxx", "a runaway log is not read whole")
	assert.NotContains(t, excerpt, "abcdefghijklmnopqrstuvwxyz0123456789")
	assert.NotContains(t, excerpt, "LOCAL-ONLY-STDERR", "local-only output never leaves the run")
	assert.NotContains(t, excerpt, `"messages"`, "runner output outranks console logs")
}

func TestFailureExcerpt_FallsBackToTheFailedChecksArtifacts(t *testing.T) {
	t.Parallel()
	manifest := publishedFailedManifest(t, "console", "console failure detail\n")

	assert.Contains(t, FailureExcerpt(manifest, ""), "console failure detail")
	assert.Empty(t, FailureExcerpt(Manifest{}, ""))
}
