package cli

// SPEC-MODELUX-003 T9 (REQ-MUX3-03; Effort Option Contract): which agents an
// effort select covers, which levels it offers, and which choices store an
// effort, through the form flow of a real session.

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// writeComposeToolPresetFixture saves D with tool bound to its custom preset:
// D's balanced tiers overlaid with tiers.
func writeComposeToolPresetFixture(t *testing.T, provider string, tiers map[string]string) string {
	t.Helper()
	cfg := composeFixtureConfig("claude-code", "codex")
	agents := maps.Clone(cfg.Quality.Presets["balanced"].Agents)
	maps.Copy(agents, tiers)
	name := provider + "-custom"
	cfg.Quality.Presets[name] = config.QualityPreset{Description: "custom", Agents: agents}
	cfg.Quality.Providers = map[string]string{provider: name}
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, cfg))
	return dir
}

// runComposeEffortSession runs a real file session's forms over dir with the
// run's catalog probe answering catalog.
func runComposeEffortSession(t *testing.T, dir, catalog string, lines ...string) string {
	t.Helper()
	composeSessionCatalog(t, catalog)
	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{Presets: true}, lines...)
	require.NoError(t, err, out)
	return out
}

// The Claude effort select leaves a haiku member out: 검증·리뷰 with
// ux-validator on haiku offers reviewer's and qa-explorer's opus levels
// only, and qa-explorer's automatic high stores nothing.
func TestComposeEffortOptions_ClaudeLeavesHaikuOut(t *testing.T) {
	dir := writeComposeToolPresetFixture(t, "claude", map[string]string{"ux-validator": "haiku"})

	out := runComposeEffortSession(t, dir, "CAT-A",
		"2\n", "2\n", "\n", "\n", "\n", "\n", "4\n", "2\n", "\n")

	assert.Contains(t, out, "검증·리뷰 effort \n"+
		"1. 그대로 — opus-5-5 · max / opus-5-5 · high\n"+
		"2. opus-5-5 · low\n"+
		"3. opus-5-5 · medium\n"+
		"4. opus-5-5 · high\n"+
		"5. opus-5-5 · xhigh\n"+
		"6. opus-5-5 · max\n")
	assert.Equal(t, map[string]string{"reviewer": "high"}, composeBindLoad(t, dir).Quality.Presets["claude-custom"].Efforts)
}

// A group whose every member is haiku asks no Claude effort.
func TestComposeEffortOptions_AllHaikuGroupAsksNoEffort(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")

	out := runComposeEffortSession(t, dir, "CAT-A", "2\n", "2\n", "\n", "5\n", "\n", "\n", "\n")

	assert.Contains(t, out, "검증·리뷰 effort \n")
	assert.NotContains(t, out, "구현 effort")
	assert.Contains(t, out, "적용할까요? \n")
}

// Codex offers the levels every member's model reports: Sol's four for
// 검증·리뷰, which mixes Astra, Luna, and the analysis rung's Sol. Each member
// at the chosen level stores it unless that level is its automatic one.
func TestComposeEffortOptions_CodexIntersectsTheMembersLevels(t *testing.T) {
	dir := writeComposeToolPresetFixture(t, "codex", nil)

	// 구성 편집, Codex, 역할 그룹, every tier and effort kept up to 검증·리뷰
	// effort xhigh, 적용.
	out := runComposeEffortSession(t, dir, "CAT-E",
		"2\n", "3\n", "\n", "\n", "\n", "\n", "5\n", "2\n", "\n")

	assert.Contains(t, out, "검증·리뷰 effort \n"+
		"1. 그대로 — 6-astra · max / 6.1-sol · xhigh / 6-luna · max\n"+
		"2. 6-astra · low / 6.1-sol · low / 6-luna · low\n"+
		"3. 6-astra · medium / 6.1-sol · medium / 6-luna · medium\n"+
		"4. 6-astra · high / 6.1-sol · high / 6-luna · high\n"+
		"5. 6-astra · xhigh / 6.1-sol · xhigh / 6-luna · xhigh\n"+
		"Enter")
	assert.Equal(t, composeEfforts("xhigh", "reviewer", "ux-validator"),
		composeBindLoad(t, dir).Quality.Presets["codex-custom"].Efforts)
}

// A level is stored where the custom binding's automatic effort differs
// from it (reviewer and qa-explorer, on opus) and not where it is the
// binding's automatic one (ux-validator, on the sonnet ladder), which lands
// every member on the chosen level, so the choice is what the operator gets.
func TestComposeEffortOptions_AutomaticIsJudgedWhereTheChoiceLands(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")

	out := runComposeEffortSession(t, dir, "CAT-A",
		"2\n", "2\n", "\n", "\n", "\n", "\n", "3\n", "2\n", "\n")

	assert.Contains(t, out, "검증·리뷰 effort \n"+
		"1. 그대로 — opus-5-5 · max / opus-5-5 · high / sonnet-5-5 · high\n"+
		"2. opus-5-5 · low / sonnet-5-5 · low\n"+
		"3. opus-5-5 · medium / sonnet-5-5 · medium\n"+
		"4. opus-5-5 · high / sonnet-5-5 · high\n"+
		"5. opus-5-5 · xhigh / sonnet-5-5 · xhigh\n"+
		"6. opus-5-5 · max / sonnet-5-5 · max\n")
	saved := composeBindLoad(t, dir).Quality
	assert.Equal(t, composeEfforts("medium", "reviewer", "qa-explorer"), saved.Presets["claude-custom"].Efforts)
	for agent, model := range map[string]string{"qa-explorer": "opus", "ux-validator": "sonnet"} {
		got, effort := saved.ClaudeAgentProfile(agent, "sonnet")
		assert.Equal(t, []string{model, "medium"}, []string{got, effort}, agent)
	}
}

// Choosing the automatic level of the start stores nothing, so the edit is
// no change.
func TestComposeEffortOptions_AutomaticChoiceOnTheStartIsNoChange(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)

	// 구현 effort 6 is sonnet-5-5 · max (자동), the start's own level.
	out := runComposeEffortSession(t, dir, "CAT-A", "2\n", "2\n", "\n", "\n", "6\n", "\n", "\n", "\n")

	assert.Contains(t, out, "구현 effort \n1. 그대로 — sonnet-5-5 · max\n")
	assert.Contains(t, out, "변경 없음\n")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// 모든 도구 같게 asks no effort: its edit binds every tool alike.
func TestComposeEffortOptions_AllToolsTabAsksNoEffort(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")

	out := runComposeEffortSession(t, dir, "CAT-A", "2\n", "1\n", "\n", "2\n", "\n", "\n", "\n", "\n", "\n")

	assert.NotContains(t, out, " effort \n")
	assert.Contains(t, out, "적용할까요? \n")
}
