package tui_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

// runAccessible runs one wizard form in huh accessible mode over input and
// returns what it printed.
func runAccessible(t *testing.T, form *huh.Form, input string) string {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, form.WithAccessible(true).WithInput(strings.NewReader(input)).WithOutput(&out).Run())
	return out.String()
}

// Acceptance S3, wizard part: the REA Confirm defaults to Yes, Enter keeps
// it, n declines it, and its Korean title is printed once.
func TestReaStep_AccessibleAnswers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  bool
	}{
		{"\n", false},
		{"y\n", true},
	}
	for _, tc := range cases {
		result := tui.TestDefaultResult(tui.InitWizardOpts{Platforms: []string{"claude-code"}})
		require.NotNil(t, result.Rea)
		require.False(t, *result.Rea, "the REA step starts at No")

		out := runAccessible(t, tui.TestBuildReaStepFor(5, 6, result, "ko"), tc.input)

		assert.Equal(t, tc.want, *result.Rea, "input %q", tc.input)
		assert.Equal(t, 1, strings.Count(out, "REA 역공학 MCP"), out)
		assert.Contains(t, out, "[5/6] REA 역공학 MCP")
		assert.Contains(t, out, "[y/N]")
	}
}

// The step text is the spec.md Form Text: Korean for ko, English otherwise.
func TestReaStepText_FormText(t *testing.T) {
	t.Parallel()
	ko := tui.TestReaStepTextFor("ko")
	assert.Equal(t, "REA 역공학 MCP", ko.Title)
	assert.Equal(t, "Claude Code·Codex·Antigravity에 rea-agents@6.5.0을 프로젝트 범위로 등록합니다 (Node ^22.19.0 || ^24.11.0 || >=26.0.0)", ko.Description)

	for _, lang := range []string{"en", "ja", "zh", ""} {
		en := tui.TestReaStepTextFor(lang)
		assert.Equal(t, "REA reverse-engineering MCP", en.Title, lang)
		assert.Equal(t, "Registers rea-agents@6.5.0 for Claude Code, Codex, and Antigravity in this project (Node ^22.19.0 || ^24.11.0 || >=26.0.0)", en.Description, lang)
	}
}

// The REA step joins the wizard only while REA is undecided and a REA target
// platform is selected: --rea, --no-rea, or an existing decision presets it,
// and OMP or OpenCode alone never asks (REQ-REA-02, REQ-REA-09).
func TestBuildStepList_ReaStep(t *testing.T) {
	t.Parallel()
	on, off := true, false
	cases := []struct {
		name      string
		opts      tui.InitWizardOpts
		wantSteps int
	}{
		{"claude-code asks", tui.InitWizardOpts{Platforms: []string{"claude-code"}}, 6},
		{"codex asks", tui.InitWizardOpts{Platforms: []string{"codex"}}, 6},
		{"antigravity-cli asks", tui.InitWizardOpts{Platforms: []string{"antigravity-cli"}}, 6},
		{"a target among others asks", tui.InitWizardOpts{Platforms: []string{"omp", "codex", "opencode"}}, 6},
		{"--rea presets it", tui.InitWizardOpts{Platforms: []string{"claude-code"}, Rea: &on}, 5},
		{"--no-rea presets it", tui.InitWizardOpts{Platforms: []string{"claude-code"}, Rea: &off}, 5},
		{"opencode and omp only", tui.InitWizardOpts{Platforms: []string{"opencode", "omp"}}, 5},
		{"no platforms", tui.InitWizardOpts{}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Len(t, tui.TestBuildStepList(tc.opts), tc.wantSteps)
		})
	}
}

// The REA step follows the review gate and precedes the methodology step,
// and the step list renders it in the system locale.
func TestBuildStepList_ReaStepFollowsReviewGate(t *testing.T) {
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANGUAGE", "")

	opts := tui.InitWizardOpts{Platforms: []string{"claude-code", "omp"}}
	steps := tui.TestBuildStepList(opts)
	require.Len(t, steps, 6)
	result := tui.TestDefaultResult(opts)

	assert.Contains(t, runAccessible(t, steps[3](result), "\n"), "[4/6] Review Gate")
	out := runAccessible(t, steps[4](result), "n\n")
	assert.Contains(t, out, "[5/6] REA reverse-engineering MCP")
	assert.False(t, *result.Rea)
	assert.NotContains(t, runAccessible(t, steps[5](result), "\n"), "REA")
}

// Before any step runs the REA answer is the preset decision, else No, and
// it never aliases the preset.
func TestDefaultResult_Rea(t *testing.T) {
	t.Parallel()
	on, off := true, false
	assert.False(t, *tui.TestDefaultResult(tui.InitWizardOpts{}).Rea)
	assert.True(t, *tui.TestDefaultResult(tui.InitWizardOpts{Rea: &on}).Rea)
	preset := tui.TestDefaultResult(tui.InitWizardOpts{Rea: &off})
	assert.False(t, *preset.Rea)
	assert.NotSame(t, &off, preset.Rea)
}

// A result built without defaultResult still starts the REA step at No.
func TestReaStep_ZeroResultStartsAtNo(t *testing.T) {
	t.Parallel()
	result := &tui.InitWizardResult{}
	out := runAccessible(t, tui.TestBuildReaStepFor(1, 1, result, "en"), "\n")
	require.NotNil(t, result.Rea)
	assert.False(t, *result.Rea)
	assert.Contains(t, out, "[1/1] REA reverse-engineering MCP")
}

// The non-interactive wizard returns the same REA defaults without a form.
func TestRunInitWizard_AccessibleReaDefault(t *testing.T) {
	t.Parallel()
	on := true
	result, err := tui.RunInitWizard(tui.InitWizardOpts{Accessible: true, Platforms: []string{"claude-code"}})
	require.NoError(t, err)
	assert.False(t, *result.Rea)

	result, err = tui.RunInitWizard(tui.InitWizardOpts{Accessible: true, Platforms: []string{"claude-code"}, Rea: &on})
	require.NoError(t, err)
	assert.True(t, *result.Rea)
}
