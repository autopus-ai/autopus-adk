//go:build unix

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-002 integration verification (plan task T10) through the
// production command: newReactBandCmd() with its real runner, clock, user
// cache directory, git, store, and diagnose side ((*bandDiagnoser).
// enableLocalPatch); only gh and claude are fake binaries on PATH. Expected
// values come from acceptance.md and spec.md text or from git itself, never
// from the package under test.

// lpitWarning is the run output's warning sentence (spec.md BS Record).
const lpitWarning = "This patch was derived by an AI model from untrusted CI logs; read the whole patch file before running anything."

// lpitFormatPatch is the canonical format-patch of Local Patch Flow step 11.
var lpitFormatPatch = strings.Fields("format-patch --stdout --no-signature --no-thread --no-numbered --no-cover-letter " +
	"--no-notes --no-attach --no-add-header --no-to --no-cc --no-from --no-base --no-signoff --subject-prefix=PATCH " +
	"--full-index --no-textconv --no-ext-diff")

// S4 (with the S7, S12, and S13 argv checks of the same run): the flag on
// produces one local patch outside the repository and nothing remote.
func TestReactBandLocalPatchIT_S4_ProductionCommandLeavesOneLocalPatch(t *testing.T) {
	w := newLPITWorld(t, lpitConfig)
	w.answerS4()
	before := w.snapshot()
	run := w.run(nil, "--format", "json")
	require.NoError(t, run.err, run.stdout+run.stderr)

	claim := w.record(healthband.LocalPatchKindClaim, "")
	key, lp := claim.Key, w.lp()
	require.Regexp(t, `^[0-9a-f]{32}$`, claim.ClaimID)
	require.Equal(t, "ci-failure-rate-ci-c6d37d0a-e1042-"+claim.ClaimID[:8], key, "<series-slug>-<h8>-<episode-id>-<c8>")
	worktree, patchPath, branch := filepath.Join(lp, key, "worktree"), filepath.Join(lp, key+".patch"), "autopus/band/"+key
	prep, intent := w.record(healthband.LocalPatchKindPrep, ""), w.record(healthband.LocalPatchKindStage, healthband.StageApplyIntent)
	assert.Equal(t, w.base, prep.BaseSHA)
	assert.Equal(t, []string{"decision", "claim", "prep", "stage:worktree_intent", "stage:worktree_done", "stage:message",
		"stage:apply_intent", "stage:apply_done", "stage:commit_done", "stage:branch_intent", "stage:branch_done",
		"stage:patch_intent", "stage:patch_done", "result"}, w.trail())

	// The branch, its commit, the worktree at it, and the patch file.
	commit := strings.TrimSpace(w.inspect("rev-parse", "refs/heads/"+branch))
	tree, parents, message, ok := lpParseCommit(w.inspect("cat-file", "commit", commit))
	require.True(t, ok)
	assert.Equal(t, []string{w.base}, parents, "the only parent is the prep base")
	assert.Equal(t, intent.Tree, tree, "the apply_intent expected tree")
	assert.Equal(t, w.record(healthband.LocalPatchKindStage, healthband.StageCommitDone).CommitOID, commit)
	assert.Contains(t, w.inspect("worktree", "list", "--porcelain"), "worktree "+worktree+"\nHEAD "+commit+"\ndetached\n")
	patch, err := os.ReadFile(patchPath)
	require.NoError(t, err)
	assert.Equal(t, w.inspect(append(slices.Clone(lpitFormatPatch), w.base+".."+commit)...), string(patch), "the canonical format-patch, byte for byte")
	sum := sha256.Sum256(patch)
	assert.Equal(t, hex.EncodeToString(sum[:]), w.record(healthband.LocalPatchKindStage, healthband.StagePatchDone).PatchSHA256)
	assert.Equal(t, hex.EncodeToString(sum[:]), w.record(healthband.LocalPatchKindStage, healthband.StagePatchIntent).PatchSHA256)
	w.assertCommitMessage(t, commit, message)

	// The result record and the run output.
	result := w.record(healthband.LocalPatchKindResult, "")
	assert.Equal(t, healthband.LocalPatchRecord{Status: "done", BSID: "BS-BAND-001", BaseSHA: w.base, CommitSHA: commit,
		Branch: branch, WorktreePath: worktree, PatchPath: patchPath},
		healthband.LocalPatchRecord{Status: result.Status, BSID: result.BSID, BaseSHA: result.BaseSHA, CommitSHA: result.CommitSHA,
			Branch: result.Branch, WorktreePath: result.WorktreePath, PatchPath: result.PatchPath})
	// The JSON envelope masks the home directory as ~ in every string
	// (output_json.go maskHomePath), so the patch path is home-relative there.
	assert.Equal(t, []bandLocalPatchReport{{ClaimID: claim.ClaimID, Status: "done", PatchPath: "~" + strings.TrimPrefix(patchPath, w.home),
		Files:          []healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 1, Removed: 1}},
		RequestedModel: "claude-opus-5-5", ActualModel: "claude-opus-5-5", Warning: lpitWarning,
	}}, decodeBandLPEnvelope(t, run.stdout).Data.LocalPatches)
	w.assertBSPointer(t, lp, key, claim.ClaimID)
	w.assertModes(t, lp, patchPath)

	// Nothing else changed, nothing ran, nothing remote.
	w.assertOnlyBandArtifacts(t, before, key)
	w.assertRepoUnchanged(t, before, branch)
	w.assertNoHookNorNetwork(t)
	w.assertNoRemoteCall(t)
	w.assertPolicyArgv(t, key, commit)
	w.assertConfinedCalls(t, worktree, 2)
	_, err = os.Lstat(filepath.Join(w.repo, ".git", "lfs"))
	assert.True(t, os.IsNotExist(err), "no $GIT_COMMON_DIR/lfs/")
}

// S4: the text output of the same flow shows the local_patch claim's block
// after the series rows.
func TestReactBandLocalPatchIT_S4_TextOutputShowsTheLocalPatch(t *testing.T) {
	w := newLPITWorld(t, lpitConfig)
	w.answerS4()
	run := w.run(nil)
	require.NoError(t, run.err, run.stdout+run.stderr)
	claim := w.record(healthband.LocalPatchKindClaim, "")
	_, block, found := strings.Cut(run.stdout, "\nlocal patch ")
	require.True(t, found, run.stdout)
	assert.Equal(t, "local patch "+claim.ClaimID+" done patch="+filepath.Join(w.lp(), claim.Key+".patch")+"\n"+
		"  file pkg/foo/foo.go +1 -1\n  model requested=claude-opus-5-5 actual=claude-opus-5-5\n  warning: "+lpitWarning+"\n",
		"local patch "+block)
}

// The S4 fixture is live, so the 0-marker and 0-hook oracles are not
// vacuous: a commit by the user's own git, outside band, runs the hooks of
// the relative core.hooksPath, writes their markers, and logs trace2 hook
// events.
func TestReactBandLocalPatchIT_FixtureHooksRunOutsideBand(t *testing.T) {
	w := newLPITWorld(t, lpitConfig)
	cmd := exec.Command(w.realGit, "-c", "user.name=User", "commit", "--allow-empty", "-q", "-m", "user commit")
	cmd.Dir = w.repo
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	for _, hook := range []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit", "reference-transaction"} {
		_, err := os.Lstat(filepath.Join(w.markers, "tracked-"+hook))
		assert.NoError(t, err, "the %s hook ran", hook)
	}
	hooks := 0
	for _, event := range w.traceEvents() {
		if event.Event == "child_start" && event.ChildClass == "hook" {
			hooks++
		}
	}
	assert.Positive(t, hooks, "trace2 logs hook children through the test HOME")
}

// assertCommitMessage: the band identity although the repository and the
// global configuration set author.* and committer.*, a message whose bytes
// hash to the message stage (the -F file), and the S4 lines.
func (w *lpitWorld) assertCommitMessage(t *testing.T, commit, message string) {
	t.Helper()
	raw := w.inspect("cat-file", "commit", commit)
	assert.Regexp(t, regexp.MustCompile(`(?m)^author autopus-band <band@autopus\.invalid> \d+ [+-]\d{4}$`), raw)
	assert.Regexp(t, regexp.MustCompile(`(?m)^committer autopus-band <band@autopus\.invalid> \d+ [+-]\d{4}$`), raw)
	sum := sha256.Sum256([]byte(message))
	assert.Equal(t, hex.EncodeToString(sum[:]), w.record(healthband.LocalPatchKindStage, healthband.StageMessage).MessageSHA256)
	lines := strings.Split(message, "\n")
	assert.Equal(t, "fix(band): ci-failure-rate-ci anomaly local patch (e1042)", lines[0])
	assert.Contains(t, lines, "Patch model: claude-opus-5-5", "the --model value of the patch request")
	assert.Contains(t, lines, "Related: BS-BAND-001")
	assert.True(t, strings.HasSuffix(message, "\n🐙 Autopus <noreply@autopus.co>\n"), message)
}

// assertBSPointer: BS-BAND-001's 추천 방향 section ends with the three
// pointer lines and the Diagnosis model line, and the three local-patch
// sentences replace 001's diagnosis-only ones (spec.md BS Record).
func (w *lpitWorld) assertBSPointer(t *testing.T, lp, key, claimID string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.repo, ".autopus", "brainstorms", "BS-BAND-001.md"))
	require.NoError(t, err)
	bs := string(data)
	section, _, found := strings.Cut(bs[strings.Index(bs, "## 추천 방향\n"):], "\n\n## Evolution Ideas\n")
	require.True(t, found, bs)
	want := "\n\nLocal patch (3σ, local only, if produced): branch autopus/band/" + key + ", worktree " + lp + "/" + key +
		"/worktree/, patch file " + lp + "/" + key + ".patch, outside this repository.\n\n" +
		"The outcome, the changed files, and the model that wrote the patch are in the run output and in " +
		".autopus/metrics/localpatch-events.jsonl under claim " + claimID + "; nothing was pushed.\n\n" +
		"Reviewer warning: this local branch holds a patch that an AI model derived from untrusted CI logs and that nothing " +
		"has run. Read the whole patch file before you open the worktree in an IDE, run any command or agent in it, or push " +
		"the branch, because repository hooks and tool configuration files run on checkout, commit, and build, and the edit " +
		"guard does not cover that worktree.\n\nDiagnosis model: requested claude-opus-5-5, actual claude-opus-5-5."
	assert.True(t, strings.HasSuffix(section, want), section)
	assert.Regexp(t, `(?m)^- When: detected \S+; tier 3 with health_band\.allow_local_patch may leave one local patch outside `+
		`this repository \(see 추천 방향\); band pushes nothing\.$`, bs)
	assert.Contains(t, bs, "- Explicit non-goals: changes unrelated to ")
	assert.Contains(t, bs, "; band itself pushes nothing and changes no GitHub state.\n")
	assert.Contains(t, section, "Tier 3 with health_band.allow_local_patch adds at most one local branch, worktree, and patch "+
		"file and opens no pull request.")
}

// assertModes: <lp> is 0700 and the patch file 0600 (REQ-14).
func (w *lpitWorld) assertModes(t *testing.T, lp, patchPath string) {
	t.Helper()
	info, err := os.Lstat(lp)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Lstat(patchPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// assertOnlyBandArtifacts: outside .git/ only the BS and the metrics files
// changed; inside .git/ nothing changed or went away, and only the band
// branch with its reflog, one worktrees/<name>/ entry, and loose objects
// appeared.
func (w *lpitWorld) assertOnlyBandArtifacts(t *testing.T, before lpitSnapshot, key string) {
	t.Helper()
	after := w.snapshot()
	outside := func(files map[string]string) map[string]string {
		kept := map[string]string{}
		for rel, sum := range files {
			if rel != ".autopus/brainstorms/BS-BAND-001.md" && !strings.HasPrefix(rel, ".autopus/metrics/") {
				kept[rel] = sum
			}
		}
		return kept
	}
	assert.Equal(t, outside(before.files), outside(after.files))
	_, err := os.Lstat(filepath.Join(w.repo, ".autopus", "brainstorms", "BS-BAND-001.md"))
	assert.NoError(t, err)
	object := regexp.MustCompile(`^objects/[0-9a-f]{2}/[0-9a-f]{38}$`)
	admin := map[string]bool{}
	for rel, sum := range after.gitDir {
		if old, existed := before.gitDir[rel]; existed {
			assert.Equal(t, old, sum, ".git/%s changed", rel)
			continue
		}
		name, inAdmin := strings.CutPrefix(rel, "worktrees/")
		switch {
		case inAdmin:
			admin[strings.SplitN(name, "/", 2)[0]] = true
		case object.MatchString(rel), rel == "refs/heads/autopus/band/"+key, rel == "logs/refs/heads/autopus/band/"+key:
		default:
			assert.Fail(t, "an unexpected new file inside .git/", rel)
		}
	}
	for rel := range before.gitDir {
		_, kept := after.gitDir[rel]
		assert.True(t, kept, ".git/%s went away", rel)
	}
	assert.Len(t, admin, 1, "one worktrees/<name>/ entry")
	assert.Contains(t, after.gitDir, "refs/heads/autopus/band/"+key)
}
