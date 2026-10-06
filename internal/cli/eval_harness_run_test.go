package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// TestEvalHarnessRun_S1_LoadPreconditions_ExitOneWithTheDocumentAlone is S1
// at the CLI: each load defect is exactly ["invalid"] with its detail and a
// missing baseline is ["baseline_missing"], always with exit 1, the document
// alone on stdout, and the human lines on stderr. Nothing here generates, so
// the cases run in parallel.
func TestEvalHarnessRun_S1_LoadPreconditions_ExitOneWithTheDocumentAlone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(h *harnessTree)
		reasons []any
		details []any
		hint    string
	}{
		{"unknown field", func(h *harnessTree) {
			task := harnessSurfaceTask("GT-FIX-A", harnessRouter)
			task["extra"] = 1
			h.writeJSON(harnessSurfacePath("GT-FIX-A"), task)
		}, []any{"invalid"}, []any{"unknown_field"}, "GT-FIX-A.json"},
		{"threshold_bp 5", func(h *harnessTree) {
			manifest := harnessManifest()
			manifest["live"].(map[string]any)["threshold_bp"] = 5
			h.writeJSON(harneval.ManifestPath, manifest)
		}, []any{"invalid"}, []any{"policy_out_of_range"}, "manifest.json"},
		{"symlinked task file", func(h *harnessTree) {
			h.write("elsewhere/GT-FIX-D.json", "{}\n")
			require.NoError(h.t, os.Symlink(filepath.Join(h.root, "elsewhere", "GT-FIX-D.json"),
				filepath.Join(h.root, filepath.FromSlash(harnessSurfacePath("GT-FIX-D")))))
		}, []any{"invalid"}, []any{"symlink_not_allowed"}, "GT-FIX-D.json"},
		{"agent task without expected_tests", func(h *harnessTree) {
			task := harnessAgentTask()
			delete(task, "expected_tests")
			h.writeJSON(harnessAgentPath, task)
		}, []any{"invalid"}, []any{"expected_tests_missing"}, "GT-AG-001.json"},
		{"baseline missing", func(*harnessTree) {}, []any{"baseline_missing"}, []any{}, "baseline --init"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tree := standardHarnessTree(t)
			tc.mutate(tree)

			got := runHarness(t, evalHarnessDeps{}, "run", "--format", "json", "--dir", tree.root)

			assert.Equal(t, 1, got.code)
			doc := harnessDoc(t, got.stdout)
			assert.Equal(t, "fail", doc["status"])
			assert.Equal(t, tc.reasons, doc["failure_reasons"])
			assert.Equal(t, tc.details, doc["details"])
			assert.Equal(t, []any{}, doc["transitions"])
			assert.Contains(t, got.stderr, tc.hint)
			assert.NotContains(t, got.stderr, "Error:", "the document already explains the failure")
		})
	}
}

// TestEvalHarnessRun_S1_NormalFixture_PassesWithExitZero: a valid set pinned
// by --init runs clean with totals.declared_surface 3 and the oracle digest.
func TestEvalHarnessRun_S1_NormalFixture_PassesWithExitZero(t *testing.T) {
	tree := standardHarnessTree(t)
	deps := harnessDeps(harnessRouter)
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--init", "--dir", tree.root).code)

	got := runHarness(t, deps, "run", "--format", "json", "--dir", tree.root)

	require.Equal(t, 0, got.code, got.stderr)
	doc := harnessDoc(t, got.stdout)
	assert.Equal(t, "pass", doc["status"])
	assert.Equal(t, map[string]any{"declared_surface": 3.0, "executed_surface": 3.0, "passed_surface": 3.0, "declared_agent": 1.0}, doc["totals"])
	assert.Equal(t, harnessStandardSet, doc["set_digest"])
	assert.Equal(t, "2026-10-07T01:02:03Z", doc["produced_at"])
	assert.Empty(t, got.stderr, "a clean pass has no guidance")
}

// TestEvalHarnessRun_S3_BaselineComparison_ExitsOneWithTransitions is the S3
// main fixture through the CLI. The baseline (A, B, E, F pass, C fail) is
// pinned by --init; then B's file disappears, C's appears, D is added, E is
// retired, and F is deleted without a tombstone.
func TestEvalHarnessRun_S3_BaselineComparison_ExitsOneWithTransitions(t *testing.T) {
	tree := newHarnessTree(t, map[string]string{
		"GT-FIX-A": harnessRouter, "GT-FIX-B": harnessGone, "GT-FIX-C": harnessHook,
		"GT-FIX-E": harnessRouter, "GT-FIX-F": harnessRouter,
	})
	require.Equal(t, 0, runHarness(t, harnessDeps(harnessRouter, harnessGone), "baseline", "--init", "--dir", tree.root).code)
	tree.writeJSON(harnessSurfacePath("GT-FIX-D"), harnessSurfaceTask("GT-FIX-D", harnessRouter))
	retired := harnessSurfaceTask("GT-FIX-E", harnessRouter)
	retired["status"] = map[string]any{"state": harneval.StateRetired, "reason": "superseded"}
	tree.writeJSON(harnessSurfacePath("GT-FIX-E"), retired)
	tree.remove(harnessSurfacePath("GT-FIX-F"))

	got := runHarness(t, harnessDeps(harnessRouter, harnessHook), "run", "--format", "json", "--dir", tree.root)

	assert.Equal(t, 1, got.code)
	doc := harnessDoc(t, got.stdout)
	assert.InDelta(t, 0.75, doc["pass_rate"], 1e-9)
	assert.InDelta(t, 0.8, doc["baseline_pass_rate"], 1e-9)
	assert.InDelta(t, -0.05, doc["regression_delta"], 1e-9)
	assert.Equal(t, []any{
		map[string]any{"task_id": "GT-FIX-B", "kind": "regression"},
		map[string]any{"task_id": "GT-FIX-C", "kind": "improved"},
		map[string]any{"task_id": "GT-FIX-D", "kind": "new"},
		map[string]any{"task_id": "GT-FIX-E", "kind": "retired"},
		map[string]any{"task_id": "GT-FIX-F", "kind": "task_missing"},
	}, doc["transitions"])
	assert.Equal(t, []any{"regression", "set_digest_mismatch", "task_missing"}, doc["failure_reasons"])
	assert.Contains(t, got.stderr, "--accept-regression <task-id> --reason <text>")
}

// TestEvalHarnessRun_S3_ImprovementOnly_PassesWithTheHintOnStderrOnly is the
// S3 second fixture: baseline A pass, C fail, an unchanged set, and now C
// passes. It exits 0 with delta 0.5, stdout is one JSON document equal to
// the --output file, and the baseline --update hint is on stderr only.
func TestEvalHarnessRun_S3_ImprovementOnly_PassesWithTheHintOnStderrOnly(t *testing.T) {
	tree := newHarnessTree(t, map[string]string{"GT-FIX-A": harnessRouter, "GT-FIX-C": harnessGone})
	pinned := runHarness(t, harnessDeps(harnessRouter), "baseline", "--init", "--dir", tree.root)
	require.Equal(t, 0, pinned.code, pinned.stderr)
	assert.Contains(t, pinned.stderr, "recorded as fail: GT-FIX-C", "a failing golden task is never pinned silently")
	output := filepath.Join(t.TempDir(), "result.json")

	got := runHarness(t, harnessDeps(harnessRouter, harnessGone), "run", "--format", "json", "--output", output, "--dir", tree.root)

	require.Equal(t, 0, got.code, got.stderr)
	doc := harnessDoc(t, got.stdout)
	assert.Equal(t, "pass", doc["status"])
	assert.Equal(t, []any{}, doc["failure_reasons"])
	assert.InDelta(t, 0.5, doc["regression_delta"], 1e-9)
	assert.Equal(t, []any{map[string]any{"task_id": "GT-FIX-C", "kind": "improved"}}, doc["transitions"])
	assert.Contains(t, got.stderr, "auto eval harness baseline --update")
	assert.NotContains(t, got.stdout, "baseline --update")
	written, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.Equal(t, got.stdout, string(written))
}

// TestEvalHarnessRun_UnusableInvocation_ExitsOneWithoutADocument: a format
// other than json, an unwritable --output, and a pipeline that cannot produce
// a document all exit 1 with an error and nothing on stdout.
func TestEvalHarnessRun_UnusableInvocation_ExitsOneWithoutADocument(t *testing.T) {
	tree := standardHarnessTree(t)
	deps := harnessDeps(harnessRouter)
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--init", "--dir", tree.root).code)
	broken := harnessDeps(harnessRouter)
	broken.run.Mutate = func(*harneval.Generation) error { return errors.New("surface refused") }
	cases := map[string]struct {
		deps evalHarnessDeps
		args []string
		want string
	}{
		"text format":       {deps, []string{"--format", "text"}, `unsupported --format "text"`},
		"unwritable output": {deps, []string{"--output", filepath.Join(t.TempDir(), "missing", "r.json")}, "write result"},
		"no document":       {broken, nil, "surface refused"},
	}
	for name, tc := range cases {
		got := runHarness(t, tc.deps, append([]string{"run", "--dir", tree.root}, tc.args...)...)
		assert.Equal(t, 1, got.code, name)
		assert.Empty(t, got.stdout, name)
		assert.Contains(t, got.stderr, "Error: ", name)
		assert.Contains(t, got.stderr, tc.want, name)
	}
}

// TestEvalHarnessDigest_PrintsTheDigestsARunCompares: the set, agent set, and
// task digests equal the independent oracle, the surface digest is the one a
// run reports for the same tree, and an unloadable set or a failed
// generation exits 1 without a document.
func TestEvalHarnessDigest_PrintsTheDigestsARunCompares(t *testing.T) {
	tree := standardHarnessTree(t)
	deps := harnessDeps(harnessRouter)

	got := runHarness(t, deps, "digest", "--format", "json", "--dir", tree.root)

	require.Equal(t, 0, got.code, got.stderr)
	doc := harnessDoc(t, got.stdout)
	assert.Equal(t, "1", doc["set_version"])
	assert.Equal(t, harnessStandardSet, doc["set_digest"])
	assert.Equal(t, harnessStandardAgents, doc["agent_set_digest"])
	row := func(id, kind, digest string) any {
		return map[string]any{"id": id, "kind": kind, "state": "active", "expectation_digest": digest}
	}
	assert.Equal(t, []any{
		row("GT-AG-001", "agent", harnessAgentDigest), row("GT-FIX-A", "surface", harnessRouterDigest),
		row("GT-FIX-B", "surface", harnessRouterDigest), row("GT-FIX-C", "surface", harnessRouterDigest),
	}, doc["tasks"])
	assert.Regexp(t, `^[0-9a-f]{64}$`, doc["surface_digest"])
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--init", "--dir", tree.root).code)
	assert.Equal(t, harnessDoc(t, runHarness(t, deps, "run", "--dir", tree.root).stdout)["surface_digest"], doc["surface_digest"])

	// A file where the adapter needs a directory fails generation.
	cases := map[string]struct {
		deps evalHarnessDeps
		args []string
		want string
	}{
		"failed generation": {harnessDeps("a", "a/b"), nil, "generate surface"},
		"text format":       {deps, []string{"--format", "text"}, "unsupported --format"},
	}
	for name, tc := range cases {
		failed := runHarness(t, tc.deps, append([]string{"digest", "--dir", tree.root}, tc.args...)...)
		assert.Equal(t, 1, failed.code, name)
		assert.Empty(t, failed.stdout, name)
		assert.Contains(t, failed.stderr, tc.want, name)
	}
	tree.write(harnessSurfacePath("GT-FIX-A"), "{}{}")
	unloadable := runHarness(t, deps, "digest", "--dir", tree.root)
	assert.Equal(t, 1, unloadable.code)
	assert.Contains(t, unloadable.stderr, "load golden set: invalid: trailing_data")
}

// TestEvalHarness_RootRegistersTheNamespace: the public `auto eval harness`
// commands are reachable from the root command.
func TestEvalHarness_RootRegistersTheNamespace(t *testing.T) {
	t.Parallel()
	root := NewRootCmd()
	for _, name := range []string{"run", "baseline", "applicable", "digest"} {
		cmd, _, err := root.Find([]string{"eval", "harness", name})
		require.NoError(t, err, name)
		assert.Equal(t, name, cmd.Name())
		assert.True(t, strings.HasPrefix(cmd.CommandPath(), "auto eval harness"), cmd.CommandPath())
	}
}
