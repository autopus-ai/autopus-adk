package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/harneval"
)

// Surface files the fake claude-code adapter can generate; surface tasks
// assert file_exists on one of them.
const (
	harnessRouter = ".claude/skills/auto/SKILL.md"
	harnessGone   = ".claude/skills/gone/SKILL.md"
	harnessHook   = ".claude/hooks/guard.sh"
)

// Expectation and set digests of the task documents this file writes. They
// were computed outside Go by the pkg/harneval scratch oracle (hashlib over
// hand-written canonical JSON) for byte-identical task documents.
const (
	harnessRouterDigest   = "59ba8cba7c76c869aa11acfe0c0c8540d70f15d6cf6a436a69e3c3d8a9a81905"
	harnessGoneDigest     = "25230fe5f65e5d0bd0d50beb73dbf56f71a4eba6a2d670e5a57436bdefcd0865"
	harnessAgentDigest    = "0ef15594db1cf6b56beb4680f3b1be6f082cd566c112b3e70d023a76e2266188"
	harnessStandardSet    = "b6e83ca287658ef51dd27772f0bb887a1fc39ee993a3b9fa0d64252a4514b2b7"
	harnessStandardAgents = "95681c12046c448b2746b8c1e036dff6e8e22bc9abeb2b67971f8f154a463818"
)

const harnessCorpus = `[{"id":"a01","prompt":"fix it"}]` + "\n"

// harnessTree is a throwaway repository tree holding a golden set.
type harnessTree struct {
	t    *testing.T
	root string
}

// newHarnessTree writes the manifest, the agent task GT-AG-001 with its
// corpus, and one active surface task per id asserting file_exists on path.
func newHarnessTree(t *testing.T, surface map[string]string) *harnessTree {
	t.Helper()
	tree := &harnessTree{t: t, root: t.TempDir()}
	tree.writeJSON(harneval.ManifestPath, harnessManifest())
	tree.write("bench/corpus_a.json", harnessCorpus)
	tree.writeJSON(harnessAgentPath, harnessAgentTask())
	for id, path := range surface {
		tree.writeJSON(harnessSurfacePath(id), harnessSurfaceTask(id, path))
	}
	return tree
}

// standardHarnessTree is the pkg/harneval standard set: GT-FIX-A, B, and C
// on the router plus GT-AG-001.
func standardHarnessTree(t *testing.T) *harnessTree {
	return newHarnessTree(t, map[string]string{"GT-FIX-A": harnessRouter, "GT-FIX-B": harnessRouter, "GT-FIX-C": harnessRouter})
}

func (h *harnessTree) write(rel, body string) {
	h.t.Helper()
	full := filepath.Join(h.root, filepath.FromSlash(rel))
	require.NoError(h.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(h.t, os.WriteFile(full, []byte(body), 0o644))
}

func (h *harnessTree) writeJSON(rel string, value any) {
	h.t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	require.NoError(h.t, err)
	h.write(rel, string(data)+"\n")
}

func (h *harnessTree) remove(rel string) {
	h.t.Helper()
	require.NoError(h.t, os.Remove(filepath.Join(h.root, filepath.FromSlash(rel))))
}

// baseline returns the committed baseline bytes and their SHA-256 hex.
func (h *harnessTree) baseline() (string, string) {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.root, filepath.FromSlash(harneval.BaselinePath)))
	require.NoError(h.t, err)
	sum := sha256.Sum256(data)
	return string(data), hex.EncodeToString(sum[:])
}

// baselineRows decodes the committed baseline rows keyed by id, in file order.
func (h *harnessTree) baselineRows() ([]string, map[string]map[string]any) {
	h.t.Helper()
	body, _ := h.baseline()
	var doc struct {
		Rows []map[string]any `json:"rows"`
	}
	require.NoError(h.t, json.Unmarshal([]byte(body), &doc))
	ids := make([]string, 0, len(doc.Rows))
	rows := make(map[string]map[string]any, len(doc.Rows))
	for _, row := range doc.Rows {
		ids = append(ids, row["id"].(string))
		rows[row["id"].(string)] = row
	}
	return ids, rows
}

const harnessAgentPath = "evals/harness/tasks/agent/GT-AG-001.json"

func harnessSurfacePath(id string) string { return "evals/harness/tasks/surface/" + id + ".json" }

func harnessManifest() map[string]any {
	return map[string]any{
		"schema_version": harneval.SetSchemaV1,
		"set_version":    "1",
		"active_paths":   []any{"evals/harness/tasks/surface", "evals/harness/tasks/agent"},
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

func harnessSurfaceTask(id, path string) map[string]any {
	return map[string]any{
		"schema_version": harneval.TaskSchemaV1,
		"id":             id,
		"kind":           harneval.KindSurface,
		"category":       "routing",
		"intent":         "the router reaches its plan detail",
		"outcome":        "the plan route stays reachable",
		"variants":       []any{},
		"assertions":     []any{map[string]any{"kind": harneval.AssertFileExists, "platform": "claude-code", "path": path}},
		"provenance":     map[string]any{"kind": "manual", "ref": "SPEC-HARNEVAL-001"},
		"status":         map[string]any{"state": harneval.StateActive, "reason": ""},
	}
}

func harnessAgentTask() map[string]any {
	task := harnessSurfaceTask("GT-AG-001", harnessRouter)
	sum := sha256.Sum256([]byte(harnessCorpus))
	task["kind"] = harneval.KindAgent
	task["category"] = "skill_selection_precedence"
	task["corpus_ref"] = map[string]any{"file": "bench/corpus_a.json", "task_id": "a01", "file_sha256": hex.EncodeToString(sum[:])}
	task["expected_tests"] = []any{"TestVersionMismatchWinsOverUnknown"}
	task["provenance"] = map[string]any{"kind": "benchmark", "ref": "bench/corpus_a.json#a01"}
	return task
}

// harnessFakeAdapter is a claude-code surface made of fixed files.
type harnessFakeAdapter struct {
	adapter.PlatformAdapter
	root  string
	files []string
}

func (a harnessFakeAdapter) Name() string { return "claude-code" }

func (a harnessFakeAdapter) Generate(context.Context, *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	generated := &adapter.PlatformFiles{}
	for _, rel := range a.files {
		full := filepath.Join(a.root, filepath.FromSlash(rel))
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

// harnessDeps runs the production pipeline on a fake claude-code surface that
// holds files, with the template check skipped and the clock fixed. A run that
// generates swaps PATH and HOME for the process, so such tests stay serial.
func harnessDeps(files ...string) evalHarnessDeps {
	return evalHarnessDeps{run: harneval.RunOptions{
		Adapters: func(root string, _ harneval.Pins, _ []byte) []adapter.PlatformAdapter {
			return []adapter.PlatformAdapter{harnessFakeAdapter{root: root, files: files}}
		},
		StaleCheck: func(string) ([]string, error) { return nil, nil },
		Now:        func() time.Time { return time.Date(2026, 10, 7, 10, 2, 3, 0, time.FixedZone("KST", 9*3600)) },
	}}
}

// harnessOutcome is one command invocation as a process would see it.
type harnessOutcome struct {
	stdout, stderr string
	code           int
}

// runHarness executes `auto eval harness <args>` in-process and maps the
// returned error to the exit code and stderr line Execute would produce.
func runHarness(t *testing.T, deps evalHarnessDeps, args ...string) harnessOutcome {
	t.Helper()
	cmd := newEvalCmdWith(deps)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"harness"}, args...))
	err := cmd.Execute()
	code := 0
	if err != nil {
		code = exitCodeForError(err)
		if !isJSONFatalError(err) {
			stderr.WriteString("Error: " + err.Error() + "\n")
		}
	}
	return harnessOutcome{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// harnessDoc decodes stdout as exactly one JSON document.
func harnessDoc(t *testing.T, stdout string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var doc map[string]any
	require.NoError(t, decoder.Decode(&doc), "stdout: %q", stdout)
	require.ErrorIs(t, decoder.Decode(&json.RawMessage{}), io.EOF, "stdout holds one document only")
	return doc
}
