package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func sandboxModesByName(providers []orchestra.ProviderConfig) map[string]string {
	modes := make(map[string]string, len(providers))
	for _, provider := range providers {
		modes[provider.Name] = provider.SandboxMode
	}
	return modes
}

func receiptSandboxModes(t *testing.T, specDir string) map[string]string {
	t.Helper()
	modes := map[string]string{}
	for _, row := range readSpecReviewReceipt(t, specDir).ProviderPolicy {
		modes[row.Provider+"/"+row.Role] = row.SandboxMode
	}
	return modes
}

// Security M1: agy's read-only flags have no live evidence until RFP-3
// passes, so spec review assembles a native agy reviewer or judge as
// unverified, while claude, codex, and an OMP-backed gemini stay read-only.
func TestAssembleSpecReviewProviders_RecordsAgyUnverified(t *testing.T) {
	installReadOnlyArgvRecorders(t, "claude", "codex", "agy", "omp")
	countCodexCatalogProbes(t)
	cfg := fDefaultSpecReviewConfig()

	set, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"claude": "read-only", "codex": "read-only", "gemini": "unverified"},
		sandboxModesByName(set.Providers))
	// The judge reuses the gemini reviewer, or resolves it separately.
	for _, reviewers := range [][]orchestra.ProviderConfig{set.Providers, set.Providers[:1]} {
		judge, err := assembleSpecReviewJudge(context.Background(), cfg, reviewers, "gemini", 0)
		require.NoError(t, err)
		require.NotNil(t, judge)
		assert.Equal(t, "unverified", judge.SandboxMode, "judge with %d reviewers", len(reviewers))
	}

	cfg.Orchestra.Providers["gemini"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: "google-antigravity/gemini-3.5-pro:high"}
	set, err = assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg, FlagProviders: []string{"gemini"}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"gemini": "read-only"}, sandboxModesByName(set.Providers),
		"the OMP review backend is read-only by construction")
}

// Security L1: a reviewer or judge that never started has no recorded launch
// and claims no sandbox; one that started and then failed reports the argv
// its backend recorded executing.
func TestRunSpecReview_ReceiptSandboxModeNeedsARecordedLaunch(t *testing.T) {
	startedThenFailed := func(argv ...string) *orchestra.ProviderResponse {
		return &orchestra.ProviderResponse{EmptyOutput: true, ExitCode: 1, Execution: &orchestra.ProviderExecution{Command: argv}}
	}
	tests := []struct {
		name    string
		replies map[string]recordedReviewReply
		want    map[string]string
	}{
		{
			name: "never started",
			replies: map[string]recordedReviewReply{
				"codex/reviewer": {err: errors.New("exec: codex: start failed")},
				"claude/judge":   {err: errors.New("exec: claude: start failed")},
			},
			want: map[string]string{
				"claude/reviewer": "read-only", "codex/reviewer": "", "gemini/reviewer": "unverified", "claude/judge": "",
			},
		},
		{
			name: "started then failed",
			replies: map[string]recordedReviewReply{
				"gemini/reviewer": {resp: startedThenFailed("agy", "--print", "Review", "--yolo")},
				"claude/judge":    {resp: startedThenFailed("claude", "--print", "--dangerously-skip-permissions")},
			},
			want: map[string]string{
				"claude/reviewer": "read-only", "codex/reviewer": "read-only", "gemini/reviewer": "unrestricted", "claude/judge": "unrestricted",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newReadOnlyReviewFixture(t, nil)
			backend := fixture.useFakeBackend(t)
			backend.replies = tt.replies
			useHermeticReadiness(t)

			require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))

			assert.Equal(t, tt.want, receiptSandboxModes(t, fixture.specDir))
		})
	}
}

// Security L1 at the row seam: a judge that ended in schema_error never
// launched, so its row stays empty, while an OMP-backed reviewer, whose
// backend records no process argv, keeps its read-only stamp.
func TestSpecReviewProviderPolicyRows_SandboxModeWithoutRecordedLaunch(t *testing.T) {
	t.Parallel()

	omp := orchestra.ProviderConfig{
		Name: "gemini", Backend: config.ProviderBackendOMP, Binary: config.ProviderBackendOMP, SandboxMode: orchestra.SandboxModeReadOnly,
	}
	claude := orchestra.ProviderConfig{
		Name: "claude", Binary: "claude", Args: []string{"--print", "--permission-mode", "plan"}, SandboxMode: orchestra.SandboxModeReadOnly,
	}
	preflight := &specReviewPreflight{
		Reviewers: []specReviewReadinessEntry{{Provider: omp, Readiness: unknownReadiness("probe_failed")}},
		Judge:     &specReviewReadinessEntry{Provider: claude, Readiness: unknownReadiness("probe_failed")},
	}
	result := &orchestra.OrchestraResult{
		Responses:       []orchestra.ProviderResponse{{Provider: "gemini", Output: "{}"}},
		FailedProviders: []orchestra.FailedProvider{{Name: "claude" + specReviewJudgeSuffix, Role: "judge", FailureClass: "schema_error"}},
	}

	assert.Equal(t, []specReviewProviderPolicyRow{
		{Provider: "gemini", Role: "reviewer", SandboxMode: "read-only", Readiness: "unknown(probe_failed)"},
		{Provider: "claude", Role: "judge", Readiness: "unknown(probe_failed)"},
	}, specReviewProviderPolicyRows(preflight, result))
}
