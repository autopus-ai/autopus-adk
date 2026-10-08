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

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-002 Git Execution Policy item 4 and the run output through
// the band command (plan task T8).

// The executor gets the default branch that 001's network step resolved;
// --no-fetch leaves it empty, so the base falls back to origin/HEAD.
func TestReactBandLocalPatch_DefaultBranch_ReachesTheExecutor(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"fetch resolves the default branch": {[]string{"--format", "json"}, "trunk"},
		"--no-fetch resolves none":          {[]string{"--no-fetch", "--format", "json"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			w := newBandLPWorld(t, bandLPConfig)
			w.storeO3()
			w.runner = scriptedBandRunner(bandCmdOriginURL, "trunk", "[]")
			w.enable.reports = func(func(healthband.LocalPatchRecord), healthband.DueClaim) {}
			run := w.band(tc.args...)
			require.NoError(t, run.err, run.stdout)
			require.NotNil(t, w.enable.patcher)
			assert.Equal(t, tc.want, w.enable.patcher.defaultBranch)
		})
	}
}

// The text output shows, for every local_patch claim of the run, its result
// status, the patch file, the changed files with their numstat counts, the
// patch request's requested and actual model, and the warning.
func TestReactBandLocalPatch_TextOutput_ShowsEveryLocalPatchClaim(t *testing.T) {
	warning := "  warning: This patch was derived by an AI model from untrusted CI logs; read the whole patch file before running anything.\n"
	done := healthband.NewLocalPatchResult(lpPatchClaimID, "")
	done.PatchPath, done.Files = "/cache/lp/"+lpKey+".patch", []healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 1, Removed: 1}}
	done.Models = []localPatchModel{
		{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"},
		{Request: lpRequestPatch, Requested: "claude-opus-5-5", Actual: "claude-opus-4-8", RefusalCategory: "cyber"},
	}
	refused := healthband.NewLocalPatchResult(lpPatchClaimID, "patch_model_refused")
	refused.Models = done.Models[1:]
	for name, tc := range map[string]struct {
		result healthband.LocalPatchRecord
		want   string
	}{
		"done": {done, "local patch " + lpPatchClaimID + " done patch=/cache/lp/" + lpKey + ".patch\n" +
			"  file pkg/foo/foo.go +1 -1\n  model requested=claude-opus-5-5 actual=claude-opus-4-8\n" + warning},
		"failed": {refused, "local patch " + lpPatchClaimID + " failed:patch_model_refused\n" +
			"  model requested=claude-opus-5-5 actual=claude-opus-4-8\n" + warning},
		"no patch request": {healthband.NewLocalPatchResult(lpPatchClaimID, "no_bs"),
			"local patch " + lpPatchClaimID + " failed:no_bs\n" + warning},
	} {
		t.Run(name, func(t *testing.T) {
			w := newBandLPWorld(t, bandLPConfig)
			w.storeO3()
			w.enable.reports = func(report func(healthband.LocalPatchRecord), _ healthband.DueClaim) { report(tc.result) }
			run := w.band("--no-fetch")
			require.NoError(t, run.err, run.stdout)
			_, rows, found := strings.Cut(run.stdout, "\nlocal patch ")
			require.True(t, found, run.stdout)
			assert.Equal(t, tc.want, "local patch "+rows, "the block follows the series rows")
		})
	}
}

// A flag-on run whose deps have no diagnose side refuses the flag before
// anything runs, so no diagnosis can run unconfined.
func TestReactBandLocalPatch_UnwiredDiagnoseSide_RefusesBeforeAnythingRuns(t *testing.T) {
	t.Parallel()
	p := newBandCmdProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "autopus.yaml"), []byte(bandLPConfig), 0o600))
	runner := scriptedBandRunner(bandCmdOriginURL, "main", "[]")
	cmd := newReactBandCmdWith(reactBandDeps{runner: runner, clock: fixedAt(bandLPT0), lockWait: healthband.StoreLockWait})
	cmd.SetArgs([]string{"--project-dir", p.dir})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	err := cmd.ExecuteContext(context.Background())
	require.ErrorIs(t, err, errBandConfinedUnwired)
	assert.Empty(t, runner.calls, "no git or gh call")
	_, statErr := os.Lstat(filepath.Join(p.dir, ".autopus"))
	assert.True(t, os.IsNotExist(statErr), "no store access")
}
