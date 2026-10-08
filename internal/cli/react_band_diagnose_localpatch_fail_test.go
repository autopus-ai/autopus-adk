//go:build unix

package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// The failure paths of a flag-on diagnosis: each reports its unavailable
// reason in the BS and the outcome, and its local_patch claim ends with the
// REQ-04 code, at most one provider call, and no worktree left.

func dlpTierThree() healthband.DueClaim {
	return dlpClaim(lpDiagnoseClaimID, 3, lpT0.Add(990*time.Second))
}

// S14, S7: without local_patch_provider an all-OMP orchestra selects no
// subprocess claude, so the diagnosis is unconfined and no provider runs.
func TestReactBandDiagnoseLocalPatch_UnconfinedProviderRunsNothing(t *testing.T) {
	allOMP := map[string]config.ProviderEntry{
		"claude": lpOMPEntry("anthropic/claude-opus-5-5:max"), "codex": lpOMPEntry("openai/gpt-5.5"), "gemini": lpOMPEntry("google/gemini-3"),
	}
	verbose := map[string]config.ProviderEntry{"claude": {Binary: "claude", Args: []string{"--print", "--verbose"}}}
	for name, tc := range map[string]struct {
		harness *config.HarnessConfig
		status  string
	}{
		"all OMP":            {lpHarness("", "claude", "", allOMP), "unavailable(provider_unconfined)"},
		"spaces only":        {lpHarness("   ", "claude", "", allOMP), "unavailable(provider_unconfined)"},
		"key names codex":    {lpHarness("codex", "claude", "", map[string]config.ProviderEntry{"claude": {Binary: "claude"}}), "unavailable(provider_unconfined)"},
		"001 selects codex":  {lpHarness("", "claude", "codex", allOMP), "unavailable(provider_unconfined)"},
		"configured verbose": {lpHarness("claude", "claude", "", verbose), "unavailable(provider_policy_rejected)"},
	} {
		t.Run(name, func(t *testing.T) {
			w := newDLPWorld(t, tc.harness, dlpPatchClaim())
			outcome := w.claim(t, dlpTierThree())

			assert.Equal(t, tc.status, outcome.DiagnosisStatus)
			assert.Contains(t, w.bs(t, outcome.BSID), "\ndiagnosis_status: "+tc.status+"\n")
			assert.Equal(t, 0, w.fake.calls(t))
			require.Len(t, w.reports, 1)
			assert.Equal(t, "failed:diagnosis_unavailable", w.reports[0].Status)
			assert.Empty(t, w.reports[0].Models, "no request returned a stream")
			assert.NotContains(t, w.bs(t, outcome.BSID), "Diagnosis model:")
			assert.Empty(t, w.worktrees(t))
			assert.True(t, w.absent(lpKey))
			// C1: the provider is resolved before step 1, so no prep, stage,
			// key lock, or worktree exists; only the claim's result is written.
			assert.Equal(t, []string{healthband.LocalPatchKindResult}, dlpKinds(w.ledger.records()))
		})
	}
	// A diagnosis without a local_patch claim then writes no record at all.
	w := newDLPWorld(t, lpHarness("", "claude", "", allOMP))
	outcome := w.claim(t, dlpTierThree())
	assert.Equal(t, "unavailable(provider_unconfined)", outcome.DiagnosisStatus)
	assert.Empty(t, w.ledger.records())
	assert.Empty(t, w.worktrees(t))
	assert.Equal(t, 0, w.fake.calls(t))
}

// dlpKinds spells records as their kinds.
func dlpKinds(records []healthband.LocalPatchRecord) []string {
	var kinds []string
	for _, record := range records {
		kinds = append(kinds, record.Kind)
	}
	return kinds
}

// S3: a prep code makes the diagnosis unavailable(worktree_unavailable)
// before any provider call, and the claim ends with that code.
func TestReactBandDiagnoseLocalPatch_PrepCodeMakesWorktreeUnavailable(t *testing.T) {
	w := newDLPWorld(t, nil, dlpPatchClaim())
	w.patcher.defaultBranch = "release"

	outcome := w.claim(t, dlpTierThree())

	assert.Equal(t, "unavailable(worktree_unavailable)", outcome.DiagnosisStatus)
	assert.Equal(t, "BS-BAND-001", outcome.BSID, "001 writes the evidence-only BS")
	assert.Equal(t, 0, w.fake.calls(t))
	assert.Equal(t, "base_unavailable", w.ledger.prep().Code)
	require.Len(t, w.reports, 1)
	assert.Equal(t, "failed:base_unavailable", w.reports[0].Status)
	bs := w.bs(t, outcome.BSID)
	assert.Contains(t, bs, "Local patch (3σ, local only, if produced): branch autopus/band/"+lpKey)
	assert.NotContains(t, bs, "Diagnosis model:")
	assert.Empty(t, w.worktrees(t))
}

// S3: without durable decision and claim records (a nil executor), no
// diagnosis creates a worktree or starts a provider, and no record follows.
func TestReactBandDiagnoseLocalPatch_RecordsUnavailableRunsNoGitOrProvider(t *testing.T) {
	w := newDLPWorld(t, nil)
	cacheDir := t.TempDir()
	d := newBandDiagnoser(w.repo, lpS13Harness(), false, nil)
	d.now = func() time.Time { return lpT0 }
	d.bsOptions = brainstorm.Options{CacheDir: func() (string, error) { return cacheDir, nil }}
	var reports []healthband.LocalPatchRecord
	after := d.enableLocalPatch(nil, []healthband.LocalPatchRecord{dlpPatchClaim()}, func(r healthband.LocalPatchRecord) { reports = append(reports, r) })

	outcome := d.Run(context.Background(), dlpTierThree())
	after(context.Background(), dlpTierThree(), outcome, healthband.Recorded{})

	assert.Equal(t, "unavailable(worktree_unavailable)", outcome.DiagnosisStatus)
	assert.Equal(t, 0, w.fake.calls(t))
	assert.Empty(t, reports)
	assert.Empty(t, w.ledger.records())
	assert.NotContains(t, w.bs(t, outcome.BSID), "Local patch (3σ", "the claim is not durable, so the BS names no artifact")
	assert.Contains(t, w.bs(t, outcome.BSID), "\n**Providers**: claude\n")
}

// S15: a model refusal fallback during the diagnosis is recorded and the
// diagnosis proceeds; the patch request on the operator's model still ends done.
func TestReactBandDiagnoseLocalPatch_DiagnosisFallbackIsRecordedAndKept(t *testing.T) {
	w := newDLPWorld(t, nil, dlpPatchClaim())
	w.fake.answer(t, 1, lpStream(lpInit55, lpFallback48, lpAssistant48, lpResult(dlpDiagnosisText)))
	w.fake.answer(t, 2, lpStream(lpInit55, lpAssistant55, lpResult(lpReplyWith(lpFooDiff))))

	outcome := w.claim(t, dlpTierThree())

	assert.Equal(t, bandDiagnosisOK, outcome.DiagnosisStatus)
	assert.Contains(t, w.bs(t, outcome.BSID), "\n\nDiagnosis model: requested claude-opus-5-5, actual claude-opus-4-8; "+
		"model_substituted (refusal category cyber).\n\n## Evolution Ideas\n")
	require.Len(t, w.reports, 1)
	assert.Equal(t, healthband.ClaimDone, w.reports[0].Status)
	assert.True(t, w.reports[0].ModelSubstituted)
	assert.Equal(t, localPatchModel{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-4-8",
		RefusalCategory: "cyber"}, w.reports[0].Models[0])
}

// S15, S3: a stream without a success result is 001's empty output, and a
// provider that outlives its timeout is provider_timeout; either ends the
// claim diagnosis_unavailable with the stream's model recorded.
func TestReactBandDiagnoseLocalPatch_EmptyOrTimedOutDiagnosis(t *testing.T) {
	cases := map[string]struct {
		stream string
		status string
	}{
		"no result event": {lpStream(lpInit55, lpAssistant55), "unavailable(provider_empty_output)"},
		"is_error result": {lpStream(lpInit55, `{"type":"result","subtype":"success","is_error":true,"result":"x"}`),
			"unavailable(provider_empty_output)"},
		"blank result": {lpStream(lpInit55, lpAssistant55, lpResult(" \n")), "unavailable(provider_empty_output)"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := newDLPWorld(t, nil, dlpPatchClaim())
			w.fake.answer(t, 0, tc.stream)
			outcome := w.claim(t, dlpTierThree())

			assert.Equal(t, tc.status, outcome.DiagnosisStatus)
			assert.Equal(t, 1, w.fake.calls(t))
			require.Len(t, w.reports, 1)
			assert.Equal(t, "failed:diagnosis_unavailable", w.reports[0].Status)
			assert.Equal(t, []localPatchModel{{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}},
				w.reports[0].Models)
			assert.Empty(t, w.worktrees(t))
		})
	}
	w := newDLPWorld(t, nil, dlpPatchClaim())
	calls := 0
	w.patcher.provider.run = func(context.Context, orchestra.OrchestraConfig, orchestra.ProviderConfig, string) (*orchestra.ProviderResponse, error) {
		calls++
		return &orchestra.ProviderResponse{TimedOut: true, Output: lpStream(lpInit55, lpAssistant55)}, nil
	}
	outcome := w.claim(t, dlpTierThree())
	assert.Equal(t, "unavailable(provider_timeout)", outcome.DiagnosisStatus)
	assert.Equal(t, 1, calls)
	require.Len(t, w.reports, 1)
	assert.Equal(t, "failed:diagnosis_unavailable", w.reports[0].Status)
	assert.Equal(t, []localPatchModel{{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}},
		w.reports[0].Models, "a partial stream still records its model")
	assert.Empty(t, w.worktrees(t))
}

// S3: a diagnose claim whose BS cannot be written ends its local_patch claim
// no_bs; the worktree goes through the Cleanup Rules.
func TestReactBandDiagnoseLocalPatch_BSLockTimeoutEndsNoBS(t *testing.T) {
	w := newDLPWorld(t, nil, dlpPatchClaim())
	w.fake.answer(t, 0, lpStream(lpInit55, lpAssistant55, lpResult(dlpDiagnosisText)))
	cacheDir := t.TempDir()
	w.diagnoser.bsOptions = brainstorm.Options{CacheDir: func() (string, error) { return cacheDir, nil }, LockWait: 50 * time.Millisecond}
	held, err := filelock.Acquire(context.Background(), filepath.Join(cacheDir, "autopus", "bs-band.lock"), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Unlock()) }()

	outcome := w.claim(t, dlpTierThree())

	assert.Equal(t, "failed:bs_lock_timeout", outcome.Status)
	assert.Empty(t, outcome.BSID)
	assert.Equal(t, 1, w.fake.calls(t), "at most one provider call")
	require.Len(t, w.reports, 1)
	assert.Equal(t, "failed:no_bs", w.reports[0].Status)
	assert.Empty(t, w.worktrees(t))
}

// S1: a diagnoser that enableLocalPatch never switched on is 001's, even
// with health_band.local_patch_provider set: project dir cwd, no
// --restricted, no record.
func TestReactBandDiagnoseLocalPatch_FlagOffIgnoresTheProviderKey(t *testing.T) {
	fakes := installBandFakeProviders(t, "claude")
	fixture := newBandDiagnoseFixture(t)
	harness := bandHarness("claude", "", map[string]config.ProviderEntry{"claude": config.DefaultClaudeProviderEntry()})
	harness.HealthBand.LocalPatchProvider = "claude"

	outcome := fixture.diagnoser(harness, nil).Run(context.Background(), fixture.claim)

	assert.Equal(t, bandDiagnosisOK, outcome.DiagnosisStatus)
	real, err := filepath.EvalSymlinks(fixture.projectDir)
	require.NoError(t, err)
	assert.Equal(t, real+"\n", fakes.record(t, "claude", "cwd"))
	argv := fakes.argv(t, "claude")
	assert.NotContains(t, argv, "--restricted")
	assert.NotContains(t, argv, "--verbose")
	assert.NoFileExists(t, filepath.Join(fixture.projectDir, ".autopus", "metrics", healthband.LocalPatchEventsFile))
}

// --no-agent runs no provider, so a flag-on diagnosis needs no worktree:
// 001's skipped(no_agent) BS and no record of SPEC-SIGMABAND-002.
func TestReactBandDiagnoseLocalPatch_NoAgentKeeps001(t *testing.T) {
	w := newDLPWorld(t, nil)
	w.diagnoser.noAgent = true

	outcome := w.claim(t, dlpClaim(lpDiagnoseClaimID, 2, lpT0.Add(990*time.Second)))

	assert.Equal(t, bandDiagnosisSkippedNoAgent, outcome.DiagnosisStatus)
	assert.Equal(t, 0, w.fake.calls(t))
	assert.Empty(t, w.ledger.records())
	assert.NotContains(t, w.bs(t, outcome.BSID), "Diagnosis model:")
}
