package intake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/harneval"
)

// The S6 candidate of fingerprint Y and the files its promotion writes.
const (
	promoteCandidateID = "GTC-023e9302ff0b"
	promoteTaskID      = "GT-INC-023E9302"
	promoteCandidateAt = IntakeDir + "/" + promoteCandidateID + ".json"
	promoteTaskAt      = SurfaceTaskDir + "/" + promoteTaskID + ".json"
	promoteLinkAt      = PromotedDir + "/" + promoteTaskID + ".json"
	promoteAgentDir    = harneval.EvalRoot + "/tasks/agent"
	promoteCorpus      = `[{"id":"a01","prompt":"fix it"}]` + "\n"
)

// errPromoteCrash stops a promotion at an injected interruption point.
var errPromoteCrash = errors.New("simulated crash")

// promoteManifest is a harness_golden_set.v1 manifest the SPEC-HARNEVAL-001
// strict loader accepts, declaring the surface and agent active paths.
func promoteManifest(activePaths ...string) map[string]any {
	if len(activePaths) == 0 {
		activePaths = []string{SurfaceTaskDir, promoteAgentDir}
	}
	return map[string]any{
		"schema_version": harneval.SetSchemaV1,
		"set_version":    "1",
		"active_paths":   activePaths,
		"floors":         map[string]any{"surface_tasks": 1, "agent_tasks": 1},
		"live": map[string]any{
			"k": 2, "threshold_bp": -1000, "completeness_floor": 0.9, "max_agent_runs": 96,
			"trial_timeout_seconds": 180, "workspace_revision": strings.Repeat("a", 40),
			"baseline_ref": "v0.50.122", "model": "gpt-test",
		},
		"pins": map[string]any{
			"generator_version": "v0.50.123", "project_name": "harneval-fixture",
			"codex_model_catalog": "", "codex_cli_version": "codex-cli 0.160.0",
			"opencode_cli_version": "1.18.7",
		},
	}
}

// promoteAgentTask is an agent task with a valid corpus_ref and expected_tests.
func promoteAgentTask() map[string]any {
	sum := sha256.Sum256([]byte(promoteCorpus))
	task := validTaskDoc("GT-AG-001")
	task["kind"] = harneval.KindAgent
	task["category"] = "skill_selection_precedence"
	task["corpus_ref"] = map[string]any{"file": "bench/corpus_a.json", "task_id": "a01", "file_sha256": hex.EncodeToString(sum[:])}
	task["expected_tests"] = []any{"TestVersionMismatchWinsOverUnknown"}
	task["provenance"] = map[string]any{"kind": "benchmark", "ref": "bench/corpus_a.json#a01"}
	return task
}

// newPromoteProject writes the S6 golden set (surface task GT-FIX-A, agent
// task GT-AG-001 and its corpus) and lets a real intake run create the
// candidate of the Y group L-002 (representative) and L-1000.
func newPromoteProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRecord(t, root, harneval.ManifestPath, promoteManifest())
	writeRecord(t, root, SurfaceTaskDir+"/GT-FIX-A.json", validTaskDoc("GT-FIX-A"))
	writeFile(t, root, "bench/corpus_a.json", promoteCorpus)
	writeRecord(t, root, promoteAgentDir+"/GT-AG-001.json", promoteAgentTask())
	representative, member := entryY("L-002"), entryY("L-1000")
	representative.Expected, representative.Actual, representative.Repro = "y", "ya", "auto init"
	member.Expected, member.Actual, member.Repro = "y2", "ya2", "r2"
	result := runIntake(t, root, []Entry{member, representative}, nil)
	require.Equal(t, promoteCandidateID, result.Rows[0].CandidateID)
	return root
}

// editCandidate rewrites the candidate the way a person edits it.
func editCandidate(t *testing.T, root string, edit func(*Candidate)) {
	t.Helper()
	candidate := decodeCandidate(t, root, promoteCandidateID)
	edit(&candidate)
	writeRecord(t, root, promoteCandidateAt, candidate)
}

// completeDraft writes the S6 category and assertion into the draft task.
func completeDraft(c *Candidate) {
	c.Task.Category = "hooks_settings"
	c.Task.Assertions = []harneval.Assertion{{Kind: harneval.AssertFileExists, Platform: "codex", Path: ".codex/hooks.json"}}
}

// newCompletedProject is newPromoteProject with the draft completed.
func newCompletedProject(t *testing.T) string {
	t.Helper()
	root := newPromoteProject(t)
	editCandidate(t, root, completeDraft)
	return root
}

// promoteSurface is a platform adapter writing fixed files, or failing with
// err, so a promotion evaluates current_outcome on a small generated surface.
type promoteSurface struct {
	adapter.PlatformAdapter
	name, root string
	files      []string
	err        error
}

func (s promoteSurface) Name() string { return s.name }

func (s promoteSurface) Generate(context.Context, *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	if s.err != nil {
		return nil, s.err
	}
	generated := &adapter.PlatformFiles{}
	for _, rel := range s.files {
		full := filepath.Join(s.root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, []byte("fixture "+rel+"\n"), 0o644); err != nil {
			return nil, err
		}
		generated.Files = append(generated.Files, adapter.FileMapping{TargetPath: rel})
	}
	return generated, nil
}

// promoteRun returns run seams whose codex surface holds files and whose
// template check is clean.
func promoteRun(files ...string) harneval.RunOptions {
	return harneval.RunOptions{
		Adapters: func(root string, _ harneval.Pins, _ []byte) []adapter.PlatformAdapter {
			return []adapter.PlatformAdapter{promoteSurface{name: "codex", root: root, files: files}}
		},
		StaleCheck: func(string) ([]string, error) { return nil, nil },
	}
}

// runPromote promotes the S6 candidate below root on a codex surface holding
// .codex/hooks.json; edit adjusts the request first.
func runPromote(root string, edit func(*PromoteRequest)) (PromoteResult, error) {
	req := PromoteRequest{Root: root, CandidateID: promoteCandidateID, Redactor: noRedaction, Run: promoteRun(".codex/hooks.json")}
	if edit != nil {
		edit(&req)
	}
	return Promote(context.Background(), req)
}

// requirePromoteRefusal asserts err is a *RunError with reason and detail.
func requirePromoteRefusal(t *testing.T, err error, reason, detail string) {
	t.Helper()
	var refusal *RunError
	require.ErrorAs(t, err, &refusal, "error %v", err)
	assert.Equal(t, reason, refusal.Reason, "error %v", err)
	assert.Equal(t, detail, refusal.Detail, "error %v", err)
}

// removeRel deletes the file at the slash-separated path rel below root.
func removeRel(t *testing.T, root, rel string) {
	t.Helper()
	require.NoError(t, os.Remove(filepath.Join(root, filepath.FromSlash(rel))))
}

// tempNames lists every publication temp file below root.
func tempNames(t *testing.T, root string) []string {
	t.Helper()
	var temps []string
	for rel := range treeDigest(t, root) {
		if strings.Contains(filepath.Base(rel), ".json.tmp-") {
			temps = append(temps, rel)
		}
	}
	return temps
}
