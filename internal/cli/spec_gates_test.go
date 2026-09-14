package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/spec"
	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// specGatesProject scaffolds a project with a SPEC, a small Go package, and a
// go.sum, then chdirs into it so SPEC IDs resolve like they do for users.
func specGatesProject(t *testing.T) (root, specDir string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, spec.Scaffold(root, "GATES-001", "Gate Applicability"))
	specDir = filepath.Join(root, ".autopus", "specs", "SPEC-GATES-001")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "pkg", "a"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pkg", "a", "x.go"), []byte("package a\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.sum"), []byte("example.com/dep v1.0.0 h1:abc\n"), 0o644))

	origWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	require.NoError(t, os.Chdir(root))
	return root, specDir
}

func runSpecGates(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newSpecGatesCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestSpecGatesCmd_ScenariosProduceDifferentDecisions(t *testing.T) {
	_, specDir := specGatesProject(t)

	ui, err := runSpecGates(t, "SPEC-GATES-001", "--changed", "frontend/src/components/Button.tsx")
	require.NoError(t, err)
	assert.Contains(t, ui, "SPEC-GATES-001 (ui_only): 1 changed path(s)")
	assert.Contains(t, ui, "ux_verification: required — UI surface in change set: 1 path(s)")
	assert.Contains(t, ui, "integration: not_applicable — single-domain change set")

	db, err := runSpecGates(t, specDir, "--changed", "backend/db/migrations/001.sql")
	require.NoError(t, err)
	assert.Contains(t, db, "(security_or_data)")
	assert.Contains(t, db, "integration: required — security_or_data change set crosses an integration boundary")
	assert.Contains(t, db, "accessibility: not_applicable — no UI surface in change set")

	multi, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go,pkg/b/y.go", "--json")
	require.NoError(t, err)
	var receipt gates.ApplicabilityReceipt
	require.NoError(t, json.Unmarshal([]byte(multi), &receipt))
	assert.Equal(t, gates.ClassMultiDomain, receipt.ChangeClass)
	integration, ok := receipt.Decision(gates.GateIntegration)
	require.True(t, ok)
	assert.Equal(t, gates.Required, integration.Applicability)

	persisted, err := gates.ReadApplicability(specDir)
	require.NoError(t, err)
	assert.Equal(t, receipt, persisted, "--json prints exactly the persisted receipt")
	for _, id := range []gates.GateID{gates.GateSecurity, gates.GateValidation, gates.GateDataLoss, gates.GateDeterministicOracle} {
		decision, ok := persisted.Decision(id)
		require.True(t, ok)
		assert.Equal(t, gates.Required, decision.Applicability, "%s must stay required", id)
	}
}

func TestSpecGatesCmd_RecordThenReuse(t *testing.T) {
	root, specDir := specGatesProject(t)
	relEvidence := ".autopus/specs/SPEC-GATES-001/gates/evidence-build.json"

	recorded, err := runSpecGates(t, "record", "SPEC-GATES-001",
		"--gate", "build", "--status", "pass",
		"--inputs", "pkg/a/*.go", "--dynamic-deps", "go.sum", "--command", "go build ./pkg/a")
	require.NoError(t, err)
	assert.Contains(t, recorded, "evidence recorded: "+filepath.FromSlash(relEvidence))
	assert.Contains(t, recorded, "build pass, complete=true, 1 input(s), 1 dynamic dep(s)")
	info, err := os.Stat(gates.EvidencePath(specDir, gates.GateBuild))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	out, err := runSpecGates(t, "SPEC-GATES-001", "--changed", "pkg/a/x.go")
	require.NoError(t, err)
	assert.Contains(t, out, "build: reusable — exact-input evidence matches current tree")
	assert.Contains(t, out, "unit_tests: required — code change set; no prior evidence")
	assert.Contains(t, out, "receipt: "+filepath.FromSlash(".autopus/specs/SPEC-GATES-001/gate-applicability.json"))

	receipt, err := gates.ReadApplicability(specDir)
	require.NoError(t, err)
	build, ok := receipt.Decision(gates.GateBuild)
	require.True(t, ok)
	require.NotNil(t, build.ReusedEvidence)
	assert.Equal(t, relEvidence, build.ReusedEvidence.Path, "persisted pointer is project-root relative")

	byDir, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go", "--json")
	require.NoError(t, err)
	assert.Contains(t, byDir, `"path": "`+relEvidence+`"`, "addressing by directory yields the same pointer")

	require.NoError(t, os.WriteFile(filepath.Join(root, "pkg", "a", "x.go"), []byte("package a\n// touched\n"), 0o644))
	out, err = runSpecGates(t, "SPEC-GATES-001", "--changed", "pkg/a/x.go")
	require.NoError(t, err)
	assert.Contains(t, out, "build: required — code change set; input closure changed")
}

func TestSpecGatesCmd_StaleEvidenceIsNotReused(t *testing.T) {
	_, specDir := specGatesProject(t)
	_, err := runSpecGates(t, "record", specDir, "--gate", "unit_tests", "--status", "pass", "--inputs", "pkg/a/*.go")
	require.NoError(t, err)

	out, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go", "--max-age", "1ns")
	require.NoError(t, err)
	assert.Contains(t, out, "unit_tests: required — code change set; evidence older than max-age")
}

func TestSpecGatesCmd_RecordRejectsInvalidInputs(t *testing.T) {
	_, specDir := specGatesProject(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing gate", []string{"record", specDir, "--status", "pass", "--inputs", "pkg/a/*.go"}, "--gate is required"},
		{"unknown gate", []string{"record", specDir, "--gate", "lint", "--status", "pass", "--inputs", "pkg/a/*.go"}, "unknown gate"},
		{"bad status", []string{"record", specDir, "--gate", "build", "--status", "ok", "--inputs", "pkg/a/*.go"}, "invalid status"},
		{"unmatched inputs", []string{"record", specDir, "--gate", "build", "--status", "pass", "--inputs", "pkg/none/*.go"}, "matched no files"},
		{"missing dynamic dep", []string{"record", specDir, "--gate", "build", "--status", "pass", "--inputs", "pkg/a/*.go", "--dynamic-deps", "go.work"}, "missing input: go.work"},
		{"unknown spec", []string{"record", "SPEC-NOPE", "--gate", "build", "--status", "pass", "--inputs", "pkg/a/*.go"}, "SPEC-NOPE not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runSpecGates(t, tc.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			_, ok, readErr := gates.ReadEvidence(specDir, gates.GateBuild)
			require.NoError(t, readErr)
			assert.False(t, ok, "no receipt may be written for invalid input")
		})
	}
}

// The @AX annotation gate is opt-in at the CLI boundary: a code change set is
// not held behind it unless --annotation asks for it, and only then can a
// missing reference source block.
func TestSpecGatesCmd_AnnotationIsOptIn(t *testing.T) {
	_, specDir := specGatesProject(t)

	defaultRun, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go")
	require.NoError(t, err)
	assert.Contains(t, defaultRun, "annotation: not_applicable — @AX annotation not requested")

	unrequestedMissing, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go",
		"--annotation-reference-missing")
	require.NoError(t, err)
	assert.Contains(t, unrequestedMissing, "annotation: not_applicable — @AX annotation not requested")

	requested, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go", "--annotation")
	require.NoError(t, err)
	assert.Contains(t, requested, "annotation: required — code change set")

	blocked, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go",
		"--annotation", "--annotation-reference-missing")
	require.NoError(t, err)
	assert.Contains(t, blocked, "annotation: blocked — reference source missing")
}
