//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S4, S7, S13: a flag-on tier-3 diagnosis runs steps 1–2, the confined
// diagnosis in the band worktree, and a BS with the pointer and model
// lines; its local_patch claim then runs after phase C and produces one
// local patch.
func TestReactBandDiagnoseLocalPatch_TierThreeDiagnosisThenLocalPatch(t *testing.T) {
	w := newDLPWorld(t, nil, dlpPatchClaim())
	w.fake.answer(t, 1, lpStream(lpInit55, lpAssistant55, lpResult(dlpDiagnosisText)))
	w.fake.answer(t, 2, lpStream(lpInit55, lpAssistant55, lpResult(lpReplyWith(lpFooDiff))))
	t.Setenv("GH_TOKEN", "ghp_synthetic")
	t.Setenv("GIT_DIR", filepath.Join(w.repo, ".git"))

	outcome := w.claim(t, dlpClaim(lpDiagnoseClaimID, 3, lpT0.Add(990*time.Second)))

	require.Equal(t, bandDiagnosisOK, outcome.DiagnosisStatus)
	require.Equal(t, "BS-BAND-001", outcome.BSID)
	assert.Len(t, outcome.PromptManifest, 3, "001's diagnosis prompt: instructions, evaluation, one log")
	worktree := filepath.Join(w.lp(), lpKey, "worktree")
	assert.Equal(t, 2, w.fake.calls(t), "the diagnosis and the patch request")
	for n := 1; n <= 2; n++ {
		assert.Equal(t, worktree+"\n", w.fake.record(t, "cwd", n), "call %d runs in the band worktree", n)
		argv := w.fake.argv(t, n)
		assert.Equal(t, []string{"--print", "--model", "claude-opus-5-5", "--effort", "max"}, argv[:5])
		assert.Equal(t, "--tools=Read,Grep,Glob", argv[len(argv)-1])
		for _, flag := range []string{"--restricted", "--verbose", "--strict-mcp-config", "--safe-mode"} {
			assert.Contains(t, argv, flag, "call %d", n)
		}
		assert.Equal(t, []string{"stream-json"}, bandFlagValues(argv, "--output-format"))
		assert.Equal(t, []string{"plan"}, bandFlagValues(argv, "--permission-mode"))
		assert.NotContains(t, argv, "--add-dir")
		for _, line := range strings.Split(w.fake.record(t, "env", n), "\n") {
			name, _, _ := strings.Cut(line, "=")
			assert.False(t, strings.HasPrefix(name, "GIT_") || name == "GH_TOKEN", "call %d inherits %s", n, name)
		}
	}
	assert.Contains(t, w.fake.record(t, "stdin", 1), "step 3 failed", "the diagnosis prompt carries the evidence")

	bs := w.bs(t, outcome.BSID)
	assert.Empty(t, brainstorm.Validate([]byte(bs)))
	assert.Contains(t, bs, "\ndiagnosis_status: ok\n\n"+healthband.Fence(dlpDiagnosisText)+"\n")
	assert.Contains(t, bs, "\n\nLocal patch (3σ, local only, if produced): branch autopus/band/"+lpKey+", worktree "+worktree+
		"/, patch file "+filepath.Join(w.lp(), lpKey+".patch")+", outside this repository.\n\n")
	assert.Contains(t, bs, " under claim "+lpPatchClaimID+"; nothing was pushed.\n\n"+brainstorm.LocalPatchReviewerWarning+"\n\n"+
		"Diagnosis model: requested claude-opus-5-5, actual claude-opus-5-5.\n\n## Evolution Ideas\n")
	assert.Contains(t, bs, "- When: detected 2026-10-08; tier 3 with health_band.allow_local_patch may leave one local patch")

	require.Len(t, w.reports, 1)
	result := w.reports[0]
	assert.Equal(t, healthband.ClaimDone, result.Status)
	assert.Equal(t, lpPatchClaimID, result.ClaimID)
	assert.Equal(t, "BS-BAND-001", result.BSID, "the BS ID that 001 recorded before the patch stage")
	assert.Equal(t, worktree, result.WorktreePath)
	assert.Equal(t, []localPatchModel{
		{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"},
		{Request: lpRequestPatch, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"},
	}, result.Models)
	assert.Equal(t, []string{"prep", "stage:worktree_intent", "stage:worktree_done", "stage:message", "stage:apply_intent",
		"stage:apply_done", "stage:commit_done", "stage:branch_intent", "stage:branch_done", "stage:patch_intent",
		"stage:patch_done", "result"}, w.ledger.trail())
	assert.Equal(t, []healthband.LocalPatchRecord{result}, lpUnsealedAll(w.results()), "one result, the one reported")
	assert.Equal(t, result.CommitSHA+"\n", w.git(w.repo, "rev-parse", "refs/heads/autopus/band/"+lpKey))
	assert.Equal(t, []string{worktree}, w.worktrees(t), "the worktree is kept for review")
}

// S7: a tier-2 diagnosis with the flag on runs in a worktree under its own
// claim-unique key, which only Cleanup Rule 3 removes right after the BS,
// and ends with a result keyed by its diagnose claim id.
func TestReactBandDiagnoseLocalPatch_TierTwoDiagnosisRemovesItsWorktree(t *testing.T) {
	w := newDLPWorld(t, nil)
	w.fake.answer(t, 0, lpStream(lpInit55, lpAssistant55, lpResult(dlpDiagnosisText)))
	claim := dlpClaim(lpDiagnoseClaimID, 2, lpT0.Add(990*time.Second))

	outcome := w.claim(t, claim)

	require.Equal(t, bandDiagnosisOK, outcome.DiagnosisStatus)
	key := healthband.LocalPatchKey(lpSeries, "e1042", lpDiagnoseClaimID)
	require.True(t, strings.HasSuffix(key, "-0f1e2d3c"), key)
	assert.Equal(t, filepath.Join(w.lp(), key, "worktree")+"\n", w.fake.record(t, "cwd", 1))
	assert.Equal(t, 1, w.fake.calls(t))
	assert.Equal(t, []string{"prep", "stage:worktree_intent", "stage:worktree_done", "result"}, w.ledger.trail())
	result := w.ledger.result()
	assert.Equal(t, lpDiagnoseClaimID, result.ClaimID)
	assert.Equal(t, healthband.ClaimDone, result.Status)
	assert.Equal(t, []localPatchModel{{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}}, result.Models)
	assert.Empty(t, w.worktrees(t))
	assert.True(t, w.absent(key), "the empty <lp>/<key>/ goes with the worktree")
	assert.True(t, w.absent(key+".lock"))
	assert.Empty(t, w.reports, "local_patches[] lists local_patch claims only")

	bs := w.bs(t, outcome.BSID)
	assert.NotContains(t, bs, "Local patch (3σ")
	assert.Contains(t, bs, "Tier 2 is diagnosis-only: band opened no branch, commit, or pull request.\n\n"+
		"Diagnosis model: requested claude-opus-5-5, actual claude-opus-5-5.\n\n## Evolution Ideas\n")
}

// REQ-03, REQ-22: the confined diagnosis's output passes the sanitizer with
// the band worktree, its working directory, redacted as the project, and
// the user's checkout redacted as well.
func TestReactBandDiagnoseLocalPatch_OutputRedactsWorktreeAndCheckout(t *testing.T) {
	w := newDLPWorld(t, nil)
	key := healthband.LocalPatchKey(lpSeries, "e1042", lpDiagnoseClaimID)
	worktree := filepath.Join(w.lp(), key, "worktree")
	text := "### Summary\nRead " + worktree + "/pkg/foo/foo.go and " + w.repo + "/README.md.\n"
	w.fake.answer(t, 0, lpStream(lpInit55, lpAssistant55, lpResult(text)))

	outcome := w.claim(t, dlpClaim(lpDiagnoseClaimID, 2, lpT0.Add(990*time.Second)))

	require.Equal(t, bandDiagnosisOK, outcome.DiagnosisStatus)
	bs := w.bs(t, outcome.BSID)
	assert.Contains(t, bs, "Read <project>/pkg/foo/foo.go and <project>/README.md.\n")
	assert.NotContains(t, bs, w.lp())
	assert.NotContains(t, bs, w.repo)
}

// The checkout is redacted in its given and its real spelling, as the
// provider's pwd -P reports a checkout reached through a symlink.
func TestReactBandDiagnoseLocalPatch_RedactDirTakesBothSpellings(t *testing.T) {
	t.Parallel()
	real, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))

	got := bandRedactDir("a "+link+"/x.go b "+real+"/y.go", link)

	assert.Equal(t, "a <project>/x.go b <project>/y.go", got)
	assert.Equal(t, "keep / and none", bandRedactDir("keep / and none", "/"))
	assert.Empty(t, bandConfinedName(nil))
}

// lpUnsealedAll is lpUnsealed of every record.
func lpUnsealedAll(records []healthband.LocalPatchRecord) []healthband.LocalPatchRecord {
	out := make([]healthband.LocalPatchRecord, 0, len(records))
	for _, record := range records {
		out = append(out, lpUnsealed(record))
	}
	return out
}
