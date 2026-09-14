package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testContractSpecID = "SPEC-CONTRACT-001"

// testContract renders the document `auto spec change` writes, so these
// tests exercise the format the reader actually meets in a project.
type testContract struct {
	specID      string
	binding     string
	changeID    string
	declared    string
	effective   string
	tier        string
	decision    string
	newContract string
	acceptance  []string
	surface     []string
	verify      []string
	headerExtra string
}

// validTestContract is a low-risk bug fix over one module root: the case the
// compact route exists for.
func validTestContract() testContract {
	return testContract{
		specID:      testContractSpecID,
		declared:    string(KindBugfix),
		effective:   string(KindBugfix),
		tier:        string(RiskLow),
		decision:    string(DecisionCompact),
		newContract: "false",
		acceptance:  []string{"AC-001"},
		surface:     []string{"pkg/a/x.go"},
		verify:      []string{"go test ./pkg/a/..."},
	}
}

func (tc testContract) render() string {
	binding := tc.binding
	if binding == "" {
		binding = tc.specID
	}
	var sb strings.Builder
	sb.WriteString("# Change Contract: " + tc.specID + "\n\n---\n")
	sb.WriteString("schema: " + ChangeContractSchema + "\n")
	sb.WriteString("spec_id: " + tc.specID + "\n")
	if tc.changeID != "" {
		sb.WriteString("change_id: " + tc.changeID + "\n")
	}
	sb.WriteString("declared_class: " + tc.declared + "\n")
	sb.WriteString("effective_class: " + tc.effective + "\n")
	sb.WriteString("risk_tier: " + tc.tier + "\n")
	sb.WriteString("decision: " + tc.decision + "\n")
	sb.WriteString("new_exported_contract: " + tc.newContract + "\n")
	sb.WriteString("generated_at: 2026-09-06T12:00:00Z\n")
	sb.WriteString(tc.headerExtra)
	sb.WriteString("---\n\n## Referenced SPEC\n\n")
	sb.WriteString("- SPEC: `" + binding + "` (`.autopus/specs/" + binding + "/spec.md`)\n")
	if len(tc.acceptance) > 0 {
		sb.WriteString("- Acceptance criteria: `" + strings.Join(tc.acceptance, "`, `") + "`\n")
	}
	sb.WriteString("\nRequirements stay in the referenced SPEC.\n\n## Intended Surface\n\n")
	for _, path := range tc.surface {
		sb.WriteString("- `" + path + "`\n")
	}
	sb.WriteString("\nA change outside this surface invalidates the contract: re-run `auto spec change`.\n\n")
	sb.WriteString("## Verification Plan\n\n")
	for i, entry := range tc.verify {
		sb.WriteString(string(rune('1'+i)) + ". " + entry + "\n")
	}
	sb.WriteString("\n## Gate Applicability\n\n| gate | applicability | reason |\n|---|---|---|\n")
	sb.WriteString("| `validation` | `required` | mandatory safety gate |\n")
	return sb.String()
}

// contractProject lays out {root}/.autopus/specs/{SPEC} and returns both the
// project root and the SPEC directory a run would resolve.
func contractProject(t *testing.T) (root, specDir string) {
	t.Helper()
	root = t.TempDir()
	specDir = filepath.Join(root, ".autopus", "specs", testContractSpecID)
	require.NoError(t, os.MkdirAll(specDir, 0o755))
	return root, specDir
}

func writeContractAt(t *testing.T, root, dir, document string) string {
	t.Helper()
	contractDir := filepath.Join(root, ".autopus", "specs", dir)
	require.NoError(t, os.MkdirAll(contractDir, 0o755))
	path := filepath.Join(contractDir, ChangeContractFile)
	require.NoError(t, os.WriteFile(path, []byte(document), 0o600))
	return path
}

func authorize(t *testing.T, specDir string, changed ...string) CompactRouteDecision {
	t.Helper()
	return AuthorizeCompactRoute(specDir, testContractSpecID, nil, changed)
}

func TestAuthorizeCompactRoute_LowRiskContractOverItsOwnSurface(t *testing.T) {
	root, specDir := contractProject(t)
	writeContractAt(t, root, testContractSpecID, validTestContract().render())

	decision := authorize(t, specDir, "pkg/a/x.go")

	require.True(t, decision.Authorized, decision.Reason)
	assert.Equal(t, KindBugfix, decision.Contract.Risk.EffectiveClass)
	assert.Equal(t, []string{"AC-001"}, decision.Contract.AcceptanceIDs)
	assert.Equal(t, []string{"go test ./pkg/a/..."}, decision.Contract.VerificationPlan)
	assert.Contains(t, decision.Reason, "low-risk bugfix_existing_contract")
}

// The recorded tier is a claim. A contract that writes "low" over a
// high-risk class is internally inconsistent and authorizes nothing.
func TestAuthorizeCompactRoute_ForgedLowMetadataOverHighRiskClassIsRefused(t *testing.T) {
	for name, mutate := range map[string]func(tc *testContract){
		"high-risk effective class": func(tc *testContract) { tc.effective = string(KindFeature) },
		"high-risk declared class":  func(tc *testContract) { tc.declared = string(KindSecurityOrData) },
		"effective below declared":  func(tc *testContract) { tc.effective = string(KindDocsOnly) },
	} {
		t.Run(name, func(t *testing.T) {
			root, specDir := contractProject(t)
			contract := validTestContract()
			mutate(&contract)
			writeContractAt(t, root, testContractSpecID, contract.render())

			decision := authorize(t, specDir, "pkg/a/x.go")

			assert.False(t, decision.Authorized, decision.Reason)
			assert.NotEmpty(t, decision.Reason)
		})
	}
}

// The declaration plus the declared surface are reassessed with the same
// rules `auto spec change` used, so a low record over an escalating surface
// never shortens a run.
func TestAuthorizeCompactRoute_SurfaceReassessmentOverridesTheRecord(t *testing.T) {
	cases := map[string]func(tc *testContract){
		"security surface": func(tc *testContract) {
			tc.surface = []string{"backend/db/migrations/001.sql"}
		},
		"two production roots": func(tc *testContract) {
			tc.surface = []string{"pkg/a/x.go", "pkg/b/y.go"}
		},
		"public contract surface": func(tc *testContract) {
			tc.surface = []string{"api/service.proto"}
		},
		"new exported contract": func(tc *testContract) { tc.newContract = "true" },
		"declared test only touching production": func(tc *testContract) {
			tc.declared, tc.effective = string(KindTestOnly), string(KindTestOnly)
			tc.surface = []string{"pkg/a/x_test.go", "pkg/a/x.go"}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			root, specDir := contractProject(t)
			contract := validTestContract()
			mutate(&contract)
			writeContractAt(t, root, testContractSpecID, contract.render())

			decision := authorize(t, specDir, contract.surface...)

			assert.False(t, decision.Authorized, decision.Reason)
			assert.True(t, decision.Contract.Risk.Escalated() ||
				decision.Contract.Risk.EffectiveClass != decision.Contract.Recorded.EffectiveClass,
				"the reassessment, not the record, decided: %s", decision.Reason)
		})
	}
}

// What the tree actually changed bounds the route as much as what the
// contract declared.
func TestAuthorizeCompactRoute_ChangedPathsOutsideTheSurfaceKeepTheFullRoute(t *testing.T) {
	root, specDir := contractProject(t)
	contract := validTestContract()
	contract.surface = []string{"pkg/a"}
	writeContractAt(t, root, testContractSpecID, contract.render())

	inside := authorize(t, specDir, "pkg/a/x.go", "pkg/a/nested/y.go")
	require.True(t, inside.Authorized, inside.Reason)

	bookkeeping := authorize(t, specDir,
		"pkg/a/x.go", ".autopus/specs/"+testContractSpecID+"/change.md",
		".autopus/pipeline-state/"+testContractSpecID+".yaml")
	assert.True(t, bookkeeping.Authorized,
		"the contract and its receipts are records of the change, not the change: %s", bookkeeping.Reason)

	outside := authorize(t, specDir, "pkg/a/x.go", "pkg/b/y.go")
	assert.False(t, outside.Authorized)
	assert.Contains(t, outside.Reason, "pkg/b/y.go")
}

// A surface entry coarse enough to cover half the tree does not turn the
// work inside it into a low-risk change: the paths actually changed are
// assessed under the same declared class.
func TestAuthorizeCompactRoute_CoarseSurfaceDoesNotLaunderTheActualChangeSet(t *testing.T) {
	root, specDir := contractProject(t)
	contract := validTestContract()
	contract.surface = []string{"pkg"}
	writeContractAt(t, root, testContractSpecID, contract.render())

	oneRoot := authorize(t, specDir, "pkg/a/x.go")
	require.True(t, oneRoot.Authorized, oneRoot.Reason)

	twoRoots := authorize(t, specDir, "pkg/a/x.go", "pkg/b/y.go")
	assert.False(t, twoRoots.Authorized)
	assert.Contains(t, twoRoots.Reason, EscalationMultiDomain)

	securitySurface := authorize(t, specDir, "pkg/auth/token.go")
	assert.False(t, securitySurface.Authorized)
	assert.Contains(t, securitySurface.Reason, EscalationSecuritySurface)
}

// A contract that refuses itself is honoured: an escalated record never takes
// the compact route even when its surface would have qualified.
func TestAuthorizeCompactRoute_EscalatedRecordIsHonoured(t *testing.T) {
	root, specDir := contractProject(t)
	contract := validTestContract()
	contract.tier, contract.decision = string(RiskHigh), string(DecisionEscalate)
	writeContractAt(t, root, testContractSpecID, contract.render())

	decision := authorize(t, specDir, "pkg/a/x.go")

	assert.False(t, decision.Authorized)
	assert.Contains(t, decision.Reason, string(DecisionEscalate))
}
