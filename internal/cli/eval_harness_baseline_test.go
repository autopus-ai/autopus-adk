package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// harnessStandardBaseline is the exact --init output for the standard set on
// a surface where every surface task passes: digests from the independent
// oracle, rows in id order, no timestamp.
const harnessStandardBaseline = `{
  "schema_version": "harness_eval_baseline.v1",
  "set_version": "1",
  "set_digest": "` + harnessStandardSet + `",
  "rows": [
    {
      "id": "GT-AG-001",
      "kind": "agent",
      "state": "active",
      "result": "not_run",
      "expectation_digest": "` + harnessAgentDigest + `"
    },
    {
      "id": "GT-FIX-A",
      "kind": "surface",
      "state": "active",
      "result": "pass",
      "expectation_digest": "` + harnessRouterDigest + `"
    },
    {
      "id": "GT-FIX-B",
      "kind": "surface",
      "state": "active",
      "result": "pass",
      "expectation_digest": "` + harnessRouterDigest + `"
    },
    {
      "id": "GT-FIX-C",
      "kind": "surface",
      "state": "active",
      "result": "pass",
      "expectation_digest": "` + harnessRouterDigest + `"
    }
  ]
}
`

// TestEvalHarnessBaseline_Init_WritesTheFirstBaselineOnce: --init pins the
// set when no baseline exists and refuses afterwards, leaving it untouched.
func TestEvalHarnessBaseline_Init_WritesTheFirstBaselineOnce(t *testing.T) {
	tree := standardHarnessTree(t)
	deps := harnessDeps(harnessRouter)

	first := runHarness(t, deps, "baseline", "--init", "--dir", tree.root)

	require.Equal(t, 0, first.code, first.stderr)
	body, sum := tree.baseline()
	assert.Equal(t, harnessStandardBaseline, body)
	assert.Contains(t, first.stdout, "wrote evals/harness/baseline.json with 4 rows")
	again := runHarness(t, deps, "baseline", "--init", "--dir", tree.root)
	assert.Equal(t, 1, again.code)
	assert.Contains(t, again.stderr, "baseline_exists")
	_, after := tree.baseline()
	assert.Equal(t, sum, after)
}

// TestEvalHarnessBaseline_S7_RegressionNeedsAcceptanceWithAReason is S7's
// first fixture: GT-FIX-B passed at baseline and fails now. Without the flag
// and without a reason the update exits 1 and leaves the baseline
// byte-identical; with both it records the accepted failure.
func TestEvalHarnessBaseline_S7_RegressionNeedsAcceptanceWithAReason(t *testing.T) {
	tree := newHarnessTree(t, map[string]string{"GT-FIX-A": harnessRouter, "GT-FIX-B": harnessGone, "GT-FIX-C": harnessRouter})
	require.Equal(t, 0, runHarness(t, harnessDeps(harnessRouter, harnessGone), "baseline", "--init", "--dir", tree.root).code)
	_, pinned := tree.baseline()
	regressed := harnessDeps(harnessRouter)

	unflagged := runHarness(t, regressed, "baseline", "--update", "--dir", tree.root)
	reasonless := runHarness(t, regressed, "baseline", "--update", "--accept-regression", "GT-FIX-B", "--dir", tree.root)

	assert.Equal(t, 1, unflagged.code)
	assert.Contains(t, unflagged.stderr, "regression_not_accepted: GT-FIX-B")
	assert.Equal(t, 1, reasonless.code)
	assert.Contains(t, reasonless.stderr, "accept_reason_required: GT-FIX-B")
	_, unchanged := tree.baseline()
	assert.Equal(t, pinned, unchanged, "a refused update leaves the baseline byte-identical")

	accepted := runHarness(t, regressed, "baseline", "--update", "--accept-regression", "GT-FIX-B",
		"--accept-regression", "GT-FIX-Z", "--reason", "hook intentionally removed", "--dir", tree.root)

	require.Equal(t, 0, accepted.code, accepted.stderr)
	assert.Contains(t, accepted.stderr, "GT-FIX-Z names no regression")
	ids, rows := tree.baselineRows()
	assert.Equal(t, []string{"GT-AG-001", "GT-FIX-A", "GT-FIX-B", "GT-FIX-C"}, ids)
	assert.Equal(t, map[string]any{
		"id": "GT-FIX-B", "kind": "surface", "state": "active", "result": "fail",
		"expectation_digest": harnessGoneDigest, "accepted_regression_reason": "hook intentionally removed",
	}, rows["GT-FIX-B"])
	assert.Equal(t, 0, runHarness(t, regressed, "run", "--dir", tree.root).code, "the accepted failure is the new baseline")

	// The acceptance stays while the task keeps failing and ends when it passes.
	require.Equal(t, 0, runHarness(t, regressed, "baseline", "--update", "--dir", tree.root).code)
	_, rows = tree.baselineRows()
	assert.Equal(t, "hook intentionally removed", rows["GT-FIX-B"]["accepted_regression_reason"])
	require.Equal(t, 0, runHarness(t, harnessDeps(harnessRouter, harnessGone), "baseline", "--update", "--dir", tree.root).code)
	_, rows = tree.baselineRows()
	assert.Equal(t, "pass", rows["GT-FIX-B"]["result"])
	assert.NotContains(t, rows["GT-FIX-B"], "accepted_regression_reason")
}

// TestEvalHarnessBaseline_S7_DeletionNeedsATombstoneThatStaysRetired is S7's
// second fixture: deleting GT-FIX-F without a tombstone is refused; with a
// retired tombstone the row becomes retired and survives the tombstone's own
// deletion, so no later run reports task_missing.
func TestEvalHarnessBaseline_S7_DeletionNeedsATombstoneThatStaysRetired(t *testing.T) {
	tree := newHarnessTree(t, map[string]string{"GT-FIX-A": harnessRouter, "GT-FIX-F": harnessRouter})
	deps := harnessDeps(harnessRouter)
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--init", "--dir", tree.root).code)
	_, pinned := tree.baseline()
	tree.remove(harnessSurfacePath("GT-FIX-F"))

	missing := runHarness(t, deps, "baseline", "--update", "--dir", tree.root)

	assert.Equal(t, 1, missing.code)
	assert.Contains(t, missing.stderr, "tombstone_required: GT-FIX-F")
	_, unchanged := tree.baseline()
	assert.Equal(t, pinned, unchanged)

	tombstone := harnessSurfaceTask("GT-FIX-F", harnessRouter)
	tombstone["status"] = map[string]any{"state": harneval.StateRetired, "reason": "folded into GT-FIX-A"}
	tree.writeJSON(harnessSurfacePath("GT-FIX-F"), tombstone)
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--update", "--dir", tree.root).code)
	retiredRow := map[string]any{
		"id": "GT-FIX-F", "kind": "surface", "state": "retired", "result": "pass",
		"expectation_digest": harnessRouterDigest, "retired_reason": "folded into GT-FIX-A",
	}
	_, rows := tree.baselineRows()
	assert.Equal(t, retiredRow, rows["GT-FIX-F"])

	tree.remove(harnessSurfacePath("GT-FIX-F"))
	after := harnessDoc(t, runHarness(t, deps, "run", "--dir", tree.root).stdout)
	assert.Equal(t, []any{}, after["transitions"], "a retired row is not task_missing")
	assert.Equal(t, []any{"set_digest_mismatch"}, after["failure_reasons"])
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--update", "--dir", tree.root).code)
	ids, rows := tree.baselineRows()
	assert.Equal(t, []string{"GT-AG-001", "GT-FIX-A", "GT-FIX-F"}, ids)
	assert.Equal(t, retiredRow, rows["GT-FIX-F"], "the retired row is permanent")
	assert.Equal(t, 0, runHarness(t, deps, "run", "--dir", tree.root).code)
}

// TestEvalHarnessBaseline_Preconditions_RefuseWithoutWriting: an update with
// no baseline, a vacuous or invalid set, a never-run retired task, and bad
// flag combinations exit 1 and write nothing.
func TestEvalHarnessBaseline_Preconditions_RefuseWithoutWriting(t *testing.T) {
	tree := standardHarnessTree(t)
	deps := harnessDeps(harnessRouter)
	baselineFile := filepath.Join(tree.root, filepath.FromSlash(harneval.BaselinePath))

	noBaseline := runHarness(t, deps, "baseline", "--update", "--dir", tree.root)
	assert.Equal(t, 1, noBaseline.code)
	assert.Contains(t, noBaseline.stderr, "baseline not written: baseline_missing")
	assert.Contains(t, noBaseline.stderr, "baseline --init")

	manifest := harnessManifest()
	manifest["floors"] = map[string]any{"surface_tasks": 4, "agent_tasks": 1}
	tree.writeJSON(harneval.ManifestPath, manifest)
	vacuous := runHarness(t, deps, "baseline", "--init", "--dir", tree.root)
	assert.Equal(t, 1, vacuous.code)
	assert.Contains(t, vacuous.stderr, "baseline not written: vacuous (active_surface_tasks=3 floor=4)")
	assert.NoFileExists(t, baselineFile)

	tree.writeJSON(harneval.ManifestPath, harnessManifest())
	tree.write(harnessSurfacePath("GT-FIX-A"), "{}{}")
	invalid := runHarness(t, deps, "baseline", "--init", "--dir", tree.root)
	assert.Equal(t, 1, invalid.code)
	assert.Contains(t, invalid.stderr, "baseline not written: invalid (trailing_data)")
	assert.NoFileExists(t, baselineFile)

	for _, args := range [][]string{
		{"baseline"},
		{"baseline", "--init", "--update"},
		{"baseline", "--init", "--accept-regression", "GT-FIX-A"},
		{"baseline", "--init", "--reason", "why"},
	} {
		got := runHarness(t, deps, append(args, "--dir", tree.root)...)
		assert.Equal(t, 1, got.code, args)
		assert.Contains(t, got.stderr, "Error: ", args)
	}
	assert.NoFileExists(t, baselineFile)

	// A corrupt baseline is not "missing": --init refuses instead of replacing it.
	tree.write(harneval.BaselinePath, "{}{}")
	corrupt := runHarness(t, deps, "baseline", "--init", "--dir", tree.root)
	assert.Equal(t, 1, corrupt.code)
	assert.Contains(t, corrupt.stderr, "baseline not written: invalid: trailing_data")
	body, _ := tree.baseline()
	assert.Equal(t, "{}{}", body)
}

// TestEvalHarnessBaseline_NeverRunTombstone_IsPinnedAsFail: a retired task
// the baseline never saw has no outcome, so it is recorded as fail (a pass is
// only ever proven); an unwritable baseline file is an error.
func TestEvalHarnessBaseline_NeverRunTombstone_IsPinnedAsFail(t *testing.T) {
	tree := standardHarnessTree(t)
	tombstone := harnessSurfaceTask("GT-FIX-T", harnessRouter)
	tombstone["status"] = map[string]any{"state": harneval.StateRetired, "reason": "kept for history"}
	tree.writeJSON(harnessSurfacePath("GT-FIX-T"), tombstone)
	deps := harnessDeps(harnessRouter)

	require.Equal(t, 0, runHarness(t, deps, "baseline", "--init", "--dir", tree.root).code)

	_, rows := tree.baselineRows()
	assert.Equal(t, "fail", rows["GT-FIX-T"]["result"])
	assert.Equal(t, "retired", rows["GT-FIX-T"]["state"])
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	baselineFile := filepath.Join(tree.root, filepath.FromSlash(harneval.BaselinePath))
	require.NoError(t, os.Chmod(baselineFile, 0o444))
	locked := runHarness(t, deps, "baseline", "--update", "--dir", tree.root)
	assert.Equal(t, 1, locked.code)
	assert.Contains(t, locked.stderr, "write baseline")
}
