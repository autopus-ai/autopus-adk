package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// bandOMPClaude is an OMP-backed claude entry as this module configures it.
func bandOMPClaude(tools ...string) config.ProviderEntry {
	return config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: "anthropic/claude-opus-5-5:max", Tools: tools}
}

// bandWithoutToolsProjection is the shared projection as it was before
// SPEC-REVIEWRO-001: claude keeps no --tools item.
func bandWithoutToolsProjection(providers []orchestra.ProviderConfig, opts readOnlyPolicyOptions) ([]orchestra.ProviderConfig, error) {
	projected, err := applyReadOnlyProviderPolicy(providers, opts)
	for index := range projected {
		projected[index].Args = removeArgFlag(projected[index].Args, "--tools")
	}
	return projected, err
}

// S12: one provider case at a time; every case makes at most one provider
// call, tries no second provider, and writes exactly one BS whose provider
// section holds the fenced diagnosis or the diagnosis_status line.
func TestReactBandDiagnose_S12ProviderReadOnlyContract(t *testing.T) {
	claude := map[string]config.ProviderEntry{"claude": config.DefaultClaudeProviderEntry()}
	cases := []struct {
		name     string
		harness  *config.HarnessConfig
		bins     []string // fake binaries on PATH; nil installs all
		mode     string
		edit     func(*bandDiagnoser, *bandFakeBackend)
		status   string
		calls    []string
		provider string // BS providers line
		attempt  bool   // a run was attempted, so the prompt manifest is recorded
	}{
		{name: "claude projection with tools", harness: bandHarness("claude", "", claude), status: "ok", calls: []string{"claude"}, provider: "claude", attempt: true},
		{name: "claude projection without tools", harness: bandHarness("claude", "", claude),
			edit:   func(d *bandDiagnoser, _ *bandFakeBackend) { d.project = bandWithoutToolsProjection },
			status: "unavailable(provider_policy_incomplete)", provider: "claude"},
		{name: "codex", harness: bandHarness("codex", "", map[string]config.ProviderEntry{"codex": config.CodexProviderEntryForQuality(config.QualityConf{})}),
			status: "ok", calls: []string{"codex"}, provider: "codex", attempt: true},
		{name: "gemini agy", harness: bandHarness("gemini", "", map[string]config.ProviderEntry{
			"gemini": {Binary: "agy", Args: []string{"--print", ""}, PromptViaArgs: true}}), status: "ok", calls: []string{"agy"}, provider: "gemini", attempt: true},
		{name: "OMP-backed claude", harness: bandHarness("claude", "", map[string]config.ProviderEntry{"claude": bandOMPClaude()}),
			status: "ok", provider: "claude", attempt: true},
		{name: "OMP tools not exactly read grep glob", harness: bandHarness("claude", "", map[string]config.ProviderEntry{"claude": bandOMPClaude("glob", "grep")}),
			status: "unavailable(provider_policy_incomplete)", provider: "claude"},
		{name: "OMP route not registered", harness: bandHarness("claude", "", map[string]config.ProviderEntry{"claude": bandOMPClaude()}),
			edit: func(d *bandDiagnoser, _ *bandFakeBackend) {
				d.backends = func(orchestra.OrchestraConfig) map[string]orchestra.ExecutionBackend { return nil }
			}, status: "unavailable(provider_backend_unavailable)", provider: "claude", attempt: true},
		{name: "opencode", harness: bandHarness("opencode", "", map[string]config.ProviderEntry{"opencode": {Binary: "opencode", Args: []string{"run"}}}),
			status: "unavailable(provider_unsupported)", provider: "opencode"},
		{name: "dangerous args", harness: bandHarness("claude", "", map[string]config.ProviderEntry{
			"claude": {Binary: "claude", Args: []string{"--print", "--dangerously-skip-permissions"}}}),
			status: "unavailable(provider_policy_rejected)", provider: "claude"},
		{name: "binary not on PATH", harness: bandHarness("claude", "", claude), bins: []string{"codex", "omp"},
			status: "unavailable(provider_missing)", provider: "claude", attempt: true},
		{name: "sleep past the timeout", harness: bandHarness("claude", "", claude), mode: "sleep",
			edit:   func(d *bandDiagnoser, _ *bandFakeBackend) { d.timeout = time.Second },
			status: "unavailable(provider_timeout)", calls: []string{"claude"}, provider: "claude", attempt: true},
		{name: "exit 3", harness: bandHarness("claude", "", claude), mode: "exit3",
			status: "unavailable(provider_exit_nonzero)", calls: []string{"claude"}, provider: "claude", attempt: true},
		{name: "whitespace-only stdout", harness: bandHarness("claude", "", claude), mode: "blank",
			status: "unavailable(provider_empty_output)", calls: []string{"claude"}, provider: "claude", attempt: true},
		{name: "no agent", harness: bandHarness("claude", "", claude),
			edit:   func(d *bandDiagnoser, _ *bandFakeBackend) { d.noAgent = true },
			status: "skipped(no_agent)", provider: "none"},
		{name: "no provider configured", harness: bandHarness("", "", nil), status: "unavailable(provider_unconfigured)", provider: "none"},
		{name: "selected provider not configured", harness: bandHarness("claude", "", map[string]config.ProviderEntry{"codex": {Binary: "codex"}}),
			status: "unavailable(provider_unconfigured)", provider: "claude"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakes := installBandFakeProviders(t, tc.bins...)
			if tc.mode != "" {
				t.Setenv("BAND_FAKE_MODE", tc.mode)
			}
			fixture := newBandDiagnoseFixture(t)
			backend := &bandFakeBackend{response: orchestra.ProviderResponse{Output: "### Summary\nThe flaky step failed.\n"}}
			d := fixture.diagnoser(tc.harness, nil)
			d.backends = func(orchestra.OrchestraConfig) map[string]orchestra.ExecutionBackend {
				return map[string]orchestra.ExecutionBackend{config.ProviderBackendOMP: backend}
			}
			if tc.edit != nil {
				tc.edit(d, backend)
			}

			outcome := d.Run(context.Background(), fixture.claim)

			assert.Equal(t, tc.status, outcome.DiagnosisStatus)
			assert.Empty(t, outcome.Status, "an unavailable provider never fails the claim")
			assert.Equal(t, "written", outcome.BSStatus)
			assert.Equal(t, tc.calls, fakes.calls(t), "raw subprocess runs")
			providerCalls := len(fakes.calls(t)) + len(backend.calls())
			assert.LessOrEqual(t, providerCalls, 1, "at most one provider call, no second provider")
			require.Equal(t, []string{"BS-BAND-001.md"}, fixture.bsFiles(t))
			bs := fixture.bs(t, "BS-BAND-001")
			assert.Contains(t, bs, "diagnosis_status: "+tc.status+"\n")
			assert.Contains(t, bs, "**Providers**: "+tc.provider+"\n")
			fenced := strings.Contains(bs, healthband.Fence("### Summary\nThe flaky step failed."))
			assert.Equal(t, tc.status == "ok", fenced, "only an ok diagnosis is fenced into the BS")
			assert.Equal(t, tc.attempt, len(outcome.PromptManifest) > 0, "an attempted run records the manifest of its prompt")
		})
	}
}

// S12 codex row and the OMP row in detail: codex keeps the projected
// read-only argv; an OMP-backed provider reaches only its routed backend,
// with the read/grep/glob tool set, read-only, in the project dir.
func TestReactBandDiagnose_CodexArgvAndOMPRouteCarryTheReadOnlyControls(t *testing.T) {
	fakes := installBandFakeProviders(t)
	codexFixture := newBandDiagnoseFixture(t)
	codex := bandHarness("codex", "", map[string]config.ProviderEntry{"codex": config.CodexProviderEntryForQuality(config.QualityConf{})})

	require.Equal(t, "ok", codexFixture.diagnoser(codex, nil).Run(context.Background(), codexFixture.claim).DiagnosisStatus)

	argv := fakes.argv(t, "codex")
	assert.Equal(t, "exec", argv[0])
	assert.Contains(t, strings.Join(argv, " "), "--sandbox read-only")
	assert.Subset(t, argv, []string{"--ephemeral", "--ignore-user-config", "--ignore-rules"})
	assert.NotContains(t, argv, "workspace-write")

	ompFixture := newBandDiagnoseFixture(t)
	backend := &bandFakeBackend{response: orchestra.ProviderResponse{Output: "routed diagnosis"}}
	d := ompFixture.diagnoser(bandHarness("claude", "", map[string]config.ProviderEntry{"claude": bandOMPClaude()}), nil)
	d.backends = func(cfg orchestra.OrchestraConfig) map[string]orchestra.ExecutionBackend {
		assert.Equal(t, ompFixture.projectDir, cfg.WorkingDir)
		assert.True(t, cfg.ReadOnly)
		return map[string]orchestra.ExecutionBackend{config.ProviderBackendOMP: backend}
	}

	require.Equal(t, "ok", d.Run(context.Background(), ompFixture.claim).DiagnosisStatus)

	assert.Equal(t, []string{"codex"}, fakes.calls(t), "the OMP provider never reached a raw subprocess")
	require.Len(t, backend.calls(), 1)
	request := backend.calls()[0]
	assert.Equal(t, []string{"glob", "grep", "read"}, request.Config.Tools)
	assert.Equal(t, orchestra.SandboxModeReadOnly, request.Config.SandboxMode)
	assert.Equal(t, ompFixture.projectDir, request.Config.WorkDir)
	assert.Equal(t, healthband.ProviderTimeout, request.Timeout)
	assert.Equal(t, "anthropic/claude-opus-5-5:max", request.Config.Model)
	assert.NotContains(t, request.Prompt, "BS-BAND")
}
