package cli

// SPEC-MODELUX-001 T11 (acceptance S11): the third quality choice inside the
// real `auto init` flow. Only the wizard result, the TTY and CI seams, the
// run's catalog probe, and the codex CLI are stand-ins; the init command, the
// driver, the save, and the platform adapters run for real.

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeInitStatus is the preset status line over a fresh init's config
// whose quality.default is the given preset.
func composeInitStatus(preset string) string {
	return "quality.default = " + preset + "   ultra: 현재 기본값   balanced: 현재 기본값"
}

// composeInitCustom is the custom preset S11 saves: the balanced default
// with the implementation group set to sonnet, written out from the SPEC so
// it cannot drift along with the code.
var composeInitCustom = map[string]string{
	"architect": "fable", "planner": "fable", "reviewer": "fable", "security-auditor": "fable", "spec-writer": "fable",
	"debugger": "sonnet", "deep-worker": "sonnet", "devops": "sonnet", "executor": "sonnet",
	"frontend-specialist": "sonnet", "perf-engineer": "sonnet", "tester": "sonnet", "ux-validator": "sonnet",
	"validator": "sonnet", "explorer": "sonnet", "annotator": "sonnet",
}

// stubInitWizard replaces the huh wizard with one that answers choice in the
// quality step and the wizard defaults elsewhere. Like the real wizard, it
// skips the quality step when --quality preset it. It records every call.
func stubInitWizard(t *testing.T, choice string) *[]tui.InitWizardOpts {
	t.Helper()
	original := runInitWizard
	t.Cleanup(func() { runInitWizard = original })
	calls := &[]tui.InitWizardOpts{}
	runInitWizard = func(opts tui.InitWizardOpts) (*tui.InitWizardResult, error) {
		*calls = append(*calls, opts)
		quality := choice
		if opts.Quality != "" {
			quality = opts.Quality
		}
		return &tui.InitWizardResult{
			CommentsLang: "en", CommitsLang: "ko", AILang: "ko",
			Quality: quality, Methodology: "tdd", UsageProfile: "developer",
		}, nil
	}
	return calls
}

// runComposeInit executes the real root command `auto init` with in as its
// input, for Claude Code and Codex in a fresh temp project.
func runComposeInit(t *testing.T, in io.Reader, args ...string) (string, string, error) {
	t.Helper()
	dir := t.TempDir()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(in)
	root.SetArgs(append([]string{"init", "--dir", dir, "--project", "p", "--platforms", "claude-code,codex"}, args...))
	err := root.Execute()
	return dir, out.String(), err
}

// composeInitDefaultQuality is the quality block config.Save writes for the
// code defaults, decoded the way rawQualityOf reads any written file.
func composeInitDefaultQuality(t *testing.T) config.QualityConf {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, config.DefaultFullConfig("p")))
	return rawQualityOf(t, readBlockFixture(t, dir))
}

// S11, first run: the third choice opens the shared editor from the balanced
// default; the confirmed composition is in the autopus.yaml init writes, and
// the same run generates exactly the approved preview from the one catalog
// snapshot the table read.
func TestInitCompose_RoleGroupsConfirmSavesCustomAndGeneratesThePreview(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	wizard := stubInitWizard(t, tui.QualityRoleGroups)

	dir, out, err := runComposeInit(t, strings.NewReader("g\n\n3\n\n\ny\n"))

	require.NoError(t, err, out)
	assert.Len(t, *wizard, 1)
	assert.Contains(t, out, "구성을 바꿀까요? [Enter] 유지 · g) 역할 그룹 · a) 고급\n")
	assert.Contains(t, out, "구성을 적용했습니다.\n")
	assert.Len(t, *run.probes, 1)
	assert.False(t, run.adapterProbed(t), "generation must read the run's snapshot")

	start := composeRenderedTable(t, out, composeInitStatus("balanced"))
	assert.Equal(t, "fable-5-1 · max", start["debugger"]["Claude Code"])
	written := rawQualityOf(t, readBlockFixture(t, dir))
	assert.Equal(t, "custom", written.Default)
	assert.Equal(t, config.QualityPreset{Description: "역할 그룹 구성", Agents: composeInitCustom}, written.Presets["custom"])
	defaults := composeInitDefaultQuality(t)
	assert.Equal(t, defaults.Presets["balanced"], written.Presets["balanced"])
	assert.Equal(t, defaults.Presets["ultra"], written.Presets["ultra"])

	// custom gives up the balanced standard placement, so every Claude Code
	// cell leaves its exact model ID; only debugger and deep-worker change
	// their Codex rung.
	preview := composeRenderedTable(t, out, "미리보기 (아직 바뀌지 않음)")
	claudeMarks, codexMarks := assertGeneratedEqualsTable(t, dir, preview)
	assert.Equal(t, 16, claudeMarks)
	assert.Equal(t, 2, codexMarks)
	assert.Equal(t, composeTableClaudeCell{model: "sonnet", effort: "medium"}, readComposeTableClaudeProfile(t, dir, "debugger"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, readComposeCodexProfile(t, dir, "debugger"))
}

// S11, second run and every other keep path: init goes on with the balanced
// default, writes the code-default quality block, and generates the table it
// drew.
func TestInitCompose_RoleGroupsKeepPathsProceedWithBalanced(t *testing.T) {
	cases := []struct{ name, input, says string }{
		{"cancel at the confirmation", "g\n\n3\n\n\nn\n", "취소했습니다. 설정을 바꾸지 않았습니다.\n"},
		{"enter at the question", "\n", ""},
		{"input end", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := newComposeUpdateRun(t, true, "")
			stubInitWizard(t, tui.QualityRoleGroups)

			dir, out, err := runComposeInit(t, strings.NewReader(tc.input))

			require.NoError(t, err, out)
			assert.Contains(t, out, tc.says)
			assert.NotContains(t, out, "구성을 적용했습니다.")
			assert.Len(t, *run.probes, 1)
			assert.False(t, run.adapterProbed(t), "generation must read the run's snapshot")
			assert.Equal(t, composeInitDefaultQuality(t), rawQualityOf(t, readBlockFixture(t, dir)))
			claudeMarks, codexMarks := assertGeneratedEqualsTable(t, dir, composeRenderedTable(t, out, composeInitStatus("balanced")))
			assert.Zero(t, claudeMarks+codexMarks)
			assert.Equal(t, composeTableClaudeCell{model: "claude-fable-5-1", effort: "max"}, readComposeTableClaudeProfile(t, dir, "planner"))
		})
	}
}

// balanced and ultra show the table of that choice read-only: no question,
// no stdin read, and the same run generates that table.
func TestInitCompose_BalancedOrUltraShowsTheTableReadOnly(t *testing.T) {
	for _, choice := range []string{"balanced", "ultra"} {
		t.Run(choice, func(t *testing.T) {
			run := newComposeUpdateRun(t, true, "")
			wizard := stubInitWizard(t, choice)

			dir, out, err := runComposeInit(t, driverFailingReader{t})

			require.NoError(t, err, out)
			assert.Len(t, *wizard, 1)
			assert.Contains(t, out, "에이전트 모델 구성")
			assert.NotContains(t, out, "구성을 바꿀까요?")
			assert.Len(t, *run.probes, 1)
			assert.False(t, run.adapterProbed(t), "generation must read the run's snapshot")
			want := composeInitDefaultQuality(t)
			want.Default = choice
			assert.Equal(t, want, rawQualityOf(t, readBlockFixture(t, dir)))
			claudeMarks, codexMarks := assertGeneratedEqualsTable(t, dir, composeRenderedTable(t, out, composeInitStatus(choice)))
			assert.Zero(t, claudeMarks+codexMarks)
		})
	}
}

// S11, --yes and --quality runs, plus a non-terminal stdin: unchanged init.
// None draws the table, reads stdin for it, or probes the catalog for it; the
// non-interactive ones never call the wizard. A --quality run in a terminal
// still runs the wizard, which skips its quality step, and no table follows.
func TestInitCompose_YesQualityAndNonTerminalRunsAreUnchanged(t *testing.T) {
	cases := []struct {
		name        string
		tty         bool
		args        []string
		wizardCalls int
		wantDefault string
	}{
		{"--yes", true, []string{"--yes"}, 0, "balanced"},
		{"--quality ultra --yes", true, []string{"--quality", "ultra", "--yes"}, 0, "ultra"},
		{"not a terminal", false, nil, 0, "balanced"},
		{"--quality ultra in a terminal", true, []string{"--quality", "ultra"}, 1, "ultra"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := newComposeUpdateRun(t, tc.tty, "")
			wizard := stubInitWizard(t, tui.QualityRoleGroups)

			dir, out, err := runComposeInit(t, driverFailingReader{t}, tc.args...)

			require.NoError(t, err, out)
			assert.Len(t, *wizard, tc.wizardCalls)
			assert.NotContains(t, out, "에이전트 모델 구성")
			assert.Empty(t, *run.probes)
			want := composeInitDefaultQuality(t)
			want.Default = tc.wantDefault
			assert.Equal(t, want, rawQualityOf(t, readBlockFixture(t, dir)))
		})
	}
}
