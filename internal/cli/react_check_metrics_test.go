package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-SIGMABAND-001 REQ-17 / S17: `auto react check` never writes the band
// metric store, and its gh calls stay exactly what they were before band
// existed. These tests drive the real exec path with a fake gh on PATH and a
// real git repository that has an origin remote.

const reactCheckRunListArgs = "run list --status failure --limit 5 --json databaseId,name,conclusion,headBranch,updatedAt"

const fakeGHScript = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_GH_LOG"
case "$1 $2" in
  "run list") cat "$FAKE_GH_RUNS" ;;
  "run view") printf 'FAIL: build step\n' ;;
  *) exit 1 ;;
esac
`

// setupReactCheckProject chdirs into a fresh repo with an origin remote, puts
// a fake gh first on PATH, and returns the project dir and the gh argv log.
func setupReactCheckProject(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake gh is a POSIX shell script")
	}
	stubReactExec(t, execLookPath, execOutput)

	dir := t.TempDir()
	initSyncRepo(t, dir)
	syncGit(t, dir, "remote", "add", "origin", "git@github.com:acme/app.git")

	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(fakeGHScript), 0o755))
	runs := filepath.Join(bin, "runs.json")
	require.NoError(t, os.WriteFile(runs, []byte(twoFailedRunsJSON), 0o644))
	ghLog := filepath.Join(bin, "gh.log")

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_LOG", ghLog)
	t.Setenv("FAKE_GH_RUNS", runs)
	t.Chdir(dir)
	return dir, ghLog
}

func runReactCheckCommand(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"react", "check"}, args...))
	require.NoError(t, root.Execute(), out.String())
	return out.String()
}

func readGHLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "the fake gh must have been called")
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// fileDigests maps every regular file under root (slash paths) to its SHA-256.
func fileDigests(t *testing.T, root string) map[string]string {
	t.Helper()
	digests := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		digests[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	require.NoError(t, err)
	return digests
}

func TestReactCheck_QuietHookCommandLeavesMetricsStoreAbsent(t *testing.T) {
	dir, ghLog := setupReactCheckProject(t)

	out := runReactCheckCommand(t, "--quiet")

	assert.Equal(t, "Found 2 failed run(s):\n", out)
	_, err := os.Lstat(filepath.Join(dir, ".autopus", "metrics"))
	assert.True(t, os.IsNotExist(err), "react check must not create .autopus/metrics/ (stat err: %v)", err)
	assert.Equal(t, []string{reactCheckRunListArgs}, readGHLog(t, ghLog),
		"the hook command keeps its one failure listing and makes no band gh call")
}

func TestReactCheck_VerboseKeepsExistingMetricsStoreByteIdentical(t *testing.T) {
	dir, ghLog := setupReactCheckProject(t)
	metricsDir := filepath.Join(dir, ".autopus", "metrics")
	syncWrite(t, dir, ".autopus/metrics/ci-runs.jsonl",
		`{"schema":"autopus.metric_observation.v1","series":"ci.failure_rate:CI","sample_key":"1042",`+
			`"observed_at":"2026-09-14T10:00:00Z","tiebreak":1042,"value":1,"attempt":2,"source":"gh"}`+"\n")
	syncWrite(t, dir, ".autopus/metrics/band-state.json", `{"schema":"autopus.band_state.v1","last_seq":7}`+"\n")
	before := fileDigests(t, metricsDir)

	out := runReactCheckCommand(t)

	assert.Contains(t, out, "Found 2 failed run(s):")
	assert.Equal(t, before, fileDigests(t, metricsDir), "react check must not add, change, or remove a store file")
	for _, id := range []string{"111", "222"} {
		assert.FileExists(t, filepath.Join(dir, ".autopus", "react", id+".md"), "react reports stay where they were")
	}
	assert.Equal(t, []string{
		reactCheckRunListArgs,
		"run view 111 --log-failed",
		"run view 222 --log-failed",
	}, readGHLog(t, ghLog))
}
