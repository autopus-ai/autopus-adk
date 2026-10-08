//go:build unix

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/lore"
)

// lpFooDiff changes two lines of pkg/foo/foo.go (one removed, one added).
const lpFooDiff = "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n" +
	"@@ -3,3 +3,3 @@\n func Foo() int {\n-\treturn 1\n+\treturn 2\n }\n"

func lpReplyWith(diff string) string { return "Proposal.\n\n```diff\n" + diff + "```\n" }

// lpS13Harness is the S13 configuration: an OMP-backed orchestra claude and
// health_band.local_patch_provider claude.
func lpS13Harness() *config.HarnessConfig {
	return lpHarness("claude", "claude", "", map[string]config.ProviderEntry{"claude": lpOMPEntry("anthropic/claude-opus-5-5:max")})
}

// lpPatchWorld is a fixture with a prepared worktree and a fake claude.
type lpPatchWorld struct {
	*lpFixture
	fake  lpFakeClaude
	setup *localPatchSetup
	input localPatchInput
}

func newLPPatchWorld(t *testing.T, harness *config.HarnessConfig, before ...func(*lpFixture)) *lpPatchWorld {
	t.Helper()
	fake := installLPFakeClaude(t)
	f := newLPFixture(t, harness)
	for _, hook := range before {
		hook(f)
	}
	setup := f.patcher.prepare(context.Background(), f.target())
	require.True(t, setup.ready(), "code %q", setup.code)
	reply := parseBandStream(lpStream(lpInit55, lpAssistant55, lpResult("### Summary")), lpRequestDiagnosis, "claude-opus-5-5")
	input := localPatchInput{
		outcome:   healthband.ClaimOutcome{BSID: "BS-BAND-001", DiagnosisStatus: bandDiagnosisOK},
		diagnosis: healthband.SanitizeProviderOutput("### Summary\nThe flaky step failed in pkg/foo/foo.go.\n", false, f.repo),
		logs:      []healthband.RunLog{{RunID: 4242, Attempt: 1, Evidence: healthband.SanitizeCILog("step 3 failed\n", false, f.repo)}},
		reply:     &reply,
	}
	fake.setStream(t, lpStream(lpInit55, lpAssistant55, lpResult(lpReplyWith(lpFooDiff))))
	return &lpPatchWorld{lpFixture: f, fake: fake, setup: setup, input: input}
}

func (w *lpPatchWorld) run(t *testing.T) healthband.LocalPatchRecord {
	t.Helper()
	result, written := w.patcher.patch(context.Background(), w.setup, w.input)
	require.True(t, written)
	return result
}

func TestLocalPatchPatch_Done_LocalArtifactsOnly(t *testing.T) {
	w := newLPPatchWorld(t, lpS13Harness())
	w.git(w.repo, "config", "author.name", "User")
	w.git(w.repo, "config", "author.email", "user@example.invalid")
	w.git(w.repo, "config", "committer.email", "user@example.invalid")
	require.NoError(t, os.WriteFile(filepath.Join(w.root, "band-home", ".gitconfig"), []byte("[author]\n\temail = global@example.invalid\n"), 0o600))
	userHead, userStatus := w.git(w.repo, "rev-parse", "HEAD"), w.git(w.repo, "status", "--porcelain")
	result := w.run(t)

	require.Equal(t, healthband.ClaimDone, result.Status)
	worktree, patchPath := filepath.Join(w.lp(), lpKey, "worktree"), filepath.Join(w.lp(), lpKey+".patch")
	assert.Equal(t, []string{"prep", "stage:worktree_intent", "stage:worktree_done", "stage:message", "stage:apply_intent",
		"stage:apply_done", "stage:commit_done", "stage:branch_intent", "stage:branch_done", "stage:patch_intent",
		"stage:patch_done", "result"}, w.ledger.trail())
	assert.Equal(t, lpPatchClaimID, result.ClaimID)
	assert.Equal(t, "BS-BAND-001", result.BSID)
	assert.Equal(t, w.base, result.BaseSHA)
	assert.Equal(t, "autopus/band/"+lpKey, result.Branch)
	assert.Equal(t, worktree, result.WorktreePath)
	assert.Equal(t, patchPath, result.PatchPath)
	assert.Equal(t, []healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 1, Removed: 1}}, result.Files)
	assert.Equal(t, []localPatchModel{
		{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"},
		{Request: lpRequestPatch, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"},
	}, result.Models)
	assert.False(t, result.ModelSubstituted)
	assert.Len(t, result.PromptManifest, 6)
	assert.Equal(t, lpUnsealed(w.ledger.result()), result, "the returned result is the one record written")
	assert.Equal(t, bandLocalPatchReport{
		ClaimID: lpPatchClaimID, Status: healthband.ClaimDone, PatchPath: patchPath, Files: result.Files,
		RequestedModel: "claude-opus-5-5", ActualModel: "claude-opus-5-5",
		Warning: "This patch was derived by an AI model from untrusted CI logs; read the whole patch file before running anything.",
	}, newBandLocalPatchReport(result))

	commit := result.CommitSHA
	assert.Equal(t, commit+"\n", w.git(w.repo, "rev-parse", "refs/heads/autopus/band/"+lpKey))
	assert.Equal(t, w.ledger.stage(healthband.StageCommitDone).CommitOID, commit)
	assert.Equal(t, w.base+"\n", w.git(w.repo, "rev-parse", commit+"^"))
	assert.Equal(t, w.ledger.stage(healthband.StageApplyIntent).Tree+"\n", w.git(w.repo, "rev-parse", commit+"^{tree}"))
	assert.Equal(t, commit+"\n", w.git(worktree, "rev-parse", "HEAD"))
	identity := w.git(w.repo, "log", "-1", "--format=%an <%ae>|%cn <%ce>", commit)
	assert.Equal(t, "autopus-band <band@autopus.invalid>|autopus-band <band@autopus.invalid>\n", identity)
	raw := w.git(w.repo, "cat-file", "commit", commit)
	_, message, _ := strings.Cut(raw, "\n\n")
	want, code := healthband.PatchCommitMessage(healthband.PatchCommitInput{
		Series: lpSeries, EpisodeID: "e1042", Tier: 3, Z: w.target().diagnose.Event.Z, BSID: "BS-BAND-001",
		PatchModel: "claude-opus-5-5", Lore: lore.LoreConfig{ForbiddenTrailers: []string{"Co-Authored-By"}},
	})
	require.Empty(t, code)
	assert.Equal(t, want, message, "the commit object's message equals the -F file bytes")
	assert.Contains(t, message, "Patch model: claude-opus-5-5\n")

	patch, err := os.ReadFile(patchPath)
	require.NoError(t, err)
	canonical := w.git(w.repo, healthband.GitFormatPatchArgs(w.base, commit)...)
	assert.Equal(t, canonical, string(patch), "the patch file is the canonical format-patch")
	assert.Equal(t, lpSHA256(patch), w.ledger.stage(healthband.StagePatchDone).PatchSHA256)
	assert.Equal(t, patchPath, w.ledger.stage(healthband.StagePatchIntent).Path)
	info, err := os.Lstat(patchPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	for _, gone := range []string{lpKey + ".diff", lpKey + ".lock", lpKey + ".patch.tmp-a1b2c3d4"} {
		_, err := os.Lstat(filepath.Join(w.lp(), gone))
		assert.True(t, os.IsNotExist(err), gone)
	}
	assert.Equal(t, userHead, w.git(w.repo, "rev-parse", "HEAD"))
	assert.Equal(t, userStatus, w.git(w.repo, "status", "--porcelain"))
	assert.Empty(t, w.git(w.repo, "stash", "list"))
	assert.Equal(t, "call\n", w.fake.record(t, "calls"), "one provider call: the patch request")
	assert.Equal(t, worktree+"\n", w.fake.record(t, "cwd"))
	argv := w.fake.record(t, "argv")
	for _, flag := range []string{"--restricted\n", "--verbose\n", "stream-json\n", "--strict-mcp-config\n", "--tools=Read,Grep,Glob\n"} {
		assert.Contains(t, argv, flag)
	}
	assert.False(t, w.absent(lpKey), "the worktree is kept for review")
}
