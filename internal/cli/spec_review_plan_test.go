package cli

// SPEC-REVIEWPOLICY-001 S7, S8 (R7): provider selection is separate from the
// review profile, the plan states its precedence, invalid combinations stop
// before anything runs, and the printed plan names exactly the calls that run.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func planTestDoc(class string) *spec.SpecDocument {
	doc := &spec.SpecDocument{ID: "SPEC-PLAN-001"}
	if class != "" {
		doc.Profile, doc.ChangeClass, doc.Surface = "lean", class, []string{"internal/cli/spec_timing_test.go"}
	}
	return doc
}

func TestResolveSpecReviewPlan_PrecedenceTable(t *testing.T) {
	gate := config.ReviewGateConf{Providers: []string{"claude", "codex", "gemini"}, Judge: "claude"}
	cases := []struct {
		name      string
		class     string
		opts      specReviewOptions
		multi     bool
		profile   string
		reviewers []string
		fallbacks []string
		judge     string
		quorum    int
		warning   string
	}{
		{"lean default", "test_only", specReviewOptions{}, false, "lean", []string{"claude"}, []string{"codex", "gemini"}, "", 1, ""},
		{"single provider override", "test_only", specReviewOptions{providers: []string{"codex"}}, false, "lean", []string{"codex"}, []string{}, "", 1, ""},
		{"multiple provider override", "test_only", specReviewOptions{providers: []string{"codex", "gemini"}}, false, "lean", []string{"codex"}, []string{"gemini"}, "", 1, "--providers codex,gemini"},
		{"explicit judge on lean", "test_only", specReviewOptions{judge: "codex", judgeSet: true}, false, "lean", []string{"claude"}, []string{"codex", "gemini"}, "codex", 1, "--judge codex"},
		{"multi on lean", "test_only", specReviewOptions{}, true, "lean", []string{"claude"}, []string{"codex", "gemini"}, "", 1, "--multi"},
		{"high-risk lean is full", "feature", specReviewOptions{}, false, "full", nil, nil, "claude", 0, "lean profile 조건"},
		{"raised profile", "test_only", specReviewOptions{profile: "full"}, false, "full", nil, nil, "claude", 0, "--profile full"},
		{"full with providers", "", specReviewOptions{providers: []string{"codex", "gemini"}}, false, "full", []string{"codex", "gemini"}, nil, "claude", 0, ""},
		{"full without judge", "", specReviewOptions{judge: "none", judgeSet: true}, false, "full", nil, nil, "", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := resolveSpecReviewPlan(planTestDoc(tc.class), gate, tc.opts, tc.multi, 0)
			require.NoError(t, err)
			assert.Equal(t, tc.profile, plan.Profile)
			assert.Equal(t, tc.reviewers, plan.Reviewers)
			if tc.fallbacks != nil {
				assert.Equal(t, tc.fallbacks, nilAsEmpty(plan.Fallbacks))
			}
			assert.Equal(t, tc.judge, plan.Judge)
			assert.Equal(t, tc.quorum, plan.Quorum)
			assert.Equal(t, 1, plan.MaxRounds)
			if tc.warning == "" {
				assert.Empty(t, plan.Warnings)
			} else {
				require.Len(t, plan.Warnings, 1)
				assert.Contains(t, plan.Warnings[0], tc.warning)
			}

			applied, opts, fallbacks := applySpecReviewPlan(plan, gate, tc.opts)
			assert.Equal(t, plan.Judge, applied.Judge)
			if plan.Profile == spec.ReviewProfileLean {
				assert.Equal(t, 1, applied.MinProviders)
				assert.Equal(t, plan.Reviewers, opts.providers)
				assert.Equal(t, plan.Fallbacks, fallbacks)
			}
		})
	}
}

func TestResolveSpecReviewPlan_RefusesInvalidCombinations(t *testing.T) {
	gate := config.ReviewGateConf{Providers: []string{"claude"}, Judge: "claude"}
	cases := map[string]struct {
		class string
		opts  specReviewOptions
		want  []string
	}{
		"unknown profile":        {"", specReviewOptions{profile: "quick"}, []string{"--profile", "auto, full, lean"}},
		"lean on a full SPEC":    {"", specReviewOptions{profile: "lean"}, []string{"--profile lean", "lean-eligible"}},
		"lean on a refused SPEC": {"feature", specReviewOptions{profile: "lean"}, []string{"--profile lean", "feature"}},
		"empty judge":            {"", specReviewOptions{judge: " ", judgeSet: true}, []string{"--judge"}},
		"extend and new session": {"", specReviewOptions{extendBudget: true, newSession: true}, []string{"--new-session", "--extend-budget"}},
		"explicit zero calls":    {"", specReviewOptions{maxCalls: 0, maxCallsSet: true}, []string{"--max-calls"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := resolveSpecReviewPlan(planTestDoc(tc.class), gate, tc.opts, false, 0)
			require.Error(t, err)
			for _, want := range tc.want {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func nilAsEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
