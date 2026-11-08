package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Codex wrote `exit_code: nonzero` in one case of a twenty-case document;
// that one case must not take the other nineteen with it.
const (
	salvageHeader = "schema_version: qamesh.test-scenarios.v1\nspec: SPEC-X\ncases:\n"
	goodCase      = `  - id: shows-top-word
    ac: AC-1
    kind: happy
    title: top word
    given: a file
    when: run
    then: top word shown
    automation:
      type: exec
      exec:
        build: [go, build, -o, $BIN/tool, .]
        argv: [$BIN/tool, in.txt]
        expect: {exit_code: 0, stdout_lines: [apple]}
`
	badCase = `  - id: missing-file-fails
    ac: AC-1
    kind: negative
    title: missing file
    given: no file
    when: run
    then: fails
    automation:
      type: exec
      exec:
        build: [go, build, -o, $BIN/tool, .]
        argv: [$BIN/tool, missing.txt]
        expect: {exit_code: nonzero}
`
	oneBadCase = salvageHeader + goodCase + badCase
)

func TestEvaluate_OneMalformedCaseDropsOnlyItself(t *testing.T) {
	out := Evaluate("SPEC-X", []string{"AC-1"}, []string{oneBadCase}, nil)
	require.Len(t, out.TestScenarios, 1)
	assert.Contains(t, out.TestScenarios[0].Body, "shows-top-word")
	assert.NotContains(t, out.TestScenarios[0].Body, "missing-file-fails")
	require.Len(t, out.Rejected, 1)
	assert.Equal(t, "missing-file-fails", out.Rejected[0].ID)
	assert.Contains(t, out.Rejected[0].Message, "case dropped, the rest of the document kept")
	assert.Contains(t, out.Rejected[0].Message, "nonzero")
}

// With no valid case left, the whole document is rejected as before.
func TestEvaluate_AllCasesMalformedRejectsTheDocument(t *testing.T) {
	bad := salvageHeader + badCase
	out := Evaluate("SPEC-X", []string{"AC-1"}, []string{bad}, nil)
	assert.Empty(t, out.TestScenarios)
	require.Len(t, out.Rejected, 1)
	assert.Empty(t, out.Rejected[0].ID)
}

// Codex ran ./wordtop with no build step in a project that holds no such
// file: every run would fail before reaching the product. With the project
// known, that case is dropped and the buildable one kept.
func TestEvaluateIn_UnbuiltProjectProgramIsDropped(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "present"), []byte("#!/bin/sh\n"), 0o755))
	unbuilt := strings.Replace(badCase, "build: [go, build, -o, $BIN/tool, .]\n        argv: [$BIN/tool, missing.txt]", "argv: [./wordtop, missing.txt]", 1)
	unbuilt = strings.Replace(unbuilt, "{exit_code: nonzero}", "{exit_nonzero: true}", 1)
	present := strings.Replace(unbuilt, "./wordtop", "./present", 1)
	present = strings.Replace(present, "missing-file-fails", "present-program", 1)
	require.NotEqual(t, badCase, unbuilt)

	out := evaluateIn(project, "SPEC-X", []string{"AC-1"}, []string{salvageHeader + goodCase + unbuilt + present}, nil)
	require.Len(t, out.TestScenarios, 1)
	assert.Contains(t, out.TestScenarios[0].Body, "shows-top-word")
	assert.Contains(t, out.TestScenarios[0].Body, "present-program")
	require.Len(t, out.Rejected, 1)
	assert.Equal(t, "missing-file-fails", out.Rejected[0].ID)
	assert.Equal(t, CodeExecProgramMissing, out.Rejected[0].Code)

	only := evaluateIn(project, "SPEC-X", []string{"AC-1"}, []string{salvageHeader + unbuilt}, nil)
	require.Len(t, only.Rejected, 1)
	assert.Equal(t, CodeExecProgramMissing, only.Rejected[0].Code, "a whole-document reject still names the real cause")
}
