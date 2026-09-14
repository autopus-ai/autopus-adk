package cli

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// verifyWorkspace writes an autopus.yaml with the given verify block and makes it cwd.
func verifyWorkspace(t *testing.T, verifyBlock string) string {
	t.Helper()
	dir := t.TempDir()
	yaml := "mode: full\nproject_name: test-proj\nplatforms:\n  - claude-code\n" + verifyBlock
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(yaml), 0o644))
	t.Chdir(dir)
	return dir
}

// verifyStubPath installs stub executables on an otherwise empty PATH.
func verifyStubPath(t *testing.T, names ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub executables require a POSIX shell")
	}
	binDir := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	t.Setenv("PATH", binDir)
	return binDir
}

// captureStderr mirrors captureStdout for the warning stream.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stderr
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writer
	defer func() { os.Stderr = orig }()

	fn()

	require.NoError(t, writer.Close())
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(data)
}

// Disabling verify in config must short-circuit before any tool probing.
func TestRunVerify_DisabledConfigWarnsAndSkipsToolProbes(t *testing.T) {
	verifyWorkspace(t, "verify:\n  enabled: false\n")
	// Empty PATH would make the node probe fail; reaching it proves we did not short-circuit.
	verifyStubPath(t)

	var err error
	stderr := captureStderr(t, func() {
		err = runVerifyWithOptions(nil, true, false, "desktop", verifyVisualOptions{Enabled: true})
	})

	require.NoError(t, err)
	assert.Contains(t, stderr, "verify.enabled: false")
}

// A missing node toolchain must be a hard error carrying the install hint.
func TestRunVerify_MissingNodeIsFatalWithInstallHint(t *testing.T) {
	verifyWorkspace(t, "verify:\n  enabled: true\n")
	verifyStubPath(t)

	var err error
	_ = captureStderr(t, func() {
		err = runVerifyWithOptions(nil, true, false, "desktop", verifyVisualOptions{Enabled: true})
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "node.js")
	assert.Contains(t, err.Error(), "https://nodejs.org")
}

// A missing playwright binary must degrade to a warning, not abort the run.
func TestRunVerify_MissingPlaywrightWarnsAndContinues(t *testing.T) {
	verifyWorkspace(t, "verify:\n  enabled: true\n  default_viewport: desktop\n  max_fix_attempts: 2\n")
	verifyStubPath(t, "node")

	var err error
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			err = runVerifyWithOptions(nil, true, false, "desktop", verifyVisualOptions{Enabled: true})
		})
	})

	require.NoError(t, err, "no UI changes means verify skips rather than failing")
	assert.Contains(t, stderr, "playwright")
	// git is absent from the stub PATH, so the diff step must warn instead of failing.
	assert.Contains(t, stderr, "git diff")
	assert.Contains(t, stdout, "변경된 프론트엔드 파일이 없습니다")
	assert.Contains(t, stdout, "design context: skipped (non-ui changes)")
}

// analyzeGitDiff must drop blank lines and surface an unusable git invocation.
func TestAnalyzeGitDiff_ParsesNamesAndReportsFailure(t *testing.T) {
	binDir := verifyStubPath(t, "git")
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git"),
		[]byte("#!/bin/sh\nprintf 'src/App.tsx\\n\\n  \\nREADME.md\\n'\n"), 0o755))

	files, err := analyzeGitDiff()
	require.NoError(t, err)
	assert.Equal(t, []string{"src/App.tsx", "README.md"}, files)

	require.NoError(t, os.Remove(filepath.Join(binDir, "git")))
	_, err = analyzeGitDiff()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git diff 실행 실패")
}

// The verify flag set must default fix and the visual gate on, and strict off.
func TestNewVerifyCmd_FlagDefaults(t *testing.T) {
	t.Parallel()

	cmd := newVerifyCmd()
	for name, want := range map[string]string{
		"fix":                  "true",
		"report-only":          "false",
		"viewport":             "desktop",
		"visual-gate":          "true",
		"strict-visual-gate":   "false",
		"visual-critic-report": "",
	} {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, "missing flag %q", name)
		assert.Equal(t, want, flag.DefValue, "flag %q default", name)
	}
}
