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
	assert.NotNil(t, tui.TestBuildQualityStep(2, 4, result))
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
		"ko": "역할별 직접 구성 — 역할 그룹마다 등급을 고릅니다",
		"en": "Compose by role — choose a tier for each role group",
		"ja": "Compose by role — choose a tier for each role group",
	}
	for lang, label := range labels {
		options := tui.TestQualityOptions(lang)
		require.Len(t, options, 3, lang)
		values := []string{options[0].Value, options[1].Value, options[2].Value}
		assert.Equal(t, []string{"ultra", "balanced", tui.QualityRoleGroups}, values, lang)
		assert.Equal(t, "Ultra — strict review, high coverage", options[0].Key, lang)
		assert.Equal(t, "Balanced — pragmatic defaults", options[1].Key, lang)
		assert.Equal(t, label, options[2].Key, lang)
	}
}
