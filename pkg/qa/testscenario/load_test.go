package testscenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validYAML = `schema_version: qamesh.test-scenarios.v1
spec: SPEC-X
cases:
  - id: unit-suite
    ac: AC-X-001
    kind: happy
    title: unit suite passes
    given: the module builds
    when: the suite runs
    then: it exits 0
    automation:
      type: command
      check:
        adapter: go-test
        argv: ["go", "test", "./..."]
        cwd: .
        timeout: 120s
        env_allowlist: ["CI"]
  - id: login-rejected
    ac: AC-X-002
    kind: negative
    title: bad password is rejected
    automation:
      type: gui
      scenario: login
`

func writeDocument(t *testing.T, projectDir, rel, name, body string) {
	t.Helper()
	dir := filepath.Join(projectDir, rel)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func withSpec(spec string) string {
	return strings.Replace(validYAML, "spec: SPEC-X", "spec: "+spec, 1)
}

func TestParseBytesDecodesDocument(t *testing.T) {
	t.Parallel()

	doc, err := ParseBytes("SPEC-X.yaml", []byte(validYAML))

	require.NoError(t, err)
	assert.Equal(t, "SPEC-X.yaml", doc.Path)
	assert.Equal(t, "SPEC-X", doc.Spec)
	require.Len(t, doc.Cases, 2)
	assert.Equal(t, "the suite runs", doc.Cases[0].When)
	assert.Equal(t, &Check{
		Adapter: "go-test", Argv: []string{"go", "test", "./..."}, CWD: ".", Timeout: "120s", EnvAllowlist: []string{"CI"},
	}, doc.Cases[0].Automation.Check)
	assert.Equal(t, Automation{Type: AutomationGUI, Scenario: "login"}, doc.Cases[1].Automation)
}

func TestParseBytesTrimsIdentityFields(t *testing.T) {
	t.Parallel()

	body := strings.Replace(validYAML, "    kind: negative", "    kind: \" negative \"", 1)
	body = strings.Replace(body, "  - id: login-rejected", "  - id: \" login-rejected \"", 1)

	doc, err := ParseBytes("SPEC-X.yaml", []byte(body))

	require.NoError(t, err)
	assert.Equal(t, "login-rejected", doc.Cases[1].ID)
	assert.Equal(t, KindNegative, doc.Cases[1].Kind)
}

func TestParseBytesRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":       "",
		"not yaml":    "cases: [",
		"unknown key": strings.Replace(validYAML, "    ac: AC-X-001\n", "    ac: AC-X-001\n    acc: typo\n", 1),
		// env is how a qamesh-check smuggles secrets; the schema only has env_allowlist.
		"env key":         strings.Replace(validYAML, "        cwd: .\n", "        cwd: .\n        env: [\"TOKEN\"]\n", 1),
		"second document": validYAML + "---\n" + withSpec("SPEC-Y"),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseBytes("bad.yaml", []byte(body))

			var validationErr *ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, "qa_test_scenario_parse_invalid", validationErr.Code)
			assert.Equal(t, "bad.yaml", validationErr.Path)
		})
	}
}

func TestParseBytesToleratesTrailingSeparator(t *testing.T) {
	t.Parallel()

	_, err := ParseBytes("SPEC-X.yaml", []byte(validYAML+"---\n"))

	assert.NoError(t, err)
}

func TestParseBytesValidatesDecodedDocument(t *testing.T) {
	t.Parallel()

	_, err := ParseBytes("SPEC-X.yaml", []byte(strings.Replace(validYAML, "kind: negative", "kind: sad", 1)))

	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "qa_test_scenario_kind_invalid", validationErr.Code)
}

func TestLoadDirReadsTopLevelYAMLInOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeDocument(t, dir, DirRel, "b.yaml", withSpec("SPEC-B"))
	writeDocument(t, dir, DirRel, "a.yaml", withSpec("SPEC-A"))
	writeDocument(t, dir, DirRel, "notes.yml", "not: a test scenario")
	writeDocument(t, dir, CandidatesDirRel, "c.yaml", withSpec("SPEC-C"))

	docs, err := LoadDir(dir)
	require.NoError(t, err)
	candidates, candidateErr := LoadCandidates(dir)
	require.NoError(t, candidateErr)

	require.Len(t, docs, 2)
	assert.Equal(t, []string{"SPEC-A", "SPEC-B"}, []string{docs[0].Spec, docs[1].Spec})
	assert.Equal(t, []string{"a.yaml", "b.yaml"}, []string{docs[0].Path, docs[1].Path})
	require.Len(t, candidates, 1)
	assert.Equal(t, "SPEC-C", candidates[0].Spec)
	assert.Equal(t, "c.yaml", candidates[0].Path)
}

func TestLoadDirFailsClosedOnOneInvalidFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeDocument(t, dir, DirRel, "a.yaml", withSpec("SPEC-A"))
	writeDocument(t, dir, DirRel, "b.yaml", strings.Replace(validYAML, "spec: SPEC-X\n", "", 1))

	docs, err := LoadDir(dir)

	assert.Nil(t, docs)
	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "qa_test_scenario_spec_missing", validationErr.Code)
	assert.Equal(t, "b.yaml", validationErr.Path)
}

func TestLoadDirWithoutDirectoryReturnsNothing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	docs, err := LoadDir(dir)
	require.NoError(t, err)
	candidates, candidateErr := LoadCandidates(dir)
	require.NoError(t, candidateErr)

	assert.Empty(t, docs)
	assert.Empty(t, candidates)
}
