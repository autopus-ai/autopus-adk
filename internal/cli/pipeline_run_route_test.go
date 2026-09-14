package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// routeContractDocument renders the document `auto spec change` writes, which
// is the only shape the route resolver accepts.
func routeContractDocument(specID, declared, tier, decision string, surface ...string) string {
	document := "# Change Contract: " + specID + "\n\n---\n" +
		"schema: autopus.change-contract.v1\n" +
		"spec_id: " + specID + "\n" +
		"declared_class: " + declared + "\n" +
		"effective_class: " + declared + "\n" +
		"risk_tier: " + tier + "\n" +
		"decision: " + decision + "\n" +
		"new_exported_contract: false\n" +
		"generated_at: 2026-09-06T12:00:00Z\n---\n\n" +
		"## Referenced SPEC\n\n" +
		"- SPEC: `" + specID + "` (`.autopus/specs/" + specID + "/spec.md`)\n" +
		"- Acceptance criteria: `AC-001`\n\n" +
		"## Intended Surface\n\n"
	for _, path := range surface {
		document += "- `" + path + "`\n"
	}
	return document + "\n## Verification Plan\n\n1. go test ./pkg/a/...\n"
}

func writeRouteContract(t *testing.T, root, dir, document string) {
	t.Helper()
	contractDir := filepath.Join(root, ".autopus", "specs", dir)
	require.NoError(t, os.MkdirAll(contractDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(contractDir, "change.md"), []byte(document), 0o600))
}

func writeRouteSurfaceFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// commitRouteProject makes the working tree a committed baseline, so the
// change set the resolver reads is exactly what the test changes afterwards.
func commitRouteProject(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-qm", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

// runPipelineRouteDryRun drives `pipeline run <spec> --dry-run` in the
// current directory and returns the phases the run put on its checkpoint
// together with the route line it reported.
func runPipelineRouteDryRun(t *testing.T, specID string) (map[string]pipeline.CheckpointStatus, string) {
	t.Helper()
	cmd := newPipelineRunCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{specID, "--dry-run"})
	require.NoError(t, cmd.Execute())

	cp, err := pipeline.LoadFile(specCheckpointPath(specID))
	require.NoError(t, err)
	require.NotNil(t, cp, "a dry run still records what it would dispatch")
	return cp.TaskStatus, errOut.String()
}

func fullRoutePhaseIDs() []string {
	return []string{
		string(pipeline.PhasePlan), string(pipeline.PhaseTestScaffold),
		string(pipeline.PhaseImplement), string(pipeline.PhaseValidate),
		string(pipeline.PhaseReview),
	}
}

// A low-risk contract whose declared surface still covers what the tree
// actually changed really shortens the run.
func TestPipelineRunCmd_AuthorizedCompactContractDispatchesFewerPhases(t *testing.T) {
	const specID = "SPEC-ROUTECLI-001"
	root := t.TempDir()
	chdirForTest(t, root)
	writePipelineOwnerSpec(t, root, specID)
	writeRouteSurfaceFile(t, root, "pkg/a/x.go", "package a\n")
	writeRouteContract(t, root, specID,
		routeContractDocument(specID, "bugfix_existing_contract", "low", "compact_contract", "pkg/a"))
	commitRouteProject(t, root)
	writeRouteSurfaceFile(t, root, "pkg/a/x.go", "package a\n\n// fix\n")

	statuses, reported := runPipelineRouteDryRun(t, specID)

	assert.ElementsMatch(t, []string{
		string(pipeline.PhaseImplement), string(pipeline.PhaseValidate), string(pipeline.PhaseReview),
	}, mapKeys(statuses), "the compact route dispatches implementation, validation, and review only")
	assert.Contains(t, reported, "Pipeline route: compact")
	assert.Contains(t, reported, "change.md")
	assert.Contains(t, reported, "plan and test_scaffold are not dispatched")
}

// The contract may live in a sibling CHG-<id> directory that names the SPEC.
func TestPipelineRunCmd_SiblingChangeContractSelectsTheCompactRoute(t *testing.T) {
	const specID = "SPEC-ROUTECLI-002"
	root := t.TempDir()
	chdirForTest(t, root)
	writePipelineOwnerSpec(t, root, specID)
	writeRouteSurfaceFile(t, root, "pkg/a/x.go", "package a\n")
	writeRouteContract(t, root, "CHG-ROUTECLI-02",
		routeContractDocument(specID, "bugfix_existing_contract", "low", "compact_contract", "pkg/a/x.go"))
	commitRouteProject(t, root)

	statuses, reported := runPipelineRouteDryRun(t, specID)

	assert.Len(t, statuses, 3)
	assert.NotContains(t, mapKeys(statuses), string(pipeline.PhasePlan))
	assert.Contains(t, reported, "Pipeline route: compact")
	assert.Contains(t, reported, "CHG-ROUTECLI-02")
}

// Work that escaped the declared surface is not the work the contract graded,
// so the run keeps every phase.
func TestPipelineRunCmd_ChangedPathOutsideTheSurfaceKeepsTheFullRoute(t *testing.T) {
	const specID = "SPEC-ROUTECLI-003"
	root := t.TempDir()
	chdirForTest(t, root)
	writePipelineOwnerSpec(t, root, specID)
	writeRouteSurfaceFile(t, root, "pkg/a/x.go", "package a\n")
	writeRouteContract(t, root, specID,
		routeContractDocument(specID, "bugfix_existing_contract", "low", "compact_contract", "pkg/a/x.go"))
	commitRouteProject(t, root)
	writeRouteSurfaceFile(t, root, "pkg/b/y.go", "package b\n")

	statuses, reported := runPipelineRouteDryRun(t, specID)

	assert.ElementsMatch(t, fullRoutePhaseIDs(), mapKeys(statuses))
	assert.Contains(t, reported, "Pipeline route: full")
	assert.Contains(t, reported, "pkg/b/y.go")
}

// An undeterminable change set is not evidence of a small change: without git
// (and without a supplied change set) the run cannot bound what changed.
func TestPipelineRunCmd_UndeterminableChangeSetKeepsTheFullRoute(t *testing.T) {
	const specID = "SPEC-ROUTECLI-004"
	root := t.TempDir()
	chdirForTest(t, root)
	writePipelineOwnerSpec(t, root, specID)
	writeRouteContract(t, root, specID,
		routeContractDocument(specID, "bugfix_existing_contract", "low", "compact_contract", "pkg/a/x.go"))

	statuses, reported := runPipelineRouteDryRun(t, specID)

	assert.ElementsMatch(t, fullRoutePhaseIDs(), mapKeys(statuses))
	assert.Contains(t, reported, "Pipeline route: full")
	assert.Contains(t, reported, "actual change set undeterminable")
}

// Absent, escalated, duplicated, and damaged contracts all keep every phase.
func TestPipelineRunCmd_UnauthorizedContractsKeepTheFullRoute(t *testing.T) {
	tests := []struct {
		name    string
		specID  string
		arrange func(t *testing.T, root, specID string)
		want    string
	}{
		{
			name: "absent contract", specID: "SPEC-ROUTECLI-005",
			want: "no compact change contract",
		},
		{
			name: "escalated contract", specID: "SPEC-ROUTECLI-006",
			arrange: func(t *testing.T, root, specID string) {
				writeRouteContract(t, root, specID, routeContractDocument(
					specID, "feature", "high", "escalate_to_full_spec", "pkg/a/x.go"))
			},
			want: "escalate_to_full_spec",
		},
		{
			name: "duplicate contracts", specID: "SPEC-ROUTECLI-007",
			arrange: func(t *testing.T, root, specID string) {
				document := routeContractDocument(
					specID, "bugfix_existing_contract", "low", "compact_contract", "pkg/a/x.go")
				writeRouteContract(t, root, specID, document)
				writeRouteContract(t, root, "CHG-DUPLICATE-07", document)
			},
			want: "2 change contracts",
		},
		{
			name: "damaged contract", specID: "SPEC-ROUTECLI-008",
			arrange: func(t *testing.T, root, specID string) {
				writeRouteContract(t, root, specID, "---\nschema: other.v1\n---\n")
			},
			want: "change contract unusable",
		},
		{
			name: "contract with no verification plan", specID: "SPEC-ROUTECLI-009",
			arrange: func(t *testing.T, root, specID string) {
				document := routeContractDocument(
					specID, "bugfix_existing_contract", "low", "compact_contract", "pkg/a/x.go")
				writeRouteContract(t, root, specID, document[:len(document)-len("1. go test ./pkg/a/...\n")])
			},
			want: "change contract unusable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			chdirForTest(t, root)
			writePipelineOwnerSpec(t, root, test.specID)
			writeRouteSurfaceFile(t, root, "pkg/a/x.go", "package a\n")
			if test.arrange != nil {
				test.arrange(t, root, test.specID)
			}
			commitRouteProject(t, root)

			statuses, reported := runPipelineRouteDryRun(t, test.specID)

			assert.ElementsMatch(t, fullRoutePhaseIDs(), mapKeys(statuses))
			assert.Contains(t, reported, "Pipeline route: full")
			assert.Contains(t, reported, test.want)
		})
	}
}
