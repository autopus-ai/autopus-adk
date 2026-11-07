package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const qaExecProduct = "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() { fmt.Println(\"hello \" + os.Args[1]) }\n"

const qaExecCases = `schema_version: qamesh.test-scenarios.v1
spec: INTENT-hello
cases:
  - id: greets
    ac: AC-1
    kind: happy
    title: greets the name
    given: the tool is built
    when: hello world
    then: prints hello world
    automation:
      type: exec
      exec:
        build: [go, build, -o, $BIN/hello, .]
        argv: [$BIN/hello, world]
        expect: {exit_code: 0, stdout: "hello world\n"}
  - id: greets-wrongly
    ac: AC-2
    kind: edge
    title: an expectation the product does not meet
    given: the tool is built
    when: hello you
    then: prints hi you
    automation:
      type: exec
      exec:
        build: [go, build, -o, $BIN/hello, .]
        argv: [$BIN/hello, you]
        expect: {stdout: "hi you\n"}
`

func runQAExecCLI(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"qa", "exec", "--project-dir", dir}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestQAExecCmd_ExitCodeSeparatesVerdictFromCannotRun(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module hello\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(qaExecProduct), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "qa", "test-scenarios"), 0o755))
	file := ".autopus/qa/test-scenarios/hello.yaml"
	require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(qaExecCases), 0o644))

	out, err := runQAExecCLI(t, dir, "--file", file, "--case", "greets")
	require.NoError(t, err, out)
	assert.Contains(t, out, "exec case greets (AC-1): passed")

	out, err = runQAExecCLI(t, dir, "--file", file, "--case", "greets-wrongly")
	require.Error(t, err)
	assert.Equal(t, qaExecExitMismatch, exitCodeForError(err))
	assert.Contains(t, out, "Expected: \"hi you\\n\"")
	assert.Contains(t, out, "Received: \"hello you\\n\"")

	_, err = runQAExecCLI(t, dir, "--file", file, "--case", "nope")
	assert.Equal(t, qaExecExitError, exitCodeForError(err))
}
