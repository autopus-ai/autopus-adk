package cli

// SPEC-REVMODEL-001 T2 (REQ-RVM-02, 03, 04, 05, 09; acceptance S1, S2, S3(a),
// S4(b), S9(b), S17, S18): the review tab's pure planner. It takes the
// baseline and the working value and nothing else, so init's memory path,
// the update and quality file writer, and the live check candidate plan the
// same edit the same way.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// reviewR3Config is R3 in memory: three OMP-backed providers and a review
// gate that names all of them with judge claude.
func reviewR3Config() *config.HarnessConfig {
	cfg := composeFixtureConfig("claude-code", "codex", "omp")
	cfg.Orchestra.Judge = "claude"
	cfg.Orchestra.Providers = map[string]config.ProviderEntry{
		"claude": reviewOMPEntry("anthropic/claude-fable-5-1:max"),
		"codex":  reviewOMPEntry("openai-codex/gpt-6-astra:max"),
		"gemini": reviewOMPEntry("google-antigravity/gemini-3.1-pro:high"),
	}
	for name, command := range cfg.Orchestra.Commands {
		command.Providers = []string{"claude", "codex", "gemini"}
		cfg.Orchestra.Commands[name] = command
	}
	cfg.Spec.ReviewGate.Providers = []string{"claude", "codex", "gemini"}
	cfg.Spec.ReviewGate.Judge = "claude"
	return cfg
}

func reviewOMPEntry(model string) config.ProviderEntry {
	return config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: model, Subprocess: config.SubprocessProvConf{Timeout: 900}}
}

// reviewCATGChoices is the attested strict catalog CATG after normalization.
func reviewCATGChoices(t *testing.T) []ompCustomModelChoice {
	t.Helper()
	return reviewChoicesOf(t, reviewCatalogCATG)
}

// reviewPlanOf plans working with the attested lists the tab builds from
// choices: each provider's own catalog provider.
func reviewPlanOf(t *testing.T, baseline reviewBaseline, working reviewComposition, choices []ompCustomModelChoice) (reviewPlan, error) {
	t.Helper()
	prefixes := map[string]string{
		"claude": "anthropic/", "codex": "openai-codex/", "gemini": "google-antigravity/", "gem\x1b[2Jini": "google-antigravity/",
	}
	attested := newReviewAttestation()
	for _, name := range baseline.Visible {
		for _, choice := range choices {
			if strings.HasPrefix(choice.selector(), prefixes[name]) {
				attested.OMP[name] = append(attested.OMP[name], choice)
			}
		}
	}
	return planReviewComposition(baseline, working, attested, composeReviewTextKO)
}

func reviewOn(baseline reviewBaseline, names ...string) []string {
	on := map[string]bool{}
	for _, name := range names {
		on[name] = true
	}
	return baseline.participation(on)
}

// REQ-RVM-03: the baseline order without the providers turned off, then the
// providers turned on in sortedProviderKeys order; a listed name without an
// orchestra.providers entry is neither shown nor changed.
func TestReviewBaseline_ParticipationKeepsTheBaselineOrder(t *testing.T) {
	cfg := reviewR3Config()
	baseline := newReviewBaseline(cfg, nil)
	assert.Equal(t, []string{"claude", "codex", "gemini"}, baseline.Visible)
	assert.Equal(t, []string{"claude", "codex"}, reviewOn(baseline, "claude", "codex"))

	cfg.Spec.ReviewGate.Providers = []string{"codex", "opencode", "claude"}
	baseline = newReviewBaseline(cfg, nil)
	assert.Equal(t, []string{"opencode", "claude", "gemini"}, reviewOn(baseline, "claude", "gemini"))

	cfg.Spec.ReviewGate.Providers = []string{"claude"}
	baseline = newReviewBaseline(cfg, nil)
	assert.Equal(t, []string{"claude", "codex"}, reviewOn(baseline, "claude", "codex"), "S18(2)")
	assert.Equal(t, 1, baseline.participants(baseline.reviewComposition))
}

// S1: turning gemini off changes only the list, and the commands note
// follows the change line; the after table marks the changed cell.
func TestPlanReviewComposition_S1TurnOffChangesTheListOnly(t *testing.T) {
	baseline := newReviewBaseline(reviewR3Config(), nil)
	working := baseline.reviewComposition.clone()
	working.Providers = reviewOn(baseline, "claude", "codex")

	plan, err := reviewPlanOf(t, baseline, working, reviewCATGChoices(t))

	require.NoError(t, err)
	assert.True(t, plan.Changed)
	assert.Equal(t, []string{"spec.review_gate.providers [claude, codex, gemini] → [claude, codex]"}, plan.Changes)
	assert.Equal(t, []string{composeReviewTextKO.CommandsNote}, plan.Notes)
	assert.Equal(t, [][]string{
		{"claude", "켜짐", "anthropic/claude-fable-5-1:max", "judge"},
		{"codex", "켜짐", "openai-codex/gpt-6-astra:max", ""},
		{"gemini", "꺼짐 *", "google-antigravity/gemini-3.1-pro:high", ""},
	}, plan.Table.rows)
	assert.Equal(t, []string{"provider", "리뷰", "model", "judge"}, plan.Table.header)
}

// S2: model lines follow the provider order, and the model note follows.
func TestPlanReviewComposition_S2ModelLinesInProviderOrder(t *testing.T) {
	baseline := newReviewBaseline(reviewR3Config(), nil)
	working := baseline.reviewComposition.clone()
	working.Models["codex"] = "openai-codex/gpt-6-luna:high"
	working.Models["claude"] = "anthropic/claude-opus-5-5:max"

	plan, err := reviewPlanOf(t, baseline, working, reviewCATGChoices(t))

	require.NoError(t, err)
	assert.Equal(t, []string{
		"orchestra.providers.claude.model anthropic/claude-fable-5-1:max → anthropic/claude-opus-5-5:max",
		"orchestra.providers.codex.model openai-codex/gpt-6-astra:max → openai-codex/gpt-6-luna:high",
	}, plan.Changes)
	assert.Equal(t, []string{composeReviewTextKO.ModelNote}, plan.Notes)
	assert.Equal(t, "openai-codex/gpt-6-luna:high *", plan.Table.rows[1][2])
}

// S3(a), S3(c)·(d), S18: the count and judge errors.
func TestPlanReviewComposition_S3CountAndJudgeErrors(t *testing.T) {
	baseline := newReviewBaseline(reviewR3Config(), nil)
	cases := []struct {
		name    string
		on      []string
		judge   string
		wantErr string
	}{
		{"judge off", []string{"claude", "codex"}, "gemini", "judge gemini는 리뷰에 참여하지 않는 provider라 고를 수 없습니다"},
		{"one provider", []string{"claude"}, "claude", "리뷰에는 provider가 두 개 이상 필요합니다 (지금 1개)"},
		{"no provider", nil, "claude", "리뷰에는 provider가 두 개 이상 필요합니다 (지금 0개)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			working := baseline.reviewComposition.clone()
			working.Providers = reviewOn(baseline, tc.on...)
			working.Judge = tc.judge
			_, err := reviewPlanOf(t, baseline, working, reviewCATGChoices(t))
			require.Error(t, err)
			assert.Equal(t, tc.wantErr, err.Error())
		})
	}
}

// S4(b): a changed model is held against the attested family list; a model
// equal to the baseline is not, even when the catalog lacks it.
func TestPlanReviewComposition_S4AttestsOnlyChangedModels(t *testing.T) {
	baseline := newReviewBaseline(reviewR3Config(), nil)
	cases := []struct {
		name     string
		catalog  string
		provider string
		model    string
		wantErr  string
	}{
		{"unattested new model", reviewCatalogCATN, "gemini", "google-antigravity/gemini-3-pro:high",
			"gemini model google-antigravity/gemini-3-pro:high는 설치된 OMP catalog가 증명하지 않습니다"},
		{"baseline model off the catalog", reviewCatalogCATN, "gemini", "google-antigravity/gemini-3.1-pro:high", ""},
		{"unreported thinking", reviewCatalogCATG, "codex", "openai-codex/gpt-6-luna:xhigh",
			"codex thinking xhigh는 openai-codex/gpt-6-luna가 보고하지 않는 단계입니다"},
		{"another family", reviewCatalogCATG, "codex", "anthropic/claude-opus-5-5:max",
			"codex model anthropic/claude-opus-5-5:max는 설치된 OMP catalog가 증명하지 않습니다"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			working := baseline.reviewComposition.clone()
			working.Models[tc.provider] = tc.model
			plan, err := reviewPlanOf(t, baseline, working, reviewChoicesOf(t, tc.catalog))
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.False(t, plan.Changed)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.wantErr, err.Error())
		})
	}
}

// S9(b) and S3(b): judge lines name an empty judge 없음, and the after table
// moves the judge mark.
func TestPlanReviewComposition_JudgeLines(t *testing.T) {
	cfg := reviewR3Config()
	cfg.Spec.ReviewGate.Judge = ""
	baseline := newReviewBaseline(cfg, nil)
	working := baseline.reviewComposition.clone()
	working.Judge = "codex"

	plan, err := reviewPlanOf(t, baseline, working, reviewCATGChoices(t))

	require.NoError(t, err)
	assert.Equal(t, []string{"spec.review_gate.judge judge 없음 → codex"}, plan.Changes)
	assert.Empty(t, plan.Notes)
	assert.Equal(t, "judge *", plan.Table.rows[1][3])

	baseline = newReviewBaseline(reviewR3Config(), nil)
	working = baseline.reviewComposition.clone()
	working.Providers = reviewOn(baseline, "codex", "gemini")
	working.Judge = "codex"
	plan, err = reviewPlanOf(t, baseline, working, reviewCATGChoices(t))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"spec.review_gate.providers [claude, codex, gemini] → [codex, gemini]",
		"spec.review_gate.judge claude → codex",
	}, plan.Changes)
	assert.Equal(t, []string{"*", "judge *", ""}, []string{plan.Table.rows[0][3], plan.Table.rows[1][3], plan.Table.rows[2][3]})
}

// S17(2): a working value equal to the baseline is no change, whatever the
// passes in between were.
func TestPlanReviewComposition_EqualToTheBaselineIsNoChange(t *testing.T) {
	baseline := newReviewBaseline(reviewR3Config(), nil)
	working := baseline.reviewComposition.clone()
	working.Providers = reviewOn(baseline, "claude", "codex")
	working.Providers = reviewOn(baseline, "claude", "codex", "gemini")

	plan, err := reviewPlanOf(t, baseline, working, reviewCATGChoices(t))

	require.NoError(t, err)
	assert.False(t, plan.Changed)
	assert.Empty(t, plan.Changes)
}

// REQ-RVM-08: a CLI provider's model cell reads `<binary> <model> · <effort>`
// from the raw document. Table cells are terminal-safe already; change lines
// keep the raw names, as composeDriverPlan.Changes does, and the tab prints
// them through terminalSafe.
func TestPlanReviewComposition_CLIBackendCellsAndTerminalSafety(t *testing.T) {
	cfg := reviewR3Config()
	cfg.Orchestra.Providers["claude"] = config.DefaultClaudeProviderEntry()
	cfg.Orchestra.Providers["gem\x1b[2Jini"] = reviewOMPEntry("google-antigravity/gemini-3.8-flash:high")
	delete(cfg.Orchestra.Providers, "gemini")
	cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
	baseline := newReviewBaseline(cfg, nil)
	working := baseline.reviewComposition.clone()
	working.Providers = reviewOn(baseline, "claude", "codex", "gem\x1b[2Jini")

	plan, err := reviewPlanOf(t, baseline, working, reviewCATGChoices(t))

	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex", "gem\x1b[2Jini"}, baseline.Visible)
	assert.Equal(t, "claude claude-opus-5-5 · max", plan.Table.rows[0][2])
	assert.Equal(t, `gem\x1b[2Jini`, plan.Table.rows[2][0])
	assert.Equal(t, "켜짐 *", plan.Table.rows[2][1])
	require.Len(t, plan.Changes, 1)
	assert.True(t, strings.HasSuffix(plan.Changes[0], "[claude, codex, gem\x1b[2Jini]"), plan.Changes)
}
