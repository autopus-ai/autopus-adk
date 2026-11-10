package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type secureRun struct {
	code           int
	stdout, stderr string
}

func runSecureCLI(t *testing.T, args ...string) secureRun {
	t.Helper()
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	code, _ := runRootCommand(context.Background(), root, &stderr)
	return secureRun{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// fakeScannerPath puts a fake gitleaks that writes the recorded report on
// PATH, with no other scanner, and returns a Go project in a git tree.
func fakeScannerPath(t *testing.T, withGitleaks bool) string {
	t.Helper()
	bin := t.TempDir()
	if withGitleaks {
		fixture, err := filepath.Abs(filepath.Join("..", "..", "pkg", "securityscan", "testdata", "gitleaks.json"))
		require.NoError(t, err)
		script := "#!/bin/sh\n[ \"$1\" = dir ] && [ \"$2\" = --help ] && exit 0\n" +
			"while [ $# -gt 0 ]; do\n  [ \"$1\" = --report-path ] && cp '" + fixture + "' \"$2\"\n  shift\ndone\nexit 0\n"
		require.NoError(t, os.WriteFile(filepath.Join(bin, "gitleaks"), []byte(script), 0o755))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
	return dir
}

func TestSecureScan_TableFailsOnASecretWithoutPrintingIt(t *testing.T) {
	dir := fakeScannerPath(t, true)
	run := runSecureCLI(t, "secure", "scan", "--dir", dir)
	assert.Equal(t, 1, run.code, run.stdout+run.stderr)
	assert.Contains(t, run.stdout, "Ecosystems: go")
	assert.Contains(t, run.stdout, "config/settings.py:12")
	assert.Contains(t, run.stdout, "govulncheck not installed")
	assert.Contains(t, run.stdout, "FAIL")
	assert.NotContains(t, run.stdout+run.stderr, "fixture-not-a-real-value-0001")

	none := runSecureCLI(t, "secure", "scan", "--dir", dir, "--fail-on", "none")
	assert.Equal(t, 0, none.code, none.stdout+none.stderr)
}

func TestSecureScan_JSON(t *testing.T) {
	dir := fakeScannerPath(t, true)
	run := runSecureCLI(t, "secure", "scan", "--dir", dir, "--json")
	assert.Equal(t, 1, run.code, run.stderr)
	var got struct {
		Failed   bool `json:"failed"`
		Scanners []struct {
			Name, Status string
		} `json:"scanners"`
		Findings []struct {
			Kind, ID, Location, Fingerprint string
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(run.stdout), &got), run.stdout)
	assert.True(t, got.Failed)
	require.Len(t, got.Findings, 2)
	assert.Equal(t, "generic-api-key", got.Findings[0].ID)
	assert.NotContains(t, run.stdout, "fixture-not-a-real-value-0001")
}

func TestSecureScan_MissingToolsPassWithANote(t *testing.T) {
	dir := fakeScannerPath(t, false)
	run := runSecureCLI(t, "secure", "scan", "--dir", dir)
	assert.Equal(t, 0, run.code, run.stdout+run.stderr)
	assert.Contains(t, run.stdout, "gitleaks not installed")
	assert.Contains(t, run.stdout, "PASS")
}

func TestSecureScan_RejectsAnUnknownThreshold(t *testing.T) {
	run := runSecureCLI(t, "secure", "scan", "--dir", t.TempDir(), "--fail-on", "sometimes")
	assert.NotEqual(t, 0, run.code)
	assert.Contains(t, run.stderr, "--fail-on")
}
