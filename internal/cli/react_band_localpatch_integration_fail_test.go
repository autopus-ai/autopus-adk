//go:build unix

package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// One failure path per family through the band command with the real
// diagnose side: a Patch Policy denial (S5), a model refusal fallback
// (S15), a configuration that turns unsafe during the patch request (S6),
// and a live checkout that outlives the setup deadline (S9).

// S5 policy family: a reply whose diff also adds .github/workflows/ci.yaml
// ends path_denied at step 7 with no object, branch, patch file, diff
// file, or worktree left, the BS kept, and exit 0.
func TestReactBandLocalPatchIT_S5_PolicyDenialLeavesNothing(t *testing.T) {
	w := newLPITWorld(t, lpitConfig)
	marker := "lpit-denied-7f3a9c"
	diff := lpFooDiff + "diff --git a/.github/workflows/ci.yaml b/.github/workflows/ci.yaml\nnew file mode 100644\n" +
		"--- /dev/null\n+++ b/.github/workflows/ci.yaml\n@@ -0,0 +1 @@\n+name: " + marker + "\n"
	w.answerS4()
	w.answer(2, lpStream(lpInit55, lpAssistant55, lpResult(lpReplyWith(diff))))
	before := w.snapshot()
	run := w.run(nil, "--format", "json")
	require.NoError(t, run.err, run.stdout+run.stderr)

	result := w.record(healthband.LocalPatchKindResult, "")
	assert.Equal(t, "failed:path_denied", result.Status)
	assert.Empty(t, result.Kept)
	assert.Empty(t, result.Files, "files[] only once step 7 passed")
	w.assertClaimLeftNothing(t, w.record(healthband.LocalPatchKindClaim, "").Key)
	w.assertNoObjectWrite(t, before, marker)
	w.assertRepoUnchanged(t, before, "")
	w.assertNoHookNorNetwork(t)
	assert.Equal(t, 2, w.claudeCalls())
}

// S15 model family: a patch stream with a model_refusal_fallback event and
// 3 MiB of tool events ends failed:patch_model_refused at step 6 with both
// models[] entries, before any Patch Policy check or object write.
func TestReactBandLocalPatchIT_S15_RefusalFallbackEndsThePatchRequest(t *testing.T) {
	w := newLPITWorld(t, lpitConfig)
	w.answerS4()
	tools := `{"type":"user","message":{"content":[{"type":"tool_result","content":"` + strings.Repeat("t", 3<<20) + `"}]}}`
	w.answer(2, lpStream(lpInit55, lpFallback48, tools, lpAssistant48, lpResult(lpReplyWith(lpFooDiff))))
	before := w.snapshot()
	run := w.run(nil, "--format", "json")
	require.NoError(t, run.err, run.stdout+run.stderr)

	result := w.record(healthband.LocalPatchKindResult, "")
	assert.Equal(t, "failed:patch_model_refused", result.Status)
	assert.True(t, result.ModelSubstituted)
	assert.Equal(t, []healthband.LocalPatchModel{
		{Request: "diagnosis", Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"},
		{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-4-8", RefusalCategory: "cyber"},
	}, result.Models)
	assert.NotContains(t, w.trail(), "stage:message", "the claim ended before the final message")
	w.assertClaimLeftNothing(t, w.record(healthband.LocalPatchKindClaim, "").Key)
	w.assertNoObjectWrite(t, before, "")
	assert.Empty(t, w.policyCalls("apply"), "no Patch Policy check ran")
	assert.Equal(t, []bandLocalPatchReport{{ClaimID: result.ClaimID, Status: "failed:patch_model_refused",
		Files: []healthband.PatchFile{}, RequestedModel: "claude-opus-5-5", ActualModel: "claude-opus-4-8", Warning: lpitWarning,
	}}, decodeBandLPEnvelope(t, run.stdout).Data.LocalPatches)
}

// S6 configuration family: a repository configuration that gains
// filter.mark.clean while the patch request runs ends the claim at step 8
// with no object, branch, or diff file, while Cleanup Rule 3 runs item 3
// inside the worktree first and keeps the checked-out worktree unread with
// reason git_config_unsafe and no git status in it.
func TestReactBandLocalPatchIT_S6_ConfigTurnsUnsafeDuringThePatchRequest(t *testing.T) {
	w := newLPITWorld(t, lpitConfig)
	w.answerS4()
	w.hook(2, `"$LPIT_REALGIT" -C "`+w.repo+`" config filter.mark.clean "touch `+filepath.Join(w.markers, "mark-clean")+`"`+"\n")
	before := w.snapshot()
	run := w.run(nil, "--format", "json")
	require.NoError(t, run.err, run.stdout+run.stderr)

	key := w.record(healthband.LocalPatchKindClaim, "").Key
	worktree := filepath.Join(w.lp(), key, "worktree")
	result := w.record(healthband.LocalPatchKindResult, "")
	assert.Equal(t, "failed:git_config_unsafe:filter.mark.clean", result.Status)
	assert.Equal(t, []healthband.LocalPatchKept{{Artifact: "worktree", Reason: "git_config_unsafe"}}, result.Kept)
	assert.NotContains(t, w.trail(), "stage:apply_intent", "step 8 stopped before the expected tree")
	assert.Contains(t, w.inspect("worktree", "list", "--porcelain"), "worktree "+worktree+"\nHEAD "+w.base+"\n")
	data, err := os.ReadFile(filepath.Join(worktree, "pkg", "foo", "foo.go"))
	require.NoError(t, err)
	assert.Equal(t, lpFooGo, string(data), "the kept worktree holds the base checkout")
	assert.True(t, w.refAbsent("refs/heads/autopus/band/"+key))
	for _, name := range []string{key + ".diff", key + ".patch", key + ".lock"} {
		_, err := os.Lstat(filepath.Join(w.lp(), name))
		assert.True(t, os.IsNotExist(err), name)
	}
	w.assertNoObjectWrite(t, before, "")
	checks, statuses, afterRequest := 0, 0, false
	for _, call := range w.calls() {
		afterRequest = afterRequest || call.claude == 2
		switch sub := call.sub(); {
		case !afterRequest || call.dir != worktree:
		case sub == "status":
			statuses++
		case sub == "config":
			checks++
		}
	}
	assert.Zero(t, statuses, "Cleanup Rule 3 read no file of the unsafe worktree")
	assert.Equal(t, 2, checks, "item 3 at step 8 and in Cleanup Rule 3")
	w.assertNoHookNorNetwork(t)
}

// S9 live removal: a step-2 checkout that ignores SIGTERM gets SIGKILL at
// the setup deadline (shortened through the executor seam), writes
// worktree_failed, and is removed with exactly one git worktree remove
// --force and no unlock, leaving no admin entry, no <lp>/<key>/, and no
// retention slot; the diagnosis reports unavailable(worktree_unavailable).
func TestReactBandLocalPatchIT_S9_StoppedCheckoutIsRemovedAtOnce(t *testing.T) {
	w := newLPITWorld(t, lpitConfig)
	w.answerS4()
	t.Setenv("LPIT_GIT_FAULT", "stall-checkout")
	deps := lpitDeps(func(lp *bandLocalPatchDeps) {
		lp.git.StopGrace = 2 * time.Second
		lp.patcher = func(p *bandLocalPatcher) { p.groups.setup = 6 * time.Second }
	})
	start := time.Now()
	run := w.run(&deps, "--format", "json")
	require.NoError(t, run.err, run.stdout+run.stderr)
	assert.GreaterOrEqual(t, time.Since(start), 6*time.Second, "the checkout ran until the setup deadline")

	key := w.record(healthband.LocalPatchKindClaim, "").Key
	worktree := filepath.Join(w.lp(), key, "worktree")
	assert.Equal(t, []string{"decision", "claim", "prep", "stage:worktree_intent", "stage:worktree_failed", "result"}, w.trail())
	assert.Equal(t, "worktree_failed", w.record(healthband.LocalPatchKindStage, healthband.StageWorktreeFailed).Code)
	result := w.record(healthband.LocalPatchKindResult, "")
	assert.Equal(t, "failed:worktree_failed", result.Status)
	assert.Empty(t, result.Kept)
	var removes []string
	for _, call := range w.policyCalls("worktree") {
		_, args, _ := call.policy()
		assert.NotEqual(t, "unlock", args[1], "no git worktree unlock")
		if args[1] == "remove" {
			removes = append(removes, strings.Join(args, " "))
		}
	}
	assert.Equal(t, []string{"worktree remove --force " + worktree}, removes, "one remove with a single --force")
	_, err := os.Lstat(filepath.Join(w.gitrec, "stall.term"))
	assert.NoError(t, err, "SIGTERM reached the checkout's process group first")
	_, err = os.Lstat(filepath.Join(w.gitrec, "stall.done"))
	assert.True(t, os.IsNotExist(err), "SIGKILL ended the checkout")
	w.assertClaimLeftNothing(t, key)
	kept, err := healthband.CountKeptKeys(w.lp())
	require.NoError(t, err)
	assert.Zero(t, kept, "a stopped checkout takes no retention slot")
	assert.Zero(t, w.claudeCalls(), "no worktree, no provider")
	_, results := bandITEvents(t, bandCmdProject{t: t, dir: w.repo})
	require.Len(t, results, 1)
	assert.Equal(t, bandUnavailable(bandWorktreeUnavailable), results[0].DiagnosisStatus)
}

// lpitDeps are production's band deps, which edit may adjust.
func lpitDeps(edit func(*bandLocalPatchDeps)) reactBandDeps {
	deps := reactBandDeps{runner: execBandRunner{}, clock: time.Now, lockWait: healthband.StoreLockWait,
		localPatch: bandLocalPatchDeps{enable: (*bandDiagnoser).enableLocalPatch}}
	if edit != nil {
		edit(&deps.localPatch)
	}
	return deps
}

// assertClaimLeftNothing: no worktree of <lp> is registered, and neither
// <lp>/<key>/, the patch, diff, or lock file, nor the band branch exists.
func (w *lpitWorld) assertClaimLeftNothing(t *testing.T, key string) {
	t.Helper()
	lp := w.lp()
	assert.NotContains(t, w.inspect("worktree", "list", "--porcelain"), lp)
	for _, name := range []string{key, key + ".patch", key + ".diff", key + ".lock"} {
		_, err := os.Lstat(filepath.Join(lp, name))
		assert.True(t, os.IsNotExist(err), "<lp>/%s", name)
	}
	assert.True(t, w.refAbsent("refs/heads/autopus/band/"+key), "no band branch")
	_, err := os.Lstat(filepath.Join(w.repo, ".autopus", "brainstorms", "BS-BAND-001.md"))
	assert.NoError(t, err, "the BS is kept")
}

// refAbsent reports that git knows no ref of that name, symbolic or not.
func (w *lpitWorld) refAbsent(ref string) bool {
	cmd := exec.Command(w.realGit, "show-ref", "--verify", "--quiet", ref)
	cmd.Dir, cmd.Env = w.repo, w.setupEnv
	err := cmd.Run()
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 1
}

// records are the local patch records of the band project's store.
func (w *lpitWorld) records() []healthband.LocalPatchRecord {
	w.t.Helper()
	log, err := healthband.NewStore(w.repo).ReadLocalPatchLog()
	require.NoError(w.t, err)
	return log.Records
}

// record is the first record of kind (and of phase for a stage).
func (w *lpitWorld) record(kind, phase string) healthband.LocalPatchRecord {
	w.t.Helper()
	for _, record := range w.records() {
		if record.Kind == kind && (phase == "" || record.Phase == phase) {
			return record
		}
	}
	w.t.Fatalf("no %s %s record", kind, phase)
	return healthband.LocalPatchRecord{}
}

// trail spells the records as kind or stage:phase.
func (w *lpitWorld) trail() []string {
	var trail []string
	for _, record := range w.records() {
		if record.Kind == healthband.LocalPatchKindStage {
			trail = append(trail, "stage:"+record.Phase)
			continue
		}
		trail = append(trail, record.Kind)
	}
	return trail
}
