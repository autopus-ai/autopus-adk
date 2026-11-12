package cli

// SPEC-JUDGEMODEL-001 S2, S4 (REQ-JM-RUN-01, RUN-03, RUN-04): a configured
// judge model reaches the judge request only, on every backend kind; the
// plan block and the receipt name it; another judge or no key changes
// nothing.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// withJudgeModel sets the review gate's judge, judge_model, and judge_effort.
func withJudgeModel(judge, model, effort string) func(*config.HarnessConfig) {
	return func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Judge, cfg.Spec.ReviewGate.JudgeModel, cfg.Spec.ReviewGate.JudgeEffort = judge, model, effort
	}
}

// runJudgeModelReview runs one review of fixture with opts and returns the
// recording backend and stderr.
func runJudgeModelReview(t *testing.T, fixture *readOnlyReviewFixture, opts specReviewOptions) (*recordingReviewBackend, string) {
	t.Helper()
	backend := fixture.useFakeBackend(t)
	useHermeticReadiness(t)
	stderr := captureSpecReviewStderr(t, func() {
		require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, opts))
	})
	return backend, stderr
}

func TestSpecReviewJudgeModel_JudgeRequestCarriesItsModel(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, withJudgeModel("claude", "claude-opus-5-5", "high"))

	backend, stderr := runJudgeModelReview(t, fixture, specReviewOptions{})

	reviewer, judge := backend.request(t, "claude", "reviewer"), backend.request(t, "claude", "judge")
	assert.Equal(t, withClaudeReadOnlySuffix("--print", "--model", "claude-fable-5-1", "--effort", "max", "--output-format", "json"), reviewer.Config.Args)
	assert.Equal(t, withClaudeReadOnlySuffix("--print", "--model", "claude-opus-5-5", "--effort", "high", "--output-format", "json"), judge.Config.Args)
	assert.Equal(t, "--tools=Read,Grep,Glob", judge.Config.Args[len(judge.Config.Args)-1], "the projection stays last")
	assert.Equal(t, orchestra.SandboxModeReadOnly, judge.Config.SandboxMode)
	assert.Contains(t, stderr, "  judge: claude (always) — spec.review_gate.judge\n"+
		"  judge model: claude-opus-5-5 · high — spec.review_gate.judge_model\n")
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.NotNil(t, receipt.Judge)
	assert.Equal(t, [3]string{"claude", "claude-opus-5-5", "high"}, [3]string{receipt.Judge.Provider, receipt.Judge.Model, receipt.Judge.Effort})
	assert.Equal(t, "claude", receipt.ProviderPolicy[len(receipt.ProviderPolicy)-1].Provider)
}

// Every backend kind: codex pinned so the quality profile does not resolve
// it again, agy with its id appended, and OMP with the judge selector.
func TestSpecReviewJudgeModel_AssemblyPerBackend(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
			withJudgeModel("codex", "gpt-6-luna", "high")(cfg)
			editProvider(cfg, "codex", func(e *config.ProviderEntry) { e.ModelPolicy = config.ProviderModelPolicyQuality })
		})
		backend, _ := runJudgeModelReview(t, fixture, specReviewOptions{})

		reviewer, judge := backend.request(t, "codex", "reviewer"), backend.request(t, "codex", "judge")
		assert.Equal(t, config.ProviderModelPolicyPinned, judge.Config.ModelPolicy)
		assert.True(t, reviewArgsHasRun(judge.Config.Args, "-m", "gpt-6-luna", "-c", `model_reasoning_effort="high"`), "%v", judge.Config.Args)
		assert.False(t, slices.Contains(reviewer.Config.Args, "gpt-6-luna"), "the reviewer keeps its own model: %v", reviewer.Config.Args)
		assert.Equal(t, config.ProviderModelPolicyQuality, reviewer.Config.ModelPolicy)
	})
	t.Run("agy", func(t *testing.T) {
		fixture := newReadOnlyReviewFixture(t, withJudgeModel("gemini", "gemini-3.1-pro-high", ""))
		backend, _ := runJudgeModelReview(t, fixture, specReviewOptions{})

		reviewer, judge := backend.request(t, "gemini", "reviewer"), backend.request(t, "gemini", "judge")
		assert.NotContains(t, reviewer.Config.Args, "--model")
		assert.Equal(t, append([]string{"--print", "", "--model", "gemini-3.1-pro-high"}, reviewer.Config.Args[2:]...), judge.Config.Args)
	})
}

// An OMP judge model in any accepted form — split, thinking inline, or
// inline with the same judge_effort — runs, prints, and records one
// effective model: the base selector and its thinking level once
// (REQ-JM-RUN-03, RUN-04).
func TestSpecReviewJudgeModel_OMPFormsNormalize(t *testing.T) {
	for _, form := range [][2]string{
		{"anthropic/claude-opus-5-5", "max"}, {"anthropic/claude-opus-5-5:max", ""}, {"anthropic/claude-opus-5-5:max", "max"},
	} {
		t.Run(form[0]+"+"+form[1], func(t *testing.T) {
			fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
				withJudgeModel("claude", form[0], form[1])(cfg)
				cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
				cfg.Orchestra.Providers["claude"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: "anthropic/claude-fable-5-1:max"}
			})
			installFakeOMP(t)
			installReviewJSONRecorders(t, "codex")
			backend, stderr := runJudgeModelReview(t, fixture, specReviewOptions{})

			assert.Equal(t, "anthropic/claude-fable-5-1:max", backend.request(t, "claude", "reviewer").Config.Model)
			judge := backend.request(t, "claude", "judge")
			assert.Equal(t, "anthropic/claude-opus-5-5:max", judge.Config.Model)
			assert.Equal(t, "anthropic", judge.Config.ModelFamily)
			assert.Contains(t, stderr, "  judge model: anthropic/claude-opus-5-5:max — spec.review_gate.judge_model\n")
			receipt := readSpecReviewReceipt(t, fixture.specDir)
			require.NotNil(t, receipt.Judge)
			assert.Equal(t, [2]string{"anthropic/claude-opus-5-5", "max"}, [2]string{receipt.Judge.Model, receipt.Judge.Effort})
		})
	}
}

// Without the key, or for another judge, the judge is the reviewer config as
// before, the plan says which, and the receipt names no judge model.
func TestSpecReviewJudgeModel_AbsentOrOtherJudgeKeepsTheReviewerConfig(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	backend, stderr := runJudgeModelReview(t, fixture, specReviewOptions{})
	assert.Equal(t, backend.request(t, "claude", "reviewer").Config.Args, backend.request(t, "claude", "judge").Config.Args)
	assert.NotContains(t, stderr, "judge model:")
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.NotNil(t, receipt.Judge)
	assert.Empty(t, receipt.Judge.Model+receipt.Judge.Effort)
	var raw struct {
		Judge map[string]any `json:"judge"`
	}
	data, err := os.ReadFile(filepath.Join(fixture.specDir, "review-receipt.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw.Judge, "model", "the receipt's judge keeps its keys without judge_model")
	assert.NotContains(t, raw.Judge, "effort")

	fixture = newReadOnlyReviewFixture(t, withJudgeModel("claude", "claude-opus-5-5", "high"))
	backend, stderr = runJudgeModelReview(t, fixture, specReviewOptions{judge: "codex", judgeSet: true})
	assert.Equal(t, backend.request(t, "codex", "reviewer").Config.Args, backend.request(t, "codex", "judge").Config.Args)
	assert.Contains(t, stderr, "  judge: codex (always) — --judge\n"+
		"  judge model: codex의 리뷰어 model — spec.review_gate.judge_model은 judge claude에만 씁니다\n")
	receipt = readSpecReviewReceipt(t, fixture.specDir)
	require.NotNil(t, receipt.Judge)
	assert.Equal(t, "codex", receipt.Judge.Provider)
	assert.Empty(t, receipt.Judge.Model+receipt.Judge.Effort)
	for _, request := range backend.requests {
		assert.False(t, strings.Contains(strings.Join(request.Config.Args, " "), "claude-opus-5-5"), "%s/%s", request.Provider, request.Role)
	}
}

// The receipt writer names the judge model only for an applied one, so any
// other receipt keeps its bytes.
func TestSpecReviewJudgeModel_ReceiptFields(t *testing.T) {
	t.Parallel()

	applied := &specReviewJudgeModel{Judge: "claude", Model: "claude-opus-5-5", Effort: "high", ReviewJudge: "claude"}
	other := &specReviewJudgeModel{Judge: "claude", Model: "claude-opus-5-5", Effort: "high", ReviewJudge: "codex"}
	for _, tc := range []struct {
		name     string
		evidence []specReviewRuntimeEvidence
		want     [2]string
	}{
		{"applied", []specReviewRuntimeEvidence{{JudgeModel: applied}}, [2]string{"claude-opus-5-5", "high"}},
		{"other judge", []specReviewRuntimeEvidence{{JudgeModel: other}}, [2]string{}},
		{"none configured", []specReviewRuntimeEvidence{{}}, [2]string{}},
		{"no evidence", nil, [2]string{}},
	} {
		judge := &specReviewJudgeReceipt{Provider: "claude"}
		recordSpecReviewJudgeModel(judge, tc.evidence)
		assert.Equal(t, tc.want, [2]string{judge.Model, judge.Effort}, tc.name)
	}
	recordSpecReviewJudgeModel(nil, []specReviewRuntimeEvidence{{JudgeModel: applied}})
}

// The plan line's text per backend, and none without a judge.
func TestSpecReviewJudgeModel_PlanLine(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	m := &specReviewJudgeModel{Judge: "claude", Model: "claude-haiku-5-5", ReviewJudge: "claude"}
	printSpecReviewJudgeModel(&out, specReviewProviderPlan{Judge: &orchestra.ProviderConfig{Name: "claude"}, JudgeModel: m})
	printSpecReviewJudgeModel(&out, specReviewProviderPlan{JudgeModel: m})
	printSpecReviewJudgeModel(&out, specReviewProviderPlan{Judge: &orchestra.ProviderConfig{Name: "claude"}})
	assert.Equal(t, "  judge model: claude-haiku-5-5 — spec.review_gate.judge_model\n", out.String())
	assert.Equal(t, "anthropic/claude-opus-5-5:max", judgeModelText("anthropic/claude-opus-5-5", "max", true))
	assert.Equal(t, "claude-opus-5-5 · max", judgeModelText("claude-opus-5-5", "max", false))
	for _, effort := range []string{"", "max"} {
		assert.Equal(t, "anthropic/claude-opus-5-5:max", judgeModelText("anthropic/claude-opus-5-5:max", effort, true), "thinking once")
		model, thinking := specReviewJudgeModel{Model: "anthropic/claude-opus-5-5:max", Effort: effort, OMP: true}.effective()
		assert.Equal(t, [2]string{"anthropic/claude-opus-5-5", "max"}, [2]string{model, thinking})
	}
	assert.Equal(t, "anthropic/claude-opus-5-5:max:high", judgeModelText("anthropic/claude-opus-5-5:max", "high", true),
		"a contradiction the review refuses stays visible")
	model, effort := specReviewJudgeModel{Model: "claude-opus-5-5", Effort: "high"}.effective()
	assert.Equal(t, [2]string{"claude-opus-5-5", "high"}, [2]string{model, effort}, "a CLI judge model is recorded as configured")
}
