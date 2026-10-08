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
)

func TestLocalPatchPatch_FailureCodes(t *testing.T) {
	diagnosisNotOK := func(w *lpPatchWorld) { w.input.outcome.DiagnosisStatus = bandUnavailable(bandProviderTimedOut) }
	stream := func(lines ...string) func(w *lpPatchWorld) {
		return func(w *lpPatchWorld) { w.fake.setStream(t, lpStream(lines...)) }
	}
	reply := lpResult(lpReplyWith(lpFooDiff))
	cases := []struct {
		name    string
		mutate  func(w *lpPatchWorld)
		want    string
		calls   int  // provider calls
		objects bool // steps 1–7 only: no object written, no apply_intent
	}{
		{"no BS", func(w *lpPatchWorld) { w.input.outcome.BSID = "" }, "failed:no_bs", 0, true},
		{"diagnosis unavailable", diagnosisNotOK, "failed:diagnosis_unavailable", 0, true},
		{"branch created after step 1", func(w *lpPatchWorld) { w.git(w.repo, "branch", "autopus/band/"+lpKey) }, "failed:branch_exists", 0, true},
		{"symbolic ref created after step 1", func(w *lpPatchWorld) {
			w.git(w.repo, "symbolic-ref", "refs/heads/autopus/band/"+lpKey, "refs/heads/main")
		}, "failed:branch_exists", 0, true},
		{"required Related", func(w *lpPatchWorld) { w.patcher.harness.Lore.RequiredTrailers = []string{"Related"} }, "failed:lore_unsupported_required:Related", 0, true},
		{"forbidden Related", func(w *lpPatchWorld) {
			forbid := []string{"Related"}
			w.patcher.harness.Lore.ForbiddenTrailers = &forbid
		}, "failed:lore_rejected", 0, true},
		{"provider key names codex at the patch request", func(w *lpPatchWorld) { w.patcher.harness.HealthBand.LocalPatchProvider = "codex" },
			"failed:patch_provider_unconfined", 0, true},
		{"model refusal fallback", stream(lpInit55, lpFallback48, lpAssistant48, reply), "failed:patch_model_refused", 1, true},
		{"no init event", stream(lpAssistant55, reply), "failed:patch_model_unverified", 1, true},
		{"assistant on another model", stream(lpInit55, lpAssistant48, reply), "failed:patch_model_unverified", 1, true},
		{"init other than requested", stream(lpInit48, lpAssistant48, reply), "failed:patch_model_unverified", 1, true},
		{"assistant without model", stream(lpInit55, lpAssistantNil, reply), "failed:patch_model_unverified", 1, true},
		{"no success result", stream(lpInit55, lpAssistant55), "failed:provider_empty_output", 1, true},
		{"no diff fence", stream(lpInit55, lpAssistant55, lpResult("No safe change exists.")), "failed:no_patch", 1, true},
		{"result text past 1 MiB", stream(lpInit55, lpAssistant55, lpResult(strings.Repeat("a", 1<<20+1))), "failed:patch_too_large", 1, true},
		{"denied Makefile", stream(lpInit55, lpAssistant55, lpResult(lpReplyWith(
			"diff --git a/Makefile b/Makefile\nnew file mode 100644\n--- /dev/null\n+++ b/Makefile\n@@ -0,0 +1 @@\n+all:\n"))),
			"failed:path_denied", 1, true},
		{"file staged before the commit", func(w *lpPatchWorld) {
			w.patcher.beforeCommit = func(worktree string) {
				require.NoError(t, os.WriteFile(filepath.Join(worktree, "README.md"), []byte("changed\n"), 0o644))
				w.git(worktree, "add", "README.md")
			}
		}, "failed:commit_tree_mismatch", 1, false},
		{"lease short of the patch request group", func(w *lpPatchWorld) {
			w.patcher.now = func() time.Time { return lpT0.Add(1100 * time.Second) } // 700 s left
		}, "failed:lease_exhausted", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newLPPatchWorld(t, lpS13Harness())
			tc.mutate(w)
			objects := w.git(w.repo, "count-objects", "-v")
			result := w.run(t)
			assert.Equal(t, tc.want, result.Status)
			assert.Equal(t, strings.Repeat("call\n", tc.calls), w.fake.record(t, "calls"))
			assert.Equal(t, []string{lpPatchClaimID}, w.cleaner.calls, "the Cleanup Rules handle the claim")
			trail := w.ledger.trail()
			if tc.objects {
				assert.Equal(t, objects, w.git(w.repo, "count-objects", "-v"), "no object written")
				assert.NotContains(t, trail, "stage:apply_intent")
			}
			assert.Equal(t, "result", trail[len(trail)-1])
			if tc.want == "failed:branch_exists" {
				assert.Equal(t, w.base+"\n", w.git(w.repo, "rev-parse", "refs/heads/main"), "the user's branch is unchanged")
			} else {
				assert.Empty(t, w.git(w.repo, "for-each-ref", "refs/heads/autopus/band/"), "no branch created by the claim")
			}
			_, err := os.Lstat(filepath.Join(w.lp(), lpKey+".patch"))
			assert.True(t, os.IsNotExist(err), "no patch file")
			_, err = os.Lstat(filepath.Join(w.lp(), lpKey))
			assert.True(t, os.IsNotExist(err), "no worktree")
		})
	}
}

func TestLocalPatchPatch_ModelRefused_RecordsBothModels(t *testing.T) {
	w := newLPPatchWorld(t, lpS13Harness())
	w.fake.setStream(t, lpStream(lpInit55, lpFallback48, lpAssistant48, lpResult(lpReplyWith(lpFooDiff))))
	result := w.run(t)
	assert.Equal(t, "failed:patch_model_refused", result.Status)
	assert.True(t, result.ModelSubstituted)
	assert.Equal(t, []localPatchModel{
		{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"},
		{Request: lpRequestPatch, Requested: "claude-opus-5-5", Actual: "claude-opus-4-8", RefusalCategory: "cyber"},
	}, result.Models)
	assert.NotContains(t, w.ledger.trail(), "stage:message", "the claim ends before the message and any Patch Policy check")
}

func TestLocalPatchPatch_PrepFailures_EndWithTheirCode(t *testing.T) {
	f := newLPFixture(t, nil)
	f.patcher.defaultBranch = "release"
	s := f.patcher.prepare(t.Context(), f.target())
	result, written := f.patcher.patch(t.Context(), s, localPatchInput{})
	require.True(t, written)
	assert.Equal(t, "failed:base_unavailable", result.Status)
	assert.Empty(t, f.cleaner.calls, "no worktree exists")

	f = newLPFixture(t, nil)
	f.ledger.failPrep = true
	s = f.patcher.prepare(t.Context(), f.target())
	result, _ = f.patcher.patch(t.Context(), s, localPatchInput{})
	assert.Equal(t, "failed:record_unavailable", result.Status)
}
