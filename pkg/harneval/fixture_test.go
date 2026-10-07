package harneval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixtureCorpus is a stand-in benchmark corpus file referenced by agent tasks.
const fixtureCorpus = `[{"id":"a01","prompt":"fix it"}]` + "\n"

// fixture is a throwaway repository tree holding a golden set.
type fixture struct {
	t    *testing.T
	root string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return &fixture{t: t, root: t.TempDir()}
}

func (f *fixture) write(rel, body string) {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	require.NoError(f.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(f.t, os.WriteFile(full, []byte(body), 0o644))
}

func (f *fixture) writeJSON(rel string, value any) {
	f.t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	require.NoError(f.t, err)
	f.write(rel, string(data)+"\n")
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

// standard writes a valid set: three active surface tasks, one agent task, and
// the corpus file the agent task pins by digest.
func (f *fixture) standard() {
	f.t.Helper()
	f.writeJSON(ManifestPath, validManifest())
	for _, id := range []string{"GT-FIX-A", "GT-FIX-B", "GT-FIX-C"} {
		f.writeJSON(surfacePath(id), surfaceTask(id))
	}
	f.write("bench/corpus_a.json", fixtureCorpus)
	f.writeJSON(agentPath("GT-AG-001"), agentTask("GT-AG-001"))
}

func surfacePath(id string) string { return "evals/harness/tasks/surface/" + id + ".json" }
func agentPath(id string) string   { return "evals/harness/tasks/agent/" + id + ".json" }

func sha256Of(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func validManifest() map[string]any {
	return map[string]any{
		"schema_version": SetSchemaV1,
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

func surfaceTask(id string) map[string]any {
	return map[string]any{
		"schema_version": TaskSchemaV1,
		"id":             id,
		"kind":           KindSurface,
		"category":       "routing",
		"intent":         "the router reaches its plan detail",
		"outcome":        "the plan route stays reachable",
		"variants":       []any{},
		"assertions": []any{map[string]any{
			"kind": AssertFileExists, "platform": "claude-code", "path": ".claude/skills/auto/SKILL.md",
		}},
		"provenance": map[string]any{"kind": "manual", "ref": "SPEC-HARNEVAL-001"},
		"status":     map[string]any{"state": StateActive, "reason": ""},
	}
}

func agentTask(id string) map[string]any {
	task := surfaceTask(id)
	task["kind"] = KindAgent
	task["category"] = "skill_selection_precedence"
	task["corpus_ref"] = map[string]any{
		"file": "bench/corpus_a.json", "task_id": "a01", "file_sha256": sha256Of(fixtureCorpus),
	}
	task["expected_tests"] = []any{"TestVersionMismatchWinsOverUnknown"}
	task["provenance"] = map[string]any{"kind": "benchmark", "ref": "bench/corpus_a.json#a01"}
	return task
}

// requireInvalid asserts err is an *InvalidError with exactly the detail code.
func requireInvalid(t *testing.T, err error, detail string) {
	t.Helper()
	require.Error(t, err)
	var invalid *InvalidError
	require.ErrorAs(t, err, &invalid, "error %v is not an InvalidError", err)
	require.Equal(t, detail, invalid.Detail, "error: %v", err)
}
