package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// markSpecLean turns the scaffolded SPEC into a lean one over surface.
func markSpecLean(t *testing.T, specDir, class, surface string) {
	t.Helper()
	path := filepath.Join(specDir, "spec.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := strings.Replace(string(data), "status: draft\n",
		"status: draft\nprofile: lean\nchange_class: "+class+"\nsurface: "+surface+"\n", 1)
	require.NoError(t, os.WriteFile(path, []byte(text), 0o644))
}

func TestSpecGatesCmd_LeanSpecInsideSurfaceStaysLowRisk(t *testing.T) {
	_, specDir := specGatesProject(t)
	markSpecLean(t, specDir, "bugfix_existing_contract", "pkg/a")

	out, err := runSpecGates(t, "SPEC-GATES-001", "--changed", "pkg/a/x.go,pkg/a/x_test.go", "--json")
	require.NoError(t, err)
	var receipt gates.ApplicabilityReceipt
	require.NoError(t, json.Unmarshal([]byte(out), &receipt))
	assert.Equal(t, gates.KindBugfix, receipt.ChangeRisk.DeclaredClass, "the lean SPEC's class is the declared class")
	assert.False(t, receipt.ChangeRisk.Escalated())
}

func TestSpecGatesCmd_LeanSpecOutsideSurfaceEscalates(t *testing.T) {
	_, specDir := specGatesProject(t)
	markSpecLean(t, specDir, "bugfix_existing_contract", "pkg/a")

	out, err := runSpecGates(t, "SPEC-GATES-001", "--changed", "pkg/a/x.go,pkg/a/y.go,pkg/a/sub/z.go,pkg/a2/w.go")
	require.NoError(t, err)
	assert.Contains(t, out, "lean SPEC 승격 필요")
	assert.Contains(t, out, gates.EscalationLeanSurfaceExceeded)
	assert.Contains(t, out, "선언한 surface 밖의 경로: pkg/a2/w.go")

	persisted, err := gates.ReadApplicability(specDir)
	require.NoError(t, err)
	assert.True(t, persisted.ChangeRisk.Escalated())
	authoring, ok := persisted.Decision(gates.GateSpecAuthoring)
	require.True(t, ok)
	assert.Equal(t, gates.Required, authoring.Applicability, "escalation sends the work back to a full SPEC")
}

func TestSpecGatesCmd_LeanSpecTouchingSecurityPathEscalates(t *testing.T) {
	_, specDir := specGatesProject(t)
	markSpecLean(t, specDir, "bugfix_existing_contract", "pkg/a, pkg/auth")

	out, err := runSpecGates(t, "SPEC-GATES-001", "--changed", "pkg/auth/session.go")
	require.NoError(t, err)
	assert.Contains(t, out, "lean SPEC 승격 필요: 실제 변경이 security_or_data로 판정됨")
	assert.NotContains(t, out, "선언한 surface 밖의 경로")
}

func TestSpecGatesCmd_FullSpecIsUnchanged(t *testing.T) {
	specGatesProject(t)

	out, err := runSpecGates(t, "SPEC-GATES-001", "--changed", "pkg/a/x.go")
	require.NoError(t, err)
	assert.NotContains(t, out, "lean SPEC")
}
