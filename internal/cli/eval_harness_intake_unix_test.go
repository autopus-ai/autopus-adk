//go:build !windows

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// TestEvalHarnessIntake_S5_IntakeRejectAndPruneNeverRunRepro puts a sentinel
// command on PATH, names it in two repro values (one plain, one with shell
// expansions), and runs intake, reject, and prune. It swaps PATH and the
// working directory, so it stays serial.
func TestEvalHarnessIntake_S5_IntakeRejectAndPruneNeverRunRepro(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(bin, "sentinel.log")
	script := "#!/bin/sh\necho ran >> '" + logPath + "'\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "harneval-repro-sentinel"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := intakeProject(t,
		learn.LearningEntry{ID: "L-030", Type: learn.EntryTypeGateFail, Pattern: "sentinel incident", Expected: "e", Actual: "a", Repro: "harneval-repro-sentinel"},
		learn.LearningEntry{ID: "L-031", Type: learn.EntryTypeGateFail, Pattern: "expanded incident", Expected: "e", Actual: "a",
			Repro: "$(harneval-repro-sentinel) `harneval-repro-sentinel`"},
	)

	intake := runHarness(t, evalHarnessDeps{}, "intake", "--all-eligible", "--dir", root)
	require.Equal(t, 0, intake.code, intake.stderr)
	rows := harnessDoc(t, intake.stdout)["rows"].([]any)
	require.Len(t, rows, 2)
	kept, rejected := rows[0].(map[string]any)["candidate_id"].(string), rows[1].(map[string]any)["candidate_id"].(string)
	reject := runHarness(t, evalHarnessDeps{}, "reject", rejected, "--reason", "duplicate of the sentinel incident", "--dir", root)
	require.Equal(t, 0, reject.code, reject.stderr)
	chdirForTest(t, root)
	prune := NewRootCmd()
	var out bytes.Buffer
	prune.SetOut(&out)
	prune.SetErr(&out)
	prune.SetArgs([]string{"learn", "prune", "--days", "30"})
	require.NoError(t, prune.Execute())

	assert.Equal(t, "Removed 0 entries older than 30 days.\n", out.String())
	assert.Equal(t, "harneval-repro-sentinel", readCandidate(t, root, kept)["repro"], "repro stays data")
	assert.NoFileExists(t, logPath, "no repro value ran")
}

// TestEvalHarnessIntake_ControlCharacterFileName_IsEscapedOnStderr: a refusal
// names the offending file, and a hand-made name cannot drive the terminal.
func TestEvalHarnessIntake_ControlCharacterFileName_IsEscapedOnStderr(t *testing.T) {
	t.Parallel()
	root := intakeProject(t, s3LearnEntries()...)
	surface := filepath.Join(root, "evals", "harness", "tasks", "surface")
	require.NoError(t, os.MkdirAll(surface, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "evals", "harness", "manifest.json"),
		[]byte(`{"active_paths":["evals/harness/tasks/surface"]}`), 0o644))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(surface, "GT-\x1b[2J.json")))

	got := runHarness(t, evalHarnessDeps{}, "intake", "--all-eligible", "--dir", root)

	assert.Equal(t, 1, got.code)
	assert.Contains(t, got.stderr, `evals/harness/tasks/surface/GT-\u001b[2J.json`)
	assert.NotContains(t, got.stderr, "\x1b")
}

func TestEvalHarnessIntakeAndReject_S10SymlinkedCandidates_PathUnsafe(t *testing.T) {
	t.Parallel()
	for _, command := range [][]string{
		{"intake", "--all-eligible"},
		{"reject", "GTC-8e80c7a18029", "--reason", "not a harness issue"},
	} {
		root, outside := intakeProject(t, s3LearnEntries()...), t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(outside, "GTC-8e80c7a18029.json"), []byte("outside\n"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "evals", "harness"), 0o755))
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "evals", "harness", "candidates")))
		before := projectDigest(t, outside)

		got := runHarness(t, evalHarnessDeps{}, append(command, "--dir", root)...)

		assert.Equal(t, 1, got.code, command[0])
		assert.Empty(t, got.stdout)
		assert.Contains(t, got.stderr, "harness-eval: path_unsafe")
		assert.Equal(t, before, projectDigest(t, outside))
	}
}
