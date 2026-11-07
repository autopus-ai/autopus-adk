package cli

// SPEC-MODELUX-003 T9 (REQ-MUX3-03, REQ-MUX3-04, REQ-MUX3-05, REQ-MUX3-18;
// acceptance S3, S4, S5, S18): on the Claude Code and Codex tabs every tier
// select is followed by that tool's effort select, whose choice is stored in
// <provider>-custom and generated exactly as previewed.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeEffortS3 answers S3 up to the confirmation: 구성 편집, Claude Code,
// 역할 그룹, 구현 kept on sonnet at xhigh, 검증·리뷰's tier and effort kept.
var composeEffortS3 = []string{"2\n", "2\n", "\n", "4\n", "5\n", "\n", "\n"}

// composeEffortS4 answers S4 up to the confirmation over CAT-E: 구성 편집,
// Codex, 역할 그룹, 구현 on opus at high, 검증·리뷰's tier kept at high. opus
// puts frontend-specialist on the Sol rung, whose CAT-E levels stop at xhigh.
var composeEffortS4 = []string{"2\n", "3\n", "\n", "3\n", "4\n", "\n", "4\n"}

func composeEfforts(level string, agents ...string) map[string]string {
	efforts := make(map[string]string, len(agents))
	for _, agent := range agents {
		efforts[agent] = level
	}
	return efforts
}

// runQualityEffortForms runs `auto quality --apply` over dir with the form
// answers and the run's catalog probe answering catalog.
func runQualityEffortForms(t *testing.T, dir, catalog string, lines ...string) string {
	t.Helper()
	composeSessionCatalog(t, catalog)
	out, err := runQualityForms(t, dir, nil, formInput(lines...), "--apply")
	require.NoError(t, err, out)
	return out
}

// S3: the Claude Code tab asks 구현's effort after its tier, labels every
// level with the cells it leaves, and writes, previews, and generates the
// xhigh effort; Codex keeps its standard placement.
func TestComposeEffortForms_ClaudeGroupEffortIsWrittenAndGenerated(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")
	baseline := t.TempDir()
	_, err := driftAdapterFor("codex", baseline, composeTableSnapshot(t, "CAT-A")).Generate(context.Background(), composeBindLoad(t, dir))
	require.NoError(t, err)

	out := runQualityEffortForms(t, dir, "CAT-A", composeFormLines(composeEffortS3, "2\n", "\n")...)

	assert.Contains(t, out, "구현 effort \n"+
		"1. 그대로 — sonnet-5-5 · max\n"+
		"2. sonnet-5-5 · low\n"+
		"3. sonnet-5-5 · medium\n"+
		"4. sonnet-5-5 · high\n"+
		"5. sonnet-5-5 · xhigh\n"+
		"6. sonnet-5-5 · max (자동)\n")
	preview := composeRenderedTable(t, out, composeFormPreviewMarker)
	assert.Equal(t, "sonnet · xhigh*", preview["frontend-specialist"]["Claude Code (개별 설정: claude-custom)"])
	assert.Contains(t, out, "바뀌는 설정:\n"+
		"  quality.presets.claude-custom 새로 씀 (4개 에이전트)\n"+
		"  quality.presets.claude-custom.efforts 새로 씀 (1개 에이전트)\n"+
		"  quality.providers.claude 없음 → claude-custom\n"+
		"  claude-custom은 balanced 표준 배치를 받지 않음\n"+
		"  "+composeToolBlockNotice+"\n"+
		"  quality.providers는 v0.50.88보다 오래된 auto가 읽지 않습니다\n"+
		"  quality.presets.*.efforts는 v0.50.127보다 오래된 auto가 읽지 못합니다\n"+
		"적용할까요? \n")
	assert.Equal(t, composeEfforts("xhigh", "frontend-specialist"), composeBindLoad(t, dir).Quality.Presets["claude-custom"].Efforts)
	assertGeneratedEqualsTable(t, dir, plainComposeHeaders(preview))
	assert.Equal(t, composeTableClaudeCell{model: "sonnet", effort: "xhigh"}, readComposeTableClaudeProfile(t, dir, "frontend-specialist"))
	assert.Equal(t, readComposeFile(t, baseline, ".codex", "agents", "frontend-specialist.toml"),
		readComposeFile(t, dir, ".codex", "agents", "frontend-specialist.toml"))
}

// S4: the Codex tab offers the levels the catalog reports for Sol, which
// stop at xhigh, and the chosen high is stored and generated on Sol.
func TestComposeEffortForms_CodexOffersOnlyTheCatalogLevels(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")

	out := runQualityEffortForms(t, dir, "CAT-E", composeFormLines(composeEffortS4, "2\n", "\n")...)

	assert.Contains(t, out, "구현 effort \n"+
		"1. 그대로 — 6.1-sol · xhigh\n"+
		"2. 6.1-sol · low\n"+
		"3. 6.1-sol · medium\n"+
		"4. 6.1-sol · high\n"+
		"5. 6.1-sol · xhigh (자동)\n"+
		"Enter")
	assert.Equal(t, composeEfforts("high", config.CanonicalAgentNames()...), composeBindLoad(t, dir).Quality.Presets["codex-custom"].Efforts)
	assertGeneratedEqualsTable(t, dir, plainComposeHeaders(composeRenderedTable(t, out, composeFormPreviewMarker)))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6.1-sol", Effort: "high"}, readComposeCodexProfile(t, dir, "frontend-specialist"))
	assert.Equal(t, composeTableClaudeCell{model: "claude-sonnet-5-5", effort: "max"}, readComposeTableClaudeProfile(t, dir, "frontend-specialist"))
}

// S4, the copy: an unreadable catalog prints the notice once and opens no
// effort select.
func TestComposeEffortForms_UnreadableCodexCatalogAsksNoEffort(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)

	out := runQualityEffortForms(t, dir, "CAT-C", "2\n", "3\n", "\n", "2\n", "\n", "\n", "\n", "\n", "\n")

	assert.Equal(t, 1, strings.Count(out, "Codex catalog를 읽지 못해 effort는 자동입니다\n"), out)
	assert.NotContains(t, out, " effort \n1. ")
	assert.Contains(t, out, "적용할까요? \n")
	assert.Equal(t, before, composeSessionHash(t, dir), "Enter at the confirmation cancels")
}

// S5 over the file S4 leaves: (1) every select kept is no change; (2) a tier
// change that keeps the efforts writes no efforts line; (3)
// frontend-specialist back to the automatic level drops it from the efforts.
func TestComposeEffortForms_KeepAndBackToAutomatic(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")
	runQualityEffortForms(t, dir, "CAT-E", composeFormLines(composeEffortS4, "2\n", "\n")...)
	afterS4 := composeSessionHash(t, dir)

	kept := runQualityEffortForms(t, dir, "CAT-E", "2\n", "3\n", "\n", "\n", "\n", "\n", "\n", "\n")
	assert.Contains(t, kept, "변경 없음\n")
	assert.Equal(t, afterS4, composeSessionHash(t, dir))

	tier := runQualityEffortForms(t, dir, "CAT-E", "2\n", "3\n", "2\n", "5\n", "3\n", "\n", "1\n", "2\n", "\n")
	assert.Equal(t, composeEfforts("high", config.CanonicalAgentNames()...), composeBindLoad(t, dir).Quality.Presets["codex-custom"].Efforts)
	assert.Equal(t, "opus", composeBindLoad(t, dir).Quality.Presets["codex-custom"].Agents["ux-validator"])
	assert.NotContains(t, tier, ".efforts ")
	assert.NotContains(t, tier, "v0.50.127")

	automatic := runQualityEffortForms(t, dir, "CAT-E", "2\n", "3\n", "2\n", "2\n", "\n", "5\n", "1\n", "2\n", "\n")
	assert.Contains(t, automatic, "frontend-specialist effort \n1. 그대로 — 6.1-sol · high\n2. 6.1-sol · low\n"+
		"3. 6.1-sol · medium\n4. 6.1-sol · high\n5. 6.1-sol · xhigh (자동)\n")
	assert.Contains(t, automatic, "  quality.presets.codex-custom.efforts 고쳐 씀 (3개 에이전트)\n")
	assert.NotContains(t, automatic, "v0.50.127")
	assert.Equal(t, composeEfforts("high", "reviewer", "qa-explorer", "ux-validator"),
		composeBindLoad(t, dir).Quality.Presets["codex-custom"].Efforts)
	assert.Equal(t, config.CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"}, readComposeCodexProfile(t, dir, "frontend-specialist"))
}

// S18: the effort select follows the locale; the stored values do not.
func TestComposeEffortForms_LocaleChangesTheTextOnly(t *testing.T) {
	for lang, want := range map[string][]string{
		"en": {"Implementation effort \n1. keep — 6.1-sol · xhigh\n", "5. 6.1-sol · xhigh (automatic)\n"},
		"ko": {"구현 effort \n1. 그대로 — 6.1-sol · xhigh\n", "5. 6.1-sol · xhigh (자동)\n"},
	} {
		t.Run(lang, func(t *testing.T) {
			newComposeUpdateRun(t, true, "")
			setComposeLanguage(t, lang)
			useAccessibleComposeForms(t)
			dir := writeComposeFixture(t, "claude-code", "codex")

			out := runQualityEffortForms(t, dir, "CAT-E", composeFormLines(composeEffortS4, "2\n", "\n")...)

			for _, line := range want {
				assert.Contains(t, out, line)
			}
			assert.Equal(t, composeEfforts("high", config.CanonicalAgentNames()...), composeBindLoad(t, dir).Quality.Presets["codex-custom"].Efforts)
		})
	}
}

// The unreadable-catalog notice follows the locale too.
func TestComposeEffortForms_UnreadableNoticeIsTranslated(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	setComposeLanguage(t, "en")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code", "codex")

	out := runQualityEffortForms(t, dir, "CAT-C", "2\n", "3\n", "\n", "3\n")

	assert.Contains(t, out, "Codex catalog unreadable; effort stays automatic\n")
}

// Ctrl+C at an effort select or at the confirmation after it ends the step
// with nothing written.
func TestComposeEffortForms_InterruptWritesNothing(t *testing.T) {
	for name, nth := range map[string]int{"effort select": 5, "confirmation": 8} {
		t.Run(name, func(t *testing.T) {
			composeSessionCatalog(t, "CAT-A")
			dir := writeComposeFixture(t, "claude-code", "codex")
			before := composeSessionHash(t, dir)
			abortComposeFormAt(t, nth)
			cmd, out := newComposeTestCommand()
			cmd.SetIn(formInput(composeEffortS3...))

			_, err := newComposeFormSession(t, dir).runForms(cmd, composeFormOptions{Presets: true})

			assert.True(t, errors.Is(err, errComposeInterrupted), "%v", err)
			assert.Equal(t, 1, strings.Count(out.String(), composeFormTextKO.Interrupted+"\n"))
			if nth == 8 {
				assert.Contains(t, out.String(), "바뀌는 설정:\n", "the confirmation follows the preview")
			}
			assert.Equal(t, before, composeSessionHash(t, dir))
		})
	}
}

// init's memory writer puts the efforts into the config init saves, and the
// same run generates them.
func TestComposeEffortForms_InitSavesAndGeneratesTheEfforts(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	stubInitWizard(t, tui.QualityRoleGroups)
	useAccessibleComposeForms(t)

	dir, out, err := runComposeInit(t, formInput(composeFormLines(composeEffortS3, "2\n", "\n")...))

	require.NoError(t, err, out)
	assert.NotContains(t, out, composeToolBlockNotice, "init rewrites no file")
	assert.Contains(t, out, "  quality.presets.*.efforts는 v0.50.127보다 오래된 auto가 읽지 못합니다\n")
	assert.Equal(t, composeEfforts("xhigh", "frontend-specialist"), composeBindLoad(t, dir).Quality.Presets["claude-custom"].Efforts)
	assert.Equal(t, composeTableClaudeCell{model: "sonnet", effort: "xhigh"}, readComposeTableClaudeProfile(t, dir, "frontend-specialist"))
}

func readComposeFile(t *testing.T, dir string, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{dir}, parts...)...))
	require.NoError(t, err)
	return string(data)
}
