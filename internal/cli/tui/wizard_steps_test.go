package tui_test

import (
	"testing"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildStepList_StepCounts verifies step filtering based on flags.
func TestBuildStepList_StepCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      tui.InitWizardOpts
		wantSteps int
	}{
		{
			name:      "all steps — no flags",
			opts:      tui.InitWizardOpts{},
			wantSteps: 5, // profile + lang + quality + review-gate + methodology
		},
		{
			name:      "quality pre-set — skip quality step",
			opts:      tui.InitWizardOpts{Quality: "ultra"},
			wantSteps: 4, // profile + lang + review-gate + methodology
		},
		{
			name:      "no-review-gate — skip gate step",
			opts:      tui.InitWizardOpts{NoReviewGate: true},
			wantSteps: 4, // profile + lang + quality + methodology
		},
		{
			name:      "both flags — skip quality and gate",
			opts:      tui.InitWizardOpts{Quality: "ultra", NoReviewGate: true},
			wantSteps: 3, // profile + lang + methodology
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			steps := tui.TestBuildStepList(tc.opts)
			assert.Len(t, steps, tc.wantSteps)
		})
	}
}

// TestBuildSteps_ReturnForms verifies each step builder produces a non-nil form.
func TestBuildSteps_ReturnForms(t *testing.T) {
	t.Parallel()

	result := &tui.InitWizardResult{}

	assert.NotNil(t, tui.TestBuildLangStep(1, 4, result))
	assert.NotNil(t, tui.TestBuildQualityStep(2, 4, result, nil))
	assert.NotNil(t, tui.TestBuildMethodologyStep(4, 4, result))

	// Review gate with and without providers (covers both desc branches)
	assert.NotNil(t, tui.TestBuildReviewGateStep(3, 4, result,
		tui.InitWizardOpts{Providers: []string{"claude", "openai"}}))
	assert.NotNil(t, tui.TestBuildReviewGateStep(3, 4, result, tui.InitWizardOpts{}))
}

// TestBuildStepList_FormsCallable verifies all built steps produce runnable forms.
func TestBuildStepList_FormsCallable(t *testing.T) {
	t.Parallel()

	steps := tui.TestBuildStepList(tui.InitWizardOpts{})
	result := &tui.InitWizardResult{}
	for i, step := range steps {
		assert.NotNilf(t, step(result), "step %d should produce a non-nil form", i)
	}
}

// TestDefaultResult_UsageProfile verifies the default usage profile is "developer".
func TestDefaultResult_UsageProfile(t *testing.T) {
	t.Parallel()
	result := tui.TestDefaultResult(tui.InitWizardOpts{})
	assert.Equal(t, "developer", result.UsageProfile)
}

// TestDefaultResult_ExistingProfile verifies ExistingProfile propagates to result.
func TestDefaultResult_ExistingProfile(t *testing.T) {
	t.Parallel()
	result := tui.TestDefaultResult(tui.InitWizardOpts{ExistingProfile: "fullstack"})
	assert.Equal(t, "fullstack", result.UsageProfile)
}

// TestBuildStepList_IncludesProfileStep verifies the profile step is first when no ExistingProfile.
func TestBuildStepList_IncludesProfileStep(t *testing.T) {
	t.Parallel()
	steps := tui.TestBuildStepList(tui.InitWizardOpts{})
	require.NotEmpty(t, steps)
	result := &tui.InitWizardResult{}
	assert.NotNil(t, steps[0](result), "first step (profile) must return a non-nil form")

	skipped := tui.TestBuildStepList(tui.InitWizardOpts{ExistingProfile: "developer"})
	assert.Len(t, skipped, len(steps)-1, "ExistingProfile should skip the profile step")
}

// TestBuildProfileStep verifies buildProfileStep returns a valid huh.Form.
func TestBuildProfileStep(t *testing.T) {
	t.Parallel()
	result := &tui.InitWizardResult{}
	assert.NotNil(t, tui.TestBuildProfileStep(1, 5, result))
}

// TestQualityOptions_OfferRoleGroupsThird pins the quality step's choices
// (SPEC-MODELUX-001 REQ-MUX-15): ultra and balanced keep their values, and a
// third choice hands init to the role-group composition step. Its label
// follows the operator's locale: Korean for ko, English otherwise.
func TestQualityOptions_OfferRoleGroupsThird(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "role-groups", tui.QualityRoleGroups)

	labels := map[string]string{
		"ko": "고급 — 도구·에이전트별로 model을 직접 고릅니다",
		"en": "Advanced — choose models per tool and per agent",
		"ja": "Advanced — choose models per tool and per agent",
	}
	presets := map[string]string{
		"ultra":    "Ultra — reviewer, qa-explorer: fable · frontend-specialist, ux-validator: opus",
		"balanced": "Balanced — reviewer, qa-explorer: opus · frontend-specialist, ux-validator: sonnet",
	}
	for lang, label := range labels {
		options := tui.TestQualityOptions(lang, presets)
		require.Len(t, options, 3, lang)
		values := []string{options[0].Value, options[1].Value, options[2].Value}
		assert.Equal(t, []string{"ultra", "balanced", tui.QualityRoleGroups}, values, lang)
		assert.Equal(t, presets["ultra"], options[0].Key, lang)
		assert.Equal(t, presets["balanced"], options[1].Key, lang)
		assert.Equal(t, label, options[2].Key, lang)
	}
}

// Without labels from the caller, the quality step names the presets only:
// it never claims what a preset sets without reading it.
func TestQualityOptions_WithoutLabelsNamesThePresets(t *testing.T) {
	t.Parallel()
	options := tui.TestQualityOptions("en", nil)
	assert.Equal(t, "Ultra", options[0].Key)
	assert.Equal(t, "Balanced", options[1].Key)
}

// The review gate step says what it enables, multi-CLI review, and names
// the CLIs init detected; with none it says so instead of describing a human
// review gate.
func TestReviewGateDescription_NamesMultiCLIReviewAndDetectedCLIs(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		"Multi-CLI review: each detected review CLI reviews independently. Detected: claude, codex — enable?",
		tui.TestReviewGateDescription([]string{"claude", "codex"}))
	assert.Equal(t,
		"Multi-CLI review: each detected review CLI reviews independently. Detected: claude (2 or more needed) — enable anyway?",
		tui.TestReviewGateDescription([]string{"claude"}))
	assert.Equal(t,
		"Multi-CLI review: each detected review CLI reviews independently. No review CLI detected (claude, codex, agy) — enable anyway?",
		tui.TestReviewGateDescription(nil))
}
