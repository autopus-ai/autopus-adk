package cli

// SPEC-JUDGEMODEL-001 S3 (REQ-JM-RUN-02): a judge model the judge's backend
// cannot run fails the review while the judge is assembled, before the
// readiness preflight and before any provider request.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestSpecReviewJudgeModel_RefusedBeforeAnyCall(t *testing.T) {
	ompClaude := func(cfg *config.HarnessConfig) {
		cfg.Orchestra.Providers["claude"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: "anthropic/claude-fable-5-1:max"}
	}
	foreignJudge := func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Providers = []string{"codex", "gemini"}
		editProvider(cfg, "claude", func(e *config.ProviderEntry) { e.Binary = "foo" })
	}
	cases := []struct {
		name, judge, model, effort string
		edit                       func(*config.HarnessConfig)
		want                       string
	}{
		{"selector on CLI", "claude", "anthropic/claude-opus-5-5", "", nil,
			`spec review: spec.review_gate.judge_model "anthropic/claude-opus-5-5" is an OMP model selector, but judge claude runs on the CLI backend (claude)`},
		{"effort off the ladder", "claude", "claude-opus-5-5", "turbo", nil,
			`spec review: spec.review_gate.judge_effort "turbo" is not one of low, medium, high, xhigh, max`},
		{"bare model on OMP", "claude", "claude-opus-5-5", "max", ompClaude,
			`spec review: spec.review_gate.judge_model "claude-opus-5-5" is not a <provider>/<model> selector, which judge claude on the OMP backend needs`},
		{"another family on OMP", "claude", "openai-codex/gpt-6-astra", "high", ompClaude,
			`spec review: spec.review_gate.judge_model "openai-codex/gpt-6-astra" belongs to the openai family, but judge claude belongs to anthropic`},
		{"thinking twice", "claude", "anthropic/claude-opus-5-5:max", "high", ompClaude,
			`spec review: spec.review_gate.judge_model "anthropic/claude-opus-5-5:max" already names thinking max; spec.review_gate.judge_effort "high" contradicts it`},
		{"effort on agy", "gemini", "gemini-3.1-pro", "high", nil,
			`spec review: spec.review_gate.judge_effort "high" does not apply to judge gemini on agy: agy ids carry their effort, so name the id, for example gemini-3.1-pro-high`},
		{"agy id of another family", "gemini", "claude-opus-5-5", "", nil,
			`spec review: spec.review_gate.judge_model "claude-opus-5-5" is not an agy id of judge gemini's family google`},
		{"unknown binary", "claude", "m1", "", foreignJudge,
			"spec review: judge claude runs binary foo, whose model flag Autopus does not know, so spec.review_gate.judge_model cannot be applied"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
				if tc.edit != nil {
					tc.edit(cfg)
				}
				withJudgeModel(tc.judge, tc.model, tc.effort)(cfg)
			})
			backend := fixture.useFakeBackend(t)
			spy := useReadinessRunner(t, replyWith(0, `{"loggedIn":true}`, ""))

			err := runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{})

			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			assert.Zero(t, backend.total(), "no provider request")
			assert.Empty(t, spy.argvs(), "no readiness probe")
		})
	}
}

// The application itself, below the review: only the model and effort values
// change, and a provider family the name does not give follows the selector.
func TestSpecReviewJudgeModel_ApplyChangesOnlyModelValues(t *testing.T) {
	t.Parallel()

	claude := fDefaultReadOnlyProviders()[0]
	haiku, err := applySpecReviewJudgeModel(claude, specReviewJudgeModel{Model: "claude-haiku-4-5"})
	require.NoError(t, err)
	assert.Equal(t, []string{"--print", "--model", "claude-haiku-4-5"}, haiku.Args, "claude's empty effort drops --effort, as the tab writes haiku")
	assert.Equal(t, []string{"--print", "--model", "claude-fable-5-1", "--effort", "max"}, claude.Args, "the reviewer's slice is untouched")

	bare, err := applySpecReviewJudgeModel(orchestra.ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--print"}},
		specReviewJudgeModel{Model: "claude-opus-5-5", Effort: "xhigh"})
	require.NoError(t, err)
	assert.Equal(t, []string{"--print", "--model", "claude-opus-5-5", "--effort", "xhigh"}, bare.Args, "missing flags are appended as pairs")

	omp, err := applySpecReviewJudgeModel(orchestra.ProviderConfig{Name: "reviewer-x", Backend: config.ProviderBackendOMP, Model: "example/foo-1:high"},
		specReviewJudgeModel{Model: "openai-codex/gpt-6-astra:max"})
	require.NoError(t, err)
	assert.Equal(t, "openai-codex/gpt-6-astra:max", omp.Model)
	assert.Equal(t, "openai", omp.ModelFamily)

	_, err = applySpecReviewJudgeModel(orchestra.ProviderConfig{Name: "claude", Binary: "/opt/bin/claude", Args: []string{"--print"}},
		specReviewJudgeModel{Model: "claude-opus-5-5", Effort: "max"})
	require.NoError(t, err, "a claude binary given as a path keeps its row")
}
