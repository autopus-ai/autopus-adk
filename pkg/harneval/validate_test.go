package harneval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeTask_FieldContract_RejectsWithExactDetail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(task map[string]any)
		detail string
	}{
		{"wrong schema", func(m map[string]any) { m["schema_version"] = "harness_golden_task.v2" }, DetailFieldInvalid},
		{"bad id", func(m map[string]any) { m["id"] = "gt-lower" }, DetailFieldInvalid},
		{"short id", func(m map[string]any) { m["id"] = "GT-AB" }, DetailFieldInvalid},
		{"bad kind", func(m map[string]any) { m["kind"] = "live" }, DetailFieldInvalid},
		{"empty outcome", func(m map[string]any) { m["outcome"] = " " }, DetailFieldInvalid},
		{"empty intent", func(m map[string]any) { m["intent"] = "" }, DetailFieldInvalid},
		{"empty category", func(m map[string]any) { m["category"] = "" }, DetailFieldInvalid},
		{"bad provenance", func(m map[string]any) { m["provenance"] = map[string]any{"kind": "guess", "ref": "x"} }, DetailFieldInvalid},
		{"empty provenance ref", func(m map[string]any) { m["provenance"] = map[string]any{"kind": "manual", "ref": ""} }, DetailFieldInvalid},
		{"bad state", func(m map[string]any) { m["status"] = map[string]any{"state": "paused", "reason": ""} }, DetailFieldInvalid},
		{"retired without reason", func(m map[string]any) { m["status"] = map[string]any{"state": StateRetired, "reason": ""} }, DetailFieldInvalid},
		{"unknown override", func(m map[string]any) {
			m["variants"] = []any{map[string]any{"name": "x", "overrides": map[string]any{"hooks.pre_commit_lore": false}}}
		}, DetailFieldInvalid},
		{"override wrong type", func(m map[string]any) {
			m["variants"] = []any{map[string]any{"name": "x", "overrides": map[string]any{OverridePreCommitArch: "no"}}}
		}, DetailFieldInvalid},
		{"duplicate variant name", func(m map[string]any) {
			v := map[string]any{"name": "off", "overrides": map[string]any{OverridePreCommitArch: false}}
			m["variants"] = []any{v, v}
		}, DetailFieldInvalid},
		{"surface with corpus ref", func(m map[string]any) {
			m["corpus_ref"] = map[string]any{"file": "a.json", "task_id": "a01", "file_sha256": strings.Repeat("0", 64)}
		}, DetailFieldInvalid},
		{"surface with expected tests", func(m map[string]any) { m["expected_tests"] = []any{"TestX"} }, DetailFieldInvalid},
		{"syntax error", nil, DetailMalformedJSON},
		{"wrong type", func(m map[string]any) { m["id"] = 5 }, DetailFieldInvalid},
		{"assertion wrong type", func(m map[string]any) {
			m["assertions"] = []any{map[string]any{"kind": AssertContains, "platform": "codex", "path": "AGENTS.md", "needle": 5}}
		}, DetailAssertionFieldInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			task := surfaceTask("GT-FIX-A")
			body := "{"
			if tc.mutate != nil {
				tc.mutate(task)
				body = mustJSON(t, task)
			}
			_, err := DecodeTask([]byte(body))
			requireInvalid(t, err, tc.detail)
		})
	}
}

func TestDecodeTask_AssertionFieldContract(t *testing.T) {
	t.Parallel()
	valid := map[string]map[string]any{
		"exists":   {"kind": AssertFileExists, "platform": "codex", "path": "AGENTS.md"},
		"contains": {"kind": AssertContains, "platform": "omp", "path": ".omp/x.md", "needle": "plan"},
		"json":     {"kind": AssertJSONPathPresent, "platform": "claude-code", "path": ".claude/settings.json", "json_path": "hooks.PreToolUse[*].matcher", "value_contains": "Bash"},
		"route":    {"kind": AssertRouteDetail, "platform": "claude-code", "path": ".claude/skills/auto/SKILL.md", "route": "plan", "detail": ".claude/skills/auto-plan/SKILL.md"},
		"parity": {"kind": AssertSectionParity, "heading": "## Plan", "files": []any{
			map[string]any{"platform": "codex", "path": "a.md"}, map[string]any{"platform": "omp", "path": "b.md"}}},
	}
	for name, assertion := range valid {
		task := surfaceTask("GT-FIX-A")
		task["assertions"] = []any{assertion}
		_, err := DecodeTask([]byte(mustJSON(t, task)))
		require.NoError(t, err, name)
	}
	invalid := []map[string]any{
		{"kind": AssertFileExists, "platform": "vscode", "path": "AGENTS.md"},
		{"kind": AssertFileExists, "platform": "codex", "path": "../AGENTS.md"},
		{"kind": AssertFileExists, "platform": "codex", "path": "/etc/passwd"},
		{"kind": AssertFileExists, "platform": "codex", "path": "a//b"},
		{"kind": AssertFileExists, "platform": "codex", "path": "AGENTS.md", "needle": "x"},
		{"kind": AssertContains, "platform": "codex", "path": "AGENTS.md"},
		{"kind": AssertJSONPathAbsent, "platform": "codex", "path": "a.json", "json_path": "a..b"},
		{"kind": AssertJSONPathAbsent, "platform": "codex", "path": "a.json", "json_path": "a", "value_contains": ""},
		{"kind": AssertRouteDetail, "platform": "codex", "path": "r.md", "route": "plan"},
		{"kind": AssertRouteDetail, "platform": "codex", "path": "r.md", "route": "plan", "detail": "../x.md"},
		{"kind": AssertSectionParity, "heading": "## Plan", "files": []any{map[string]any{"platform": "codex", "path": "a.md"}}},
		{"kind": AssertSectionParity, "heading": "Plan", "files": []any{
			map[string]any{"platform": "codex", "path": "a.md"}, map[string]any{"platform": "omp", "path": "b.md"}}},
		{"kind": AssertSectionParity, "platform": "codex", "path": "a.md", "heading": "## Plan", "files": []any{
			map[string]any{"platform": "codex", "path": "a.md"}, map[string]any{"platform": "omp", "path": "b.md"}}},
	}
	for index, assertion := range invalid {
		task := surfaceTask("GT-FIX-A")
		task["assertions"] = []any{assertion}
		_, err := DecodeTask([]byte(mustJSON(t, task)))
		assert.Error(t, err, "invalid assertion %d: %v", index, assertion)
		requireInvalid(t, err, DetailAssertionFieldInvalid)
	}
}

func TestDecodeManifest_PolicyAndPins(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(m map[string]any)
		detail string
	}{
		{"k zero", func(m map[string]any) { m["live"].(map[string]any)["k"] = 0 }, DetailPolicyOutOfRange},
		{"threshold below", func(m map[string]any) { m["live"].(map[string]any)["threshold_bp"] = -10001 }, DetailPolicyOutOfRange},
		{"completeness zero", func(m map[string]any) { m["live"].(map[string]any)["completeness_floor"] = 0 }, DetailPolicyOutOfRange},
		{"completeness above", func(m map[string]any) { m["live"].(map[string]any)["completeness_floor"] = 1.5 }, DetailPolicyOutOfRange},
		{"max runs", func(m map[string]any) { m["live"].(map[string]any)["max_agent_runs"] = 0 }, DetailPolicyOutOfRange},
		{"timeout", func(m map[string]any) { m["live"].(map[string]any)["trial_timeout_seconds"] = 0 }, DetailPolicyOutOfRange},
		{"surface floor", func(m map[string]any) { m["floors"].(map[string]any)["surface_tasks"] = 0 }, DetailPolicyOutOfRange},
		{"agent floor", func(m map[string]any) { m["floors"].(map[string]any)["agent_tasks"] = 0 }, DetailPolicyOutOfRange},
		{"revision", func(m map[string]any) { m["live"].(map[string]any)["workspace_revision"] = "abc" }, DetailFieldInvalid},
		{"revision upper", func(m map[string]any) { m["live"].(map[string]any)["workspace_revision"] = strings.Repeat("A", 40) }, DetailFieldInvalid},
		{"baseline ref", func(m map[string]any) { m["live"].(map[string]any)["baseline_ref"] = "" }, DetailFieldInvalid},
		{"model", func(m map[string]any) { m["live"].(map[string]any)["model"] = "" }, DetailFieldInvalid},
		{"generator version", func(m map[string]any) { m["pins"].(map[string]any)["generator_version"] = "" }, DetailFieldInvalid},
		{"project name", func(m map[string]any) { m["pins"].(map[string]any)["project_name"] = "" }, DetailFieldInvalid},
		{"codex cli", func(m map[string]any) { m["pins"].(map[string]any)["codex_cli_version"] = "" }, DetailFieldInvalid},
		{"opencode cli", func(m map[string]any) { m["pins"].(map[string]any)["opencode_cli_version"] = "" }, DetailFieldInvalid},
		{"catalog path", func(m map[string]any) { m["pins"].(map[string]any)["codex_model_catalog"] = "../c.json" }, DetailUncleanPath},
		{"schema", func(m map[string]any) { m["schema_version"] = "x" }, DetailFieldInvalid},
		{"set version", func(m map[string]any) { m["set_version"] = "" }, DetailFieldInvalid},
		{"no active paths", func(m map[string]any) { m["active_paths"] = []any{} }, DetailFieldInvalid},
	}
	for _, tc := range cases {
		manifest := validManifest()
		tc.mutate(manifest)
		_, err := DecodeManifest([]byte(mustJSON(t, manifest)))
		requireInvalid(t, err, tc.detail)
	}
	_, err := DecodeManifest([]byte(mustJSON(t, validManifest())))
	require.NoError(t, err)
}
