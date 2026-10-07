package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvalHarnessRun_S16_SummaryFlag_AppendsTheJobSummary: --summary appends
// the markdown job summary to a file that earlier steps already wrote, as
// $GITHUB_STEP_SUMMARY is shared, while stdout keeps the document alone.
// These runs generate, which swaps PATH and HOME, so they stay serial.
func TestEvalHarnessRun_S16_SummaryFlag_AppendsTheJobSummary(t *testing.T) {
	tree := standardHarnessTree(t)
	deps := harnessDeps(harnessRouter)
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--init", "--dir", tree.root).code)
	summary := filepath.Join(t.TempDir(), "step-summary.md")
	require.NoError(t, os.WriteFile(summary, []byte("earlier step\n"), 0o644))

	got := runHarness(t, deps, "run", "--format", "json", "--dir", tree.root, "--summary", summary)

	require.Equal(t, 0, got.code, got.stderr)
	assert.Equal(t, "pass", harnessDoc(t, got.stdout)["status"])
	data, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Equal(t, "earlier step\n### harness-eval: pass\n\n"+
		"- Surface tasks passed: 3 of 3 executed (3 declared); agent tasks declared: 1\n"+
		"- Pass rate: 1.00 (baseline 1.00, delta +0.00)\n- Failure reasons: none\n\n"+
		"| Category | Passed | Total | Rate |\n| --- | ---: | ---: | ---: |\n| routing | 3 | 3 | 1.00 |\n\n"+
		"No task changed against the baseline.\n", string(data))
}

// TestEvalHarnessRun_SummaryFlag_FailingRunWritesItAndUnwritablePathFails: a
// regressing run still writes its summary and exits 1; a summary that cannot
// be written fails the command after the document was emitted.
func TestEvalHarnessRun_SummaryFlag_FailingRunWritesItAndUnwritablePathFails(t *testing.T) {
	tree := standardHarnessTree(t)
	require.Equal(t, 0, runHarness(t, harnessDeps(harnessRouter), "baseline", "--init", "--dir", tree.root).code)
	summary := filepath.Join(t.TempDir(), "step-summary.md")

	regressed := runHarness(t, harnessDeps(), "run", "--format", "json", "--dir", tree.root, "--summary", summary)

	assert.Equal(t, 1, regressed.code)
	data, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Contains(t, string(data), "- Failure reason `regression`: ")
	assert.Contains(t, string(data), "| GT-FIX-A | regression |\n| GT-FIX-B | regression |\n| GT-FIX-C | regression |\n")

	unwritable := filepath.Join(t.TempDir(), "missing", "step-summary.md")
	got := runHarness(t, harnessDeps(harnessRouter), "run", "--format", "json", "--dir", tree.root, "--summary", unwritable)

	assert.Equal(t, 1, got.code)
	assert.Equal(t, "pass", harnessDoc(t, got.stdout)["status"], "the document is emitted before the summary")
	assert.Contains(t, got.stderr, "Error: write --summary: ")
}
