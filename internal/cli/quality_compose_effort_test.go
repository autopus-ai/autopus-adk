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

// A haiku agent takes no Claude effort: the group select covers the other
// members only, and the choice is stored for them alone.
func TestComposeEffortOptions_ClaudeLeavesHaikuOut(t *testing.T) {
	dir := writeComposeToolPresetFixture(t, "claude", map[string]string{"annotator": "haiku"})

	out := runComposeEffortSession(t, dir, "CAT-A",
		"2\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "4\n", "2\n", "\n")

	assert.Contains(t, out, "탐색·보조 effort \n"+
		"1. 그대로 — sonnet-5-5 · medium / opus-5-5 · high\n"+
		"2. sonnet-5-5 · low / opus-5-5 · low\n"+
		"3. sonnet-5-5 · medium / opus-5-5 · medium\n"+
		"4. sonnet-5-5 · high / opus-5-5 · high\n"+
		"5. sonnet-5-5 · xhigh / opus-5-5 · xhigh\n"+
		"6. sonnet-5-5 · max / opus-5-5 · max\n")
	assert.Equal(t, map[string]string{"explorer": "high"}, composeBindLoad(t, dir).Quality.Presets["claude-custom"].Efforts)
}

// A group whose every member is haiku asks no Claude effort.
func TestComposeEffortOptions_AllHaikuGroupAsksNoEffort(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")

	out := runComposeEffortSession(t, dir, "CAT-A",
		"2\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "5\n", "\n", "\n")

	assert.Contains(t, out, "검증·리뷰 effort \n")
	assert.NotContains(t, out, "탐색·보조 effort")
	assert.Contains(t, out, "적용할까요? \n")
}

// Codex offers the levels every member's model reports: Sol's four for
// 탐색·보조, which mixes Luna and the analysis rung's Sol. Each member at the
// chosen level stores it unless that level is its automatic one.
func TestComposeEffortOptions_CodexIntersectsTheMembersLevels(t *testing.T) {
	dir := writeComposeToolPresetFixture(t, "codex", nil)

	// 구성 편집, Codex, 역할 그룹, every tier and effort kept up to 탐색·보조
	// effort xhigh, 적용.
	out := runComposeEffortSession(t, dir, "CAT-E",
		"2\n", "3\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "5\n", "2\n", "\n")

	assert.Contains(t, out, "탐색·보조 effort \n"+
		"1. 그대로 — 6-luna · max / 6.1-sol · xhigh\n"+
		"2. 6-luna · low / 6.1-sol · low\n"+
		"3. 6-luna · medium / 6.1-sol · medium\n"+
		"4. 6-luna · high / 6.1-sol · high\n"+
		"5. 6-luna · xhigh / 6.1-sol · xhigh\n"+
		"Enter")
	assert.Equal(t, composeEfforts("xhigh", "explorer", "annotator"),
		composeBindLoad(t, dir).Quality.Presets["codex-custom"].Efforts)
}

// On the standard placement the automatic level is the one the start
// shows. A level that equals the custom binding's automatic effort but not
// the start's is stored where that binding differs (reverse-engineer, on
// opus), which lands every member on the chosen level, so the choice is what
// the operator gets.
func TestComposeEffortOptions_AutomaticIsJudgedWhereTheChoiceLands(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")

	out := runComposeEffortSession(t, dir, "CAT-A",
		"2\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "3\n", "2\n", "\n")

	assert.Contains(t, out, "탐색·보조 effort \n"+
		"1. 그대로 — sonnet-5-5 · high / opus-5-5 · high\n"+
		"2. sonnet-5-5 · low / opus-5-5 · low\n"+
		"3. sonnet-5-5 · medium / opus-5-5 · medium\n"+
		"4. sonnet-5-5 · high / opus-5-5 · high (자동)\n"+
		"5. sonnet-5-5 · xhigh / opus-5-5 · xhigh\n"+
		"6. sonnet-5-5 · max / opus-5-5 · max\n")
	saved := composeBindLoad(t, dir).Quality
	assert.Equal(t, composeEfforts("medium", "reverse-engineer"), saved.Presets["claude-custom"].Efforts)
	for _, agent := range []string{"explorer", "annotator"} {
		model, effort := saved.ClaudeAgentProfile(agent, "sonnet")
		assert.Equal(t, []string{"sonnet", "medium"}, []string{model, effort}, agent)
	}
}

// Choosing the automatic level of the start stores nothing, so the edit is
// no change.
func TestComposeEffortOptions_AutomaticChoiceOnTheStartIsNoChange(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)

	out := runComposeEffortSession(t, dir, "CAT-A",
		"2\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "4\n", "\n")

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
