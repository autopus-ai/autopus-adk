package cli

// SPEC-MODELUX-001 T12 (REQ-MUX-16, REQ-MUX-19; acceptance S12, S13): the
// real `auto quality` command opens the composition step first, hands target
// names and `t` to the existing editors unchanged, keeps its TTY error, and
// `--apply` generates with the run's catalog snapshot. Only the TTY seam, the
// run's catalog probe (CAT-A), and the codex CLI are stand-ins.

import (
	"bytes"
	"crypto/sha256"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	qualityComposeStatusD   = "quality.default = balanced   ultra: 현재 기본값   balanced: 현재 기본값\n"
	qualityComposeQuestionD = "구성을 바꿀까요? [Enter] 유지 · g) 역할 그룹 · a) 고급 · t) 도구별 설정\n"
	qualityComposeTTYError  = "interactive quality selection requires a TTY; use an explicit quality or profile command"
)

// runQualityCompose runs the real `auto quality` over dir's autopus.yaml. A
// nil in leaves the process stdin as the command input.
func runQualityCompose(t *testing.T, dir string, in io.Reader, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if in != nil {
		root.SetIn(in)
	}
	root.SetArgs(append([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality"}, args...))
	err := root.Execute()
	return out.String(), err
}

// qualityBlockOuter splits a machine-written autopus.yaml around its
// top-level quality block.
func qualityBlockOuter(t *testing.T, data string) (string, string) {
	t.Helper()
	lines := strings.SplitAfter(data, "\n")
	start := slices.Index(lines, "quality:\n")
	require.GreaterOrEqual(t, start, 0, data)
	end := start + 1
	for end < len(lines) && strings.HasPrefix(lines[end], " ") {
		end++
	}
	return strings.Join(lines[:start], ""), strings.Join(lines[end:], "")
}

// Without arguments the composition step comes first: the GAP 1 status, the
// table drawn from one catalog probe, and the question with `t`. Enter keeps
// autopus.yaml byte-identical and regenerates nothing.
func TestQualityCompose_OpensTheCompositionStepFirst(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	dir, _ := composeSessionProject(t, "D")
	before := composeSessionHash(t, dir)

	out, err := runQualityCompose(t, dir, strings.NewReader("\n"), "--apply")

	require.NoError(t, err, out)
	assert.Contains(t, out, "에이전트 모델 구성")
	assert.Contains(t, out, qualityComposeStatusD+composeSessionGolden(t, "table-D-CAT-A.golden")+qualityComposeQuestionD)
	assert.Len(t, *run.probes, 1)
	assert.NotContains(t, out, "quality.applied_platforms", "a kept composition applies nothing")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// Target names, their menu numbers, and `t` reach the editors exactly as the
// target menu did, after the table. The per-tool write goes through the
// block writer and keeps every byte outside the quality block (S12).
func TestQualityCompose_RoutesTargetsToTheExistingEditors(t *testing.T) {
	cases := []struct {
		name, input, editor, wantDefault string
	}{
		{"global", "global\n1\n", "Select quality mode:", "ultra"},
		{"t opens the target menu", "t\nglobal\n1\n", "What would you like to configure?", "ultra"},
		{"menu number of global", "1\n1\n", "Select quality mode:", "ultra"},
		{"claude-code", "claude-code\nexecutor=fable\n\n\ny\n", "Claude Code — agent models", "balanced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newComposeUpdateRun(t, true, "")
			dir, _ := composeSessionProject(t, "D")
			original := readBlockFixture(t, dir)

			out, err := runQualityCompose(t, dir, strings.NewReader(tc.input))

			require.NoError(t, err, out)
			question := strings.Index(out, qualityComposeQuestionD)
			require.GreaterOrEqual(t, question, 0, out)
			assert.Greater(t, strings.Index(out, tc.editor), question, "the editor runs after the composition question")
			written := readBlockFixture(t, dir)
			quality := rawQualityOf(t, written)
			assert.Equal(t, tc.wantDefault, quality.Default)
			if tc.wantDefault == "ultra" {
				assert.Contains(t, out, "quality.default = ultra\n")
				return
			}
			assert.Equal(t, map[string]string{config.QualityProviderClaude: "claude-custom"}, quality.Providers)
			assert.Len(t, quality.Presets["claude-custom"].Agents, len(config.CanonicalAgentNames()))
			assert.Equal(t, "fable", quality.Presets["claude-custom"].Agents["executor"])
			beforePrefix, beforeSuffix := qualityBlockOuter(t, original)
			afterPrefix, afterSuffix := qualityBlockOuter(t, written)
			assert.Equal(t, beforePrefix, afterPrefix)
			assert.Equal(t, beforeSuffix, afterSuffix)
		})
	}
}

// An editor reached from the composition prompt regenerates with --apply
// from the snapshot the table read, so the run reads the Codex catalog once
// and the adapters never probe for themselves (REQ-MUX-19, review A4).
func TestQualityCompose_RoutedEditorsApplyWithTheRunSnapshot(t *testing.T) {
	for name, input := range map[string]string{
		"global":                  "global\n1\n",
		"t opens the target menu": "t\nglobal\n1\n",
		"claude-code tool editor": "claude-code\nexecutor=fable\n\n\ny\n",
	} {
		t.Run(name, func(t *testing.T) {
			run := newComposeUpdateRun(t, true, "")
			dir, _ := composeSessionProject(t, "D")

			out, err := runQualityCompose(t, dir, strings.NewReader(input), "--apply")

			require.NoError(t, err, out)
			assert.Contains(t, out, "quality.applied_platforms = 2\n")
			assert.Len(t, *run.probes, 1, "one catalog read per run")
			assert.False(t, run.adapterProbed(t), "the apply must read the run's snapshot")
		})
	}
}

// A process stdin that is not a terminal keeps the existing error: nothing is
// read, no table is drawn, and no catalog is probed.
func TestQualityCompose_NonTerminalStdinKeepsTheExistingError(t *testing.T) {
	run := newComposeUpdateRun(t, false, "")
	dir, _ := composeSessionProject(t, "D")
	before := composeSessionHash(t, dir)

	out, err := runQualityCompose(t, dir, nil)

	require.Error(t, err)
	assert.Equal(t, qualityComposeTTYError, err.Error())
	assert.NotContains(t, out, "에이전트 모델 구성")
	assert.Empty(t, *run.probes)
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// `--apply` after a confirmed change generates with the snapshot the table
// read: one catalog probe, no adapter probing for itself, and generated
// Claude Code and Codex agents equal to the approved preview (S12, REQ-MUX-19).
func TestQualityCompose_ApplyGeneratesTheApprovedPreviewWithTheRunSnapshot(t *testing.T) {
	t.Run("R2 refresh", func(t *testing.T) {
		run := newComposeUpdateRun(t, true, "")
		dir := writeComposeRFixture(t, "R2")

		out, err := runQualityCompose(t, dir, strings.NewReader("u\ny\n"), "--apply")

		require.NoError(t, err, out)
		assert.Contains(t, out, "quality.applied_platforms = 2\n")
		assert.Len(t, *run.probes, 1)
		assert.False(t, run.adapterProbed(t), "generation must read the run's snapshot")
		planner := readComposeTableClaudeProfile(t, dir, "planner")
		assert.Equal(t, "fable", planner.model)
		assert.Equal(t, "max", planner.effort)
		codexPlanner := readComposeCodexProfile(t, dir, "planner")
		assert.Equal(t, "gpt-6-astra", codexPlanner.Model)
		assert.Equal(t, "max", codexPlanner.Effort)
		assertGeneratedEqualsTable(t, dir, composeRenderedTable(t, out, "미리보기 (아직 바뀌지 않음)"))
	})
	t.Run("D group edit", func(t *testing.T) {
		run := newComposeUpdateRun(t, true, "")
		dir, _ := composeSessionProject(t, "D")

		out, err := runQualityCompose(t, dir, strings.NewReader("g\n\n3\n\n4\ny\n"), "--apply")

		require.NoError(t, err, out)
		assert.Contains(t, out, "quality.applied_platforms = 2\n")
		assert.Len(t, *run.probes, 1)
		assert.False(t, run.adapterProbed(t), "generation must read the run's snapshot")
		explorer := readComposeCodexProfile(t, dir, "explorer")
		assert.Equal(t, "gpt-6-luna", explorer.Model)
		assert.Equal(t, "medium", explorer.Effort)
		claude := readComposeTableClaudeProfile(t, dir, "explorer")
		assert.Equal(t, "haiku", claude.model)
		assert.Empty(t, claude.effort, "a haiku agent carries no effort line")
		claudeMarks, codexMarks := assertGeneratedEqualsTable(t, dir, composeRenderedTable(t, out, "미리보기 (아직 바뀌지 않음)"))
		assert.Equal(t, 16, claudeMarks)
		assert.Equal(t, 4, codexMarks)
	})
}

// S13 F1 through the composition write: refused whether or not the variable
// is set, the file byte-identical, and the value never written.
func TestQualityCompose_RefusesTheF001FixtureAndLeaksNothing(t *testing.T) {
	for _, set := range []bool{true, false} {
		t.Run(map[bool]string{true: "variable set", false: "variable unset"}[set], func(t *testing.T) {
			setPresetNote(t, set)
			setComposeLanguage(t, "ko")
			dir := writeBlockFixture(t, blockLeakAlias)
			want := sha256.Sum256([]byte(blockLeakAlias))

			out, err := runQualityCompose(t, dir, strings.NewReader("g\n\n3\n\n4\ny\n"), "--apply")

			require.ErrorIs(t, err, errQualityWriteRefused, out)
			assert.Equal(t, qualityWriteRefusalText, err.Error())
			written := readBlockFixture(t, dir)
			assert.Equal(t, want, sha256.Sum256([]byte(written)))
			assert.NotContains(t, written, presetNoteSecret)
			assert.NotContains(t, out, "quality.applied_platforms")
		})
	}
}
