package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const playwrightStubReport = `{"suites":[{"specs":[{"tests":[{"results":[{"attachments":[` +
	`{"name":"screenshot","contentType":"image/png","path":"test-results/home.png"}]}]}]}]}`

// verifyUIWorkspace stages a workspace where git reports one changed UI file and
// npx emits a Playwright JSON report, so the UI-change branch of verify is reachable.
func verifyUIWorkspace(t *testing.T) string {
	t.Helper()
	binDir := verifyStubPath(t, "node", "playwright")
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git"),
		[]byte("#!/bin/sh\nprintf 'src/App.tsx\\nREADME.md\\n'\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "npx"),
		[]byte("#!/bin/sh\ncat <<'EOF'\n"+playwrightStubReport+"\nEOF\n"), 0o755))

	return verifyWorkspace(t, "verify:\n  enabled: true\n  default_viewport: desktop\n  max_fix_attempts: 2\n")
}

// The UI-change branch must list the UI files, report a screenshot-less run as a
// failing visual gate, and persist the gate evidence.
func TestRunVerify_UIChangeWithoutScreenshotsFailsGate(t *testing.T) {
	dir := verifyUIWorkspace(t)

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			_ = runVerifyWithOptions(nil, true, false, "mobile", verifyVisualOptions{Enabled: true})
		})
	})

	assert.Contains(t, stderr, "변경된 프론트엔드 파일 1개 감지됨")
	assert.Contains(t, stderr, "- src/App.tsx")
	assert.NotContains(t, stderr, "README.md", "non-UI files must be filtered out")
	assert.Contains(t, stderr, "snapshot comparison proof", "missing proof must be reported")

	assert.Contains(t, stdout, "verify 완료 (viewport: mobile, auto-fix: true)")
	assert.Contains(t, stdout, "시각 증거 0개 수집됨")
	assert.Contains(t, stdout, "visual gate: FAIL")
	assert.Contains(t, stdout, "screenshot_capture: FAIL")

	_, statErr := os.Stat(filepath.Join(dir, ".autopus", "design", "verify", "latest.v2.json"))
	require.NoError(t, statErr, "visual gate must persist its report")
}

// Strict gating must turn a FAIL verdict into a command error.
func TestRunVerify_StrictGateTurnsFailVerdictIntoError(t *testing.T) {
	verifyUIWorkspace(t)

	var err error
	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			err = runVerifyWithOptions(nil, true, false, "desktop", verifyVisualOptions{Enabled: true, Strict: true})
		})
	})

	require.Error(t, err, "strict mode must fail when the visual gate verdict is FAIL")
}

// --report-only must win over --fix in the reported effective mode.
func TestRunVerify_ReportOnlyOverridesFix(t *testing.T) {
	verifyUIWorkspace(t)

	var stdout string
	_ = captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			_ = runVerifyWithOptions(nil, true, true, "desktop", verifyVisualOptions{Enabled: true})
		})
	})

	assert.Contains(t, stdout, "auto-fix: false")
}

// With the visual gate disabled, verify must still report evidence without writing gate output.
func TestRunVerify_VisualGateDisabledSkipsGateEvidence(t *testing.T) {
	dir := verifyUIWorkspace(t)

	var stdout string
	_ = captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			_ = runVerifyWithOptions(nil, true, false, "desktop", verifyVisualOptions{Enabled: false})
		})
	})

	assert.Contains(t, stdout, "verify 완료 (viewport: desktop, auto-fix: true)")
	_, statErr := os.Stat(filepath.Join(dir, ".autopus"))
	assert.True(t, os.IsNotExist(statErr), "disabled gate must not write gate evidence")
}
