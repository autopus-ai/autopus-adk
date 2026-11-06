package cli

// SPEC-TUIUX-001 T8 (REQ-TUX-02, 03, 10, 15; acceptance S10, S11 (a)): an
// edit pass keeps a back stack of its asked forms. Esc restores the tiers
// and the effort working copy saved before the form and asks the form before
// it; the summary lists the pass's answers before the current form.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tuiuxBlockBefore is the summary block of the occurrence-th form whose
// title line starts with title: the lines between its header and footer.
func tuiuxBlockBefore(t *testing.T, out, title string, occurrence int) []string {
	t.Helper()
	lines := strings.Split(out, "\n")
	seen := 0
	for i, line := range lines {
		if !strings.HasPrefix(line, title) {
			continue
		}
		if seen++; seen < occurrence {
			continue
		}
		header := i - 1
		for header >= 0 && !strings.Contains(lines[header], " · 모델 구성") {
			header--
		}
		require.GreaterOrEqual(t, header, 0)
		return lines[header+1 : i-1]
	}
	t.Fatalf("form %q appears %d times, not %d:\n%s", title, seen, occurrence, out)
	return nil
}

const tuiuxBuildGroup = "구현 — executor, "

// S10: back inside the role groups pass restores the effort working copy,
// keeps the earlier answers preselected, and drops the pass at its first form.
func TestComposeNavPass_S10RoleGroups(t *testing.T) {
	newTUIUXEnv(t, true, "")
	start := []string{"2\n", "2\n", "1\n"}
	rest := tuiuxEnters(6) // the tiers and efforts of 구현, 검증·리뷰, 탐색·보조
	apply := []string{"2\n", "\n"}

	t.Run("(a) back to the effort keeps low", func(t *testing.T) {
		out, npd, dir := tuiuxQualityD(t, tuiuxLines(start, []string{"2\n", "2\n", "b\n", "\n"}, rest, apply)...)
		_, control, _ := tuiuxQualityD(t, tuiuxLines(start, []string{"2\n", "2\n"}, rest, apply)...)

		assert.Equal(t, []string{"지금까지 고른 값", "  설계·판단: fable", "  설계·판단 effort: low"}, tuiuxBlockBefore(t, out, tuiuxBuildGroup, 1))
		assert.Equal(t, []string{"지금까지 고른 값", "  설계·판단: fable"}, tuiuxBlockBefore(t, out, "설계·판단 effort", 2))
		saved := composeBindLoad(t, dir).Quality.Presets["claude-custom"]
		for _, agent := range composeDesignAgents {
			assert.Equal(t, "fable", saved.Agents[agent], agent)
			assert.Equal(t, "low", saved.Efforts[agent], agent)
		}
		assert.Equal(t, control, npd)
	})
	t.Run("(b) back past the effort drops low", func(t *testing.T) {
		out, npd, dir := tuiuxQualityD(t, tuiuxLines(start, []string{"2\n", "2\n", "b\n", "b\n", "4\n", "\n"}, rest, apply)...)
		_, control, _ := tuiuxQualityD(t, tuiuxLines(start, []string{"4\n", "\n"}, rest, apply)...)

		require.Contains(t, out, "적용할까요? \n")
		saved := composeBindLoad(t, dir).Quality.Presets["claude-custom"]
		for _, agent := range composeDesignAgents {
			assert.Equal(t, "sonnet", saved.Agents[agent], agent)
			assert.NotContains(t, saved.Efforts, agent)
		}
		assert.Equal(t, control, npd)
	})
	t.Run("(c) back to the method drops the pass", func(t *testing.T) {
		before := tuiuxNPD(t, tuiuxFixtureD(t))
		out, npd, _ := tuiuxQualityD(t, tuiuxLines(start, []string{"2\n", "2\n", "b\n", "b\n", "b\n", "1\n"}, tuiuxEnters(8), []string{"\n"})...)

		assert.Equal(t, 1, strings.Count(out, composeTextKO.NoChange+"\n"))
		assert.Equal(t, 2, strings.Count(out, "어떻게 고칠까요? \n"))
		assert.Equal(t, before, npd)
	})
}

// S11 (a): back from the second agent list asks the agent's effort again
// with its value in the summary; the effort answer stays.
func TestComposeNavPass_S11PerAgent(t *testing.T) {
	newTUIUXEnv(t, true, "")
	head := []string{"2\n", "2\n", "2\n", "2\n", "3\n", "2\n"} // 구성 편집, Claude Code, 에이전트별, planner, opus, low
	out, npd, dir := tuiuxQualityD(t, tuiuxLines(head, []string{"b\n", "\n", "1\n", "2\n", "\n"})...)
	_, control, _ := tuiuxQualityD(t, tuiuxLines(head, []string{"1\n", "2\n", "\n"})...)

	assert.Equal(t, []string{"지금까지 고른 값", "  planner: opus"}, tuiuxBlockBefore(t, out, "planner effort", 2))
	assert.Empty(t, tuiuxBlockBefore(t, out, "planner ", 1), "a list answer is no summary line")
	assert.Equal(t, []string{"지금까지 고른 값", "  planner: opus", "  planner effort: low"}, tuiuxBlockBefore(t, out, "에이전트를 고르세요", 2))
	saved := composeBindLoad(t, dir).Quality.Presets["claude-custom"]
	assert.Equal(t, "opus", saved.Agents["planner"])
	assert.Equal(t, "low", saved.Efforts["planner"])
	assert.Equal(t, control, npd)
}

// Back from an agent's value shows the list again with that agent selected:
// Enter picks it again.
func TestComposeNavPass_ListShownAgainSelectsTheAgent(t *testing.T) {
	newTUIUXEnv(t, true, "")
	out, _, _ := tuiuxQualityD(t, "2\n", "2\n", "2\n", "3\n", "b\n", "\n", "b\n", "1\n", "\n")

	assert.Equal(t, 2, strings.Count(out, "\narchitect \n1. 그대로"), "Enter on the list shown again opened architect")
}
