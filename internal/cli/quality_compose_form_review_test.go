package cli

// SPEC-MODELUX-002 T5 (REQ-MUX2-03, REQ-MUX2-09, REQ-MUX2-13; acceptance S7,
// S11): the confirmation select, keep editing with the working composition,
// an unchanged edit, the refresh, and the baseline every keep path leaves.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeFormS5 answers S5 up to its confirmation: 구성 편집, Codex, 역할
// 그룹, 설계·판단 on opus, the other groups kept.
var composeFormS5 = []string{"2\n", "3\n", "\n", "3\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n"}

func composeFormLines(head []string, tail ...string) []string {
	return append(append([]string(nil), head...), tail...)
}

// S7: Enter at the confirmation cancels with the SPEC-MODELUX-001 line and
// returns to the menu; the file stays byte-identical.
func TestComposeForms_EnterAtTheConfirmationCancels(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)

	result, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{Presets: true},
		composeFormLines(composeFormS5, "\n", "\n")...)

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.Contains(t, out, "적용할까요? \n1. 취소\n2. 적용\n3. 계속 편집\n")
	assert.Contains(t, out, "취소했습니다. 설정을 바꾸지 않았습니다.\n")
	assert.Equal(t, 2, strings.Count(out, composeFormMenuQuality))
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// S7: keep editing returns to the same tab's method select with the working
// composition, which the per-agent list then shows.
func TestComposeForms_KeepEditingKeepsTheWorkingComposition(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")

	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{Presets: true},
		composeFormLines(composeFormS5, "3\n", "2\n", "\n", "\n", "\n")...)

	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(out, "어떻게 고칠까요? \n"))
	assert.Contains(t, out, "2. planner — 6.1-sol · xhigh\n")
}

// S7: keeping every group is no change: no confirmation, nothing written.
func TestComposeForms_UnchangedEditWritesNothing(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)

	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{Presets: true},
		"2\n", "3\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n")

	require.NoError(t, err)
	assert.Contains(t, out, "변경 없음\n")
	assert.NotContains(t, out, "적용할까요?")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// S11: after an applied Codex edit, a cancelled Claude edit in the same run
// leaves the file exactly as the first write left it: the baseline is the
// last successful write. firstOnly stops after the first write.
func TestComposeForms_CancelAfterAWriteKeepsThatWrite(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	firstOnly := writeComposeFixture(t, "claude-code", "codex")
	_, _, err := runComposeForms(t, newComposeFormSession(t, firstOnly), composeFormOptions{Presets: true},
		composeFormLines(composeFormS5, "2\n", "\n")...)
	require.NoError(t, err)
	dir := writeComposeFixture(t, "claude-code", "codex")

	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{Presets: true},
		composeFormLines(composeFormS5, "2\n", "2\n", "2\n", "\n", "3\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n")...)

	require.NoError(t, err)
	assert.Contains(t, out, "취소했습니다. 설정을 바꾸지 않았습니다.\n")
	assert.Equal(t, composeSessionHash(t, firstOnly), composeSessionHash(t, dir))
	saved := composeBindLoad(t, dir).Quality
	assert.Equal(t, "codex-custom", saved.Providers["codex"])
	assert.NotContains(t, saved.Providers, "claude")
}

// The refresh previews and writes every historical built-in preset; keep
// editing from its preview binds on top of it in the 모든 도구 같게 tab.
func TestComposeForms_RefreshRewritesTheHistoricalPresets(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir, _ := composeSessionProject(t, "R")

	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{}, "2\n", "2\n", "\n")

	require.NoError(t, err)
	assert.Contains(t, out, composeFormPreviewMarker)
	saved := composeBindLoad(t, dir).Quality
	current := config.DefaultFullConfig("").Quality
	assert.Equal(t, current.Presets["balanced"].Agents, saved.Presets["balanced"].Agents)
	assert.Equal(t, current.Presets["ultra"].Agents, saved.Presets["ultra"].Agents)
	assert.Equal(t, "ultra", saved.Default)
}

func TestComposeForms_KeepEditingTheRefreshOpensTheAllToolsMethod(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir, _ := composeSessionProject(t, "R")
	before := composeSessionHash(t, dir)

	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{}, "2\n", "3\n", "\n", "\n")

	require.NoError(t, err)
	assert.NotContains(t, out, "어느 도구를 바꿀까요?")
	assert.Contains(t, out, "어떻게 고칠까요? \n1. 역할 그룹\n2. 에이전트별\n")
	assert.Contains(t, out, "설계·판단 — planner, architect, spec-writer, security-auditor \n")
	assert.Equal(t, before, composeSessionHash(t, dir), "input end cancels the bound refresh")
}
