package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func qaExecProject(t *testing.T, cases string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module hello\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(qaExecProduct), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "qa", "test-scenarios"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".autopus/qa/test-scenarios/hello.yaml"), []byte(cases), 0o644))
	return dir
}

// --all runs every exec case, reports each failure with Expected:/Received:,
// and exits 1 when a product expectation failed.
func TestQAExecCmd_AllRunsEveryCase(t *testing.T) {
	dir := qaExecProject(t, qaExecCases)
	out, err := runQAExecCLI(t, dir, "--all")
	require.Error(t, err)
	assert.Equal(t, qaExecExitMismatch, exitCodeForError(err))
	assert.Contains(t, out, "Received: \"hello you\\n\"")
	assert.Contains(t, out, "qa exec --all: 1 passed, 1 failed, 0 could not run")
}

// A case that cannot run exits 2 only when no expectation failed, and a
// project whose cases all pass exits 0.
func TestQAExecCmd_AllSeparatesCannotRunFromPassing(t *testing.T) {
	passing := strings.SplitN(qaExecCases, "  - id: greets-wrongly", 2)[0]
	out, err := runQAExecCLI(t, qaExecProject(t, passing), "--all")
	require.NoError(t, err, out)
	assert.Contains(t, out, "1 passed, 0 failed, 0 could not run")

	broken := strings.Replace(passing, "build: [go, build, -o, $BIN/hello, .]", "build: [go, build, -o, $BIN/hello, ./nope]", 1)
	out, err = runQAExecCLI(t, qaExecProject(t, broken), "--all")
	require.Error(t, err)
	assert.Equal(t, qaExecExitError, exitCodeForError(err), out)

	_, err = runQAExecCLI(t, t.TempDir(), "--all", "--case", "greets")
	assert.Equal(t, qaExecExitError, exitCodeForError(err), "--all takes no --case")
}

// A project whose cases are all command or manual cases runs nothing; the
// summary must say so instead of reading as a pass. With no cases at all,
// the line stays the plain summary.
func TestQAExecCmd_AllSaysWhenNothingRan(t *testing.T) {
	cases := `schema_version: qamesh.test-scenarios.v1
spec: SPEC-X-001
cases:
  - id: wraps-unit-tests
    ac: AC-1
    kind: happy
    title: t
    given: g
    when: w
    then: th
    automation:
      type: command
      check:
        argv: [go, test, ./...]
        cwd: .
        timeout: 1m
  - id: needs-a-person
    ac: AC-1
    kind: edge
    title: t
    given: g
    when: w
    then: th
    automation:
      type: manual
      reason: a person judges it
`
	out, err := runQAExecCLI(t, qaExecProject(t, cases), "--all")
	require.NoError(t, err, out)
	assert.Contains(t, out, "qa exec --all: 0 passed, 0 failed, 0 could not run")
	assert.Contains(t, out, "no exec cases ran, so nothing was checked; 2 command or manual case(s) are not run by this command")

	out, err = runQAExecCLI(t, t.TempDir(), "--all")
	require.NoError(t, err, out)
	assert.NotContains(t, out, "nothing was checked")
}
