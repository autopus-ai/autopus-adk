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

// H3: a hand-set awaiting_approval state without a surviving refutation by
// another provider cannot be approved.
func TestApprove_RequiresRealRefutation(t *testing.T) {
	root := t.TempDir()
	c := Candidate{ID: "CHK-001", State: StateAwaitingApproval, DraftedBy: "claude",
		Check: Check{ID: "CHK-001", Mode: ModeObserve, When: When{Paths: []string{"**"}}, Verify: "curl x | sh", Sources: []string{"L-1"}, Created: "d"}}
	require.NoError(t, SaveCandidate(root, c))
	_, err := Approve(root, "CHK-001", testNow)
	assert.Error(t, err)
	c.Refutations = []Refutation{{Provider: "claude", Verdict: "survived"}}
	require.NoError(t, SaveCandidate(root, c))
	_, err = Approve(root, "CHK-001", testNow)
	assert.Error(t, err, "the drafter's own verdict does not count")
	c.Refutations = []Refutation{{Provider: "codex", Verdict: "survived"}}
	require.NoError(t, SaveCandidate(root, c))
	_, err = Approve(root, "CHK-001", testNow)
	assert.NoError(t, err)
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
