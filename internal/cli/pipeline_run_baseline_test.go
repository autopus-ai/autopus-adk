package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

func contentDigest(body string) string {
	sum := sha256.Sum256([]byte(body))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Probe B1 in the repository: the fingerprint survives committing the
// verified tree and changes with any later content change.
func TestPipelineBaseline_FingerprintIgnoresCommitsAndCatchesEdits(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t)
	f.write("a.go", "a\n")
	f.write("b.go", "b\n")
	base := f.commit("base")
	f.write("a.go", "a2\n")
	f.write("c.go", "new\n")
	require.NoError(t, os.Remove(filepath.Join(f.root, "b.go")))
	fingerprint := func() string {
		t.Helper()
		digests, err := workingTreeDigests(f.root, base)
		require.NoError(t, err)
		return treeFingerprint(digests)
	}

	digests, err := workingTreeDigests(f.root, base)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a.go": contentDigest("a2\n"), "b.go": "deleted", "c.go": contentDigest("new\n")}, digests)
	verified := treeFingerprint(digests)

	f.commit("commit the verified tree")
	assert.Equal(t, verified, fingerprint(), "committing the verified tree changes nothing")
	f.write("a.go", "a3\n")
	assert.NotEqual(t, verified, fingerprint())
	f.write("a.go", "a2\n")
	assert.Equal(t, verified, fingerprint())
	f.write("d.go", "untracked\n")
	assert.NotEqual(t, verified, fingerprint())
	require.NoError(t, os.Remove(filepath.Join(f.root, "d.go")))
	f.write(".autopus/pipeline-state/SPEC-X.yaml", "harness state\n")
	assert.Equal(t, verified, fingerprint(), "harness state under .autopus is not code")

	runGitCommand(t, f.root, "mv", "a.go", "e.go")
	digests, err = workingTreeDigests(f.root, base)
	require.NoError(t, err)
	assert.Equal(t, "deleted", digests["a.go"], "a rename lists the side it removed")
	assert.Equal(t, contentDigest("a2\n"), digests["e.go"])
}

func TestPipelineBaseline_StatusLinesDoNotChangeTheSpecFingerprint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	fingerprint := func() string {
		value, err := specRequirementsFingerprint(dir)
		require.NoError(t, err)
		return value
	}
	write("spec.md", "# SPEC-X: x\n\n**Status**: draft\n\n## Requirements\n\nIt works.\n")
	write("plan.md", "# Plan\n\nStatus: draft\n\n## Tasks\n\n- T1\n")
	original := fingerprint()

	write("spec.md", "# SPEC-X: x\n\n**Status**: completed\n\n## Requirements\n\nIt works.\n")
	write("plan.md", "# Plan\n\nStatus: done\n\n## Tasks\n\n- T1\n")
	assert.Equal(t, original, fingerprint())
	write("spec.md", "---\nid: SPEC-X\nstatus: approved\n---\n# SPEC-X: x\n\n## Requirements\n\nIt works.\n")
	frontmatter := fingerprint()
	write("spec.md", "---\nid: SPEC-X\nstatus: completed\n---\n# SPEC-X: x\n\n## Requirements\n\nIt works.\n")
	assert.Equal(t, frontmatter, fingerprint(), "a frontmatter status line is a status line too")

	write("spec.md", "# SPEC-X: x\n\n**Status**: draft\n\n## Requirements\n\nIt works twice.\n")
	assert.NotEqual(t, original, fingerprint())
	write("spec.md", "# SPEC-X: x\n\n**Status**: draft\n\n## Requirements\n\nStatus: a body line is content.\n")
	assert.NotEqual(t, original, fingerprint())
}

// Only a completed run records a baseline; outside git the run says why it
// recorded none and still completes.
func TestPipelineBaseline_RecordedOnlyForCompletedRuns(t *testing.T) {
	const specID = "SPEC-BASE-001"
	f := newStatusFixture(t)
	f.writeSpec(specID, "approved")
	head := f.commit("base")
	chdirForTest(t, f.root)
	specDir := filepath.Join(".autopus", "specs", specID)
	path := pipelineVerificationPath(pipelineStateDir, specID)

	for _, terminal := range []pipeline.TerminalState{
		pipeline.TerminalBlocked, pipeline.TerminalFailedBudget, pipeline.TerminalCancelled,
		pipeline.TerminalPartialPreserved, pipeline.TerminalDryRun,
	} {
		var out bytes.Buffer
		recordPipelineVerification(&out, specID, specDir,
			&pipeline.PipelineResult{Receipt: pipeline.OrchestrationRunReceipt{RunID: "run-x", Terminal: terminal}})
		assert.Empty(t, out.String(), terminal)
		assert.NoFileExists(t, path, terminal)
	}

	var out bytes.Buffer
	recordPipelineVerification(&out, specID, specDir, &pipeline.PipelineResult{
		Receipt: pipeline.OrchestrationRunReceipt{RunID: "run-1", TaskID: "task-1", Terminal: pipeline.TerminalCompleted},
	})
	assert.Equal(t, "Pipeline verification baseline: .autopus/pipeline-state/SPEC-BASE-001.verification.json (HEAD "+
		head[:12]+")\n", out.String())
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	var baseline pipelineVerificationBaseline
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &baseline))
	assert.Equal(t, pipelineVerificationSchema, baseline.Schema)
	assert.Equal(t, "run-1", baseline.RunID)
	assert.Equal(t, "task-1", baseline.TaskID)
	assert.Equal(t, head, baseline.BaseHead)
	assert.Empty(t, baseline.TreePaths, "a clean tree differs from HEAD nowhere")

	outside := t.TempDir()
	chdirForTest(t, outside)
	out.Reset()
	recordPipelineVerification(&out, specID, specDir, &pipeline.PipelineResult{
		Receipt: pipeline.OrchestrationRunReceipt{RunID: "run-2", Terminal: pipeline.TerminalCompleted},
	})
	assert.Contains(t, out.String(), "Pipeline verification baseline not recorded: read git HEAD:")
	assert.NoFileExists(t, filepath.Join(outside, path))
}

// A baseline that cannot be written leaves the previous one exactly as it
// was and fails nothing: the run completed, and the reason is reported.
func TestPipelineBaseline_WriteFailureIsReportedAndAtomic(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX directory permissions that bind the user")
	}
	const specID = "SPEC-BASE-002"
	f := newStatusFixture(t)
	f.writeSpec(specID, "approved")
	f.commit("base")
	chdirForTest(t, f.root)
	specDir := filepath.Join(".autopus", "specs", specID)
	completed := &pipeline.PipelineResult{Receipt: pipeline.OrchestrationRunReceipt{RunID: "run-1", Terminal: pipeline.TerminalCompleted}}
	var out bytes.Buffer
	recordPipelineVerification(&out, specID, specDir, completed)
	path := pipelineVerificationPath(pipelineStateDir, specID)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(pipelineStateDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(pipelineStateDir, 0o755) })
	out.Reset()
	completed.Receipt.RunID = "run-2"
	recordPipelineVerification(&out, specID, specDir, completed)

	assert.Contains(t, out.String(), "Pipeline verification baseline not recorded: ")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the previous baseline is untouched")
}

// A tree whose changes git would hide is never recorded as verified.
func TestPipelineBaseline_RefusesATreeGitWouldHide(t *testing.T) {
	t.Parallel()
	f := newStatusFixture(t)
	f.write("tracked.go", "package x\n")
	head := f.commit("base")
	runGitCommand(t, f.root, "update-index", "--skip-worktree", "tracked.go")
	_, err := workingTreeDigests(f.root, head)
	require.ErrorIs(t, err, errTreeHidesChanges)
	assert.Contains(t, err.Error(), "1 marked assume-unchanged or skip-worktree: tracked.go")
}
