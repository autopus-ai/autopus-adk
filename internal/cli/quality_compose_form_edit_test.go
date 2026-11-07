package cli

// SPEC-MODELUX-002 T5 (REQ-MUX2-05, REQ-MUX2-08, REQ-MUX2-09; acceptance S4,
// S6): the role group selects and the per-agent list through the form flow,
// the preview with every changed cell marked, and the confirmed write.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const composeFormPreviewMarker = "미리보기 (아직 바뀌지 않음)"

// composeFormMarks counts the change marks of one column in the preview.
func composeFormMarks(t *testing.T, out, column string) int {
	t.Helper()
	marks := 0
	for _, row := range composeRenderedTable(t, out, composeFormPreviewMarker) {
		if strings.HasSuffix(row[column], "*") {
			marks++
		}
	}
	return marks
}

// S4 through the form flow over D: the 구현 select names both tools' cells,
// fable binds custom, the preview marks the three Claude cells that move (the
// top-rung reviewer keeps the top-rung cell) and only frontend-specialist's
// Codex cell, and the four change lines precede the confirmation.
func TestComposeForms_AllToolsGroupEditBindsCustom(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	session := newComposeFormSession(t, dir)

	_, out, err := runComposeForms(t, session, composeFormOptions{Presets: true},
		"2\n", "1\n", "\n", "2\n", "\n", "2\n", "\n")

	require.NoError(t, err)
	assert.Contains(t, out, "구현 — frontend-specialist \n"+
		"1. 그대로 — sonnet\n"+
		"2. fable — Claude Code: fable-5-1 · max | Codex: 6-astra · max\n"+
		"3. opus — Claude Code: opus-5-5 · high | Codex: 6.1-sol · xhigh\n"+
		"4. sonnet — Claude Code: sonnet-5-5 · max | Codex: 6-luna · max\n"+
		"5. haiku — Claude Code: haiku-4-5 · - | Codex: 6-luna · medium\n")
	assert.Equal(t, 3, composeFormMarks(t, out, "Claude Code"))
	assert.Equal(t, 1, composeFormMarks(t, out, "Codex"))
	assert.Contains(t, out, "바뀌는 설정:\n"+
		"  quality.presets.custom 새로 씀 (4개 에이전트)\n"+
		"  quality.default balanced → custom\n"+
		"  custom은 balanced 표준 배치를 받지 않음\n"+
		"  "+composeToolBlockNotice+"\n"+
		"적용할까요? \n1. 취소\n2. 적용\n3. 계속 편집\n")
	saved := composeBindLoad(t, dir)
	assert.Equal(t, "custom", saved.Quality.Default)
	assert.Empty(t, saved.Quality.Providers)
	assert.Equal(t, map[string]string{
		"frontend-specialist": "fable", "reviewer": "opus", "qa-explorer": "opus", "ux-validator": "sonnet",
	}, saved.Quality.Presets["custom"].Agents)
	assert.Equal(t, "custom", session.Config.Quality.Default)
}

// S6 through the form flow over D: the Claude tab's per-agent list, the
// frontend-specialist value select, the list again with the cells the new
// binding gives, and a preview that marks the moved Claude cells and no
// Codex cell.
func TestComposeForms_ClaudeAgentEditBindsClaudeCustom(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")

	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{Presets: true},
		"2\n", "2\n", "2\n", "2\n", "3\n", "\n", "\n", "2\n", "\n")

	require.NoError(t, err)
	lists := strings.Split(out, "에이전트를 고르세요 \n")
	require.Len(t, lists, 3, out)
	assert.True(t, strings.HasPrefix(lists[1], "1. 완료\n2. frontend-specialist — sonnet-5-5 · max\n"), lists[1])
	assert.Contains(t, lists[1], "3. reviewer — opus-5-5 · max\n")
	assert.Contains(t, lists[1], "5. ux-validator — sonnet-5-5 · high\n")
	assert.Contains(t, out, "1. 그대로 — sonnet-5-5 · max\n2. fable-5-1 · max  [fable]\n"+
		"3. opus-5-5 · high  [opus]\n4. sonnet-5-5 · max  [sonnet]\n5. haiku-4-5 · -  [haiku]\n")
	assert.Contains(t, lists[2], "2. frontend-specialist — opus-5-5 · high\n")
	assert.Contains(t, lists[2], "5. ux-validator — sonnet-5-5 · medium\n")
	// reviewer keeps the top-rung cell under claude-custom.
	assert.Contains(t, lists[2], "3. reviewer — opus-5-5 · max\n")
	header := composeTableHeader(t, out, composeFormPreviewMarker)
	assert.Contains(t, header, "Claude Code (개별 설정: claude-custom)")
	preview := composeRenderedTable(t, out, composeFormPreviewMarker)
	assert.Equal(t, "opus · high*", preview["frontend-specialist"]["Claude Code (개별 설정: claude-custom)"])
	assert.Equal(t, "sonnet · medium*", preview["ux-validator"]["Claude Code (개별 설정: claude-custom)"])
	assert.Equal(t, "opus-5-5 · max", preview["reviewer"]["Claude Code (개별 설정: claude-custom)"])
	assert.Zero(t, composeFormMarks(t, out, "Codex"))
	saved := composeBindLoad(t, dir).Quality
	assert.Equal(t, "claude-custom", saved.Providers["claude"])
	assert.Equal(t, "Claude Code custom agent tiers", saved.Presets["claude-custom"].Description)
	assert.Equal(t, "opus", saved.Presets["claude-custom"].Agents["frontend-specialist"])
	assert.Equal(t, "opus", saved.Presets["claude-custom"].Agents["reviewer"])
	assert.Equal(t, "sonnet", saved.Presets["claude-custom"].Agents["ux-validator"])
}
