package cli

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Provider Read-Only Contract item 1: health_band.diagnosis_provider, else
// orchestra.judge, else the lexicographically first configured provider.
func TestReactBandDiagnose_SelectBandProviderFollowsTheContractOrder(t *testing.T) {
	t.Parallel()
	providers := map[string]config.ProviderEntry{"gemini": {}, "codex": {}, "claude": {}}
	cases := []struct {
		name    string
		harness *config.HarnessConfig
		want    string
	}{
		{"diagnosis provider first", bandHarness("claude", "codex", providers), "codex"},
		{"then the judge", bandHarness("gemini", "", providers), "gemini"},
		{"then the first name", bandHarness("", "", providers), "claude"},
		{"blank values do not count", bandHarness(" ", " ", map[string]config.ProviderEntry{"gemini": {}, "codex": {}}), "codex"},
		{"none configured", bandHarness("", "", nil), ""},
		{"no config", nil, ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, selectBandProvider(tc.harness), tc.name)
	}
}

// S12: with no judge and providers gemini and codex, every one of 20 runs
// selects codex (map order never leaks into the selection).
func TestReactBandDiagnose_NoJudgeSelectsCodexOnEachOf20Runs(t *testing.T) {
	fixture := newBandDiagnoseFixture(t)
	harness := bandHarness("", "", map[string]config.ProviderEntry{
		"gemini": {Binary: "agy", Args: []string{"--print", ""}, PromptViaArgs: true},
		"codex":  config.CodexProviderEntryForQuality(config.QualityConf{}),
	})
	d := fixture.diagnoser(harness, nil)
	var selected []string
	d.run = func(_ context.Context, _ orchestra.OrchestraConfig, provider orchestra.ProviderConfig, _ string) (*orchestra.ProviderResponse, error) {
		selected = append(selected, provider.Name)
		return &orchestra.ProviderResponse{Output: "diagnosis"}, nil
	}

	for range 20 {
		require.Equal(t, "ok", d.Run(context.Background(), fixture.claim).DiagnosisStatus)
	}

	assert.Equal(t, slices.Repeat([]string{"codex"}, 20), selected)
}

// S20 and REQ-18: health_band.diagnosis_provider overrides orchestra.judge
// and gets the same controls and the same unavailable path.
func TestReactBandDiagnose_DiagnosisProviderOverridesTheJudge(t *testing.T) {
	fakes := installBandFakeProviders(t)
	providers := map[string]config.ProviderEntry{
		"claude":   config.DefaultClaudeProviderEntry(),
		"codex":    config.CodexProviderEntryForQuality(config.QualityConf{}),
		"opencode": {Binary: "opencode", Args: []string{"run"}},
	}
	codexFixture := newBandDiagnoseFixture(t)

	outcome := codexFixture.diagnoser(bandHarness("claude", "codex", providers), nil).Run(context.Background(), codexFixture.claim)

	assert.Equal(t, "ok", outcome.DiagnosisStatus)
	assert.Equal(t, []string{"codex"}, fakes.calls(t))
	assert.Contains(t, strings.Join(fakes.argv(t, "codex"), " "), "--sandbox read-only")

	opencodeFixture := newBandDiagnoseFixture(t)
	outcome = opencodeFixture.diagnoser(bandHarness("claude", "opencode", providers), nil).Run(context.Background(), opencodeFixture.claim)

	assert.Equal(t, "unavailable(provider_unsupported)", outcome.DiagnosisStatus)
	assert.Equal(t, []string{"codex"}, fakes.calls(t), "no other provider is tried after the selected one")
	assert.Contains(t, opencodeFixture.bs(t, outcome.BSID), "diagnosis_status: unavailable(provider_unsupported)\n")
}

// BS Root Resolution item 4: a busy per-user allocation lock ends the claim
// failed:bs_lock_timeout, and the episode has no BS.
func TestReactBandDiagnose_BSLockTimeoutFailsTheClaim(t *testing.T) {
	t.Parallel()
	fixture := newBandDiagnoseFixture(t)
	held, err := filelock.Acquire(context.Background(), filepath.Join(fixture.cacheDir, "autopus", "bs-band.lock"), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Unlock()) }()
	d := fixture.diagnoser(bandHarness("claude", "", nil), nil)
	d.noAgent = true
	d.bsOptions.LockWait = 50 * time.Millisecond

	outcome := d.Run(context.Background(), fixture.claim)

	assert.Equal(t, healthband.ClaimOutcome{
		Status: "failed:bs_lock_timeout", DiagnosisStatus: "skipped(no_agent)", BSStatus: "bs_lock_timeout",
	}, outcome)
	assert.Empty(t, fixture.bsFiles(t))
}

// A claim whose prompt the layers refuse never reaches a provider and still
// writes its evidence-only BS; raw (unsanitized) evidence is refused by the
// BS writer too, so it fails the claim instead of reaching a file.
func TestReactBandDiagnose_RefusedPromptRunsNoProvider(t *testing.T) {
	t.Parallel()
	fixture := newBandDiagnoseFixture(t)
	harness := bandHarness("claude", "", map[string]config.ProviderEntry{"claude": config.DefaultClaudeProviderEntry()})
	d := fixture.diagnoser(harness, nil)
	d.run = func(context.Context, orchestra.OrchestraConfig, orchestra.ProviderConfig, string) (*orchestra.ProviderResponse, error) {
		t.Fatal("a refused prompt must not reach the provider")
		return nil, nil
	}
	claim := fixture.claim
	claim.Event.Seq = 0 // not a written evaluation event, so it has no snapshot layer

	outcome := d.Run(context.Background(), claim)

	assert.Equal(t, healthband.ClaimOutcome{DiagnosisStatus: "unavailable(prompt_invalid)", BSID: "BS-BAND-001", BSStatus: "written"}, outcome)
	assert.Contains(t, fixture.bs(t, "BS-BAND-001"), "diagnosis_status: unavailable(prompt_invalid)\n")

	raw := bandStaticEvidence{logs: []healthband.RunLog{{RunID: 4242, Attempt: 1, Evidence: healthband.Evidence{Text: "raw"}}}}
	rawDiagnoser := fixture.diagnoser(harness, raw)
	rawDiagnoser.run = d.run
	outcome = rawDiagnoser.Run(context.Background(), fixture.claim)

	assert.Equal(t, "unavailable(prompt_invalid)", outcome.DiagnosisStatus)
	assert.Equal(t, "failed:bs_invalid_request", outcome.Status)
	assert.Equal(t, []string{"BS-BAND-001.md"}, fixture.bsFiles(t))
}
