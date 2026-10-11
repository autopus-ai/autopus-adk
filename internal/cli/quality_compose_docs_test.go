package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-MODELUX-001 REQ-MUX-23 / S18: the help of auto quality, auto update,
// and auto init documents the role groups, the table, the binding, and the
// update question rules. Every statement below comes from the code that
// prints it, so renaming a group, moving an agent, or rewording a label
// fails here until the documents follow.

// composeDocGroups returns each role group's label and its members in
// display order, as the composition step prints them.
func composeDocGroups(lang string) []string {
	var want []string
	for _, group := range composeRoleGroups {
		want = append(want, group.Label(lang), strings.Join(group.Agents, ", "))
	}
	return want
}

// composeDocLabels returns the table and status labels a document names in
// lang: the OMP column scope, the platform default cell, and the three GAP 1
// classes, the customized one with N for its count.
func composeDocLabels(lang string) []string {
	labels := composeTableLabelsFor(lang)
	text := composeTextFor(lang)
	return []string{
		labels.ompRequested,
		labels.platformDefault,
		text.StatusCurrent,
		text.StatusHistorical,
		strings.Replace(text.StatusCustomized, "%d", "N", 1),
	}
}

func TestQualityHelp_LongDocumentsGroupsTableAndBinding(t *testing.T) {
	t.Parallel()

	long := newQualityCmd().Long
	help := flowText(long)
	for _, want := range composeDocGroups("en") {
		assert.Contains(t, help, want)
	}
	for _, want := range composeDocLabels("en") {
		assert.Contains(t, help, want)
	}
	for _, want := range []string{
		composeTextFor("en").Question,
		"agent=tier",
		"quality.presets.custom",
		"quality.default: custom",
		"never modified",
		"`auto quality show --agents`",
		"--apply",
	} {
		assert.Contains(t, help, want)
	}
	for i, line := range strings.Split(long, "\n") {
		assert.LessOrEqual(t, len([]rune(line)), 80, "help line %d is wider than 80 columns: %q", i+1, line)
	}
}

func TestUpdateHelp_LongDocumentsTheCompositionQuestionRules(t *testing.T) {
	t.Parallel()

	update, _, err := NewRootCmd().Find([]string{"update"})
	require.NoError(t, err)
	help := flowText(update.Long)
	assert.Contains(t, help, composeTextFor("ko").Question)
	for _, want := range []string{
		"Enter", "u는", "g는 역할 그룹 4개", "a는", "y로 확인", "CI", "TTY", "auto quality --help",
	} {
		assert.Contains(t, help, want)
	}
	// Every flag the help names as skipping the question is a real flag of
	// auto update, its own or inherited from the root command.
	for _, flag := range []string{"yes", "auto", "check", "workspace", "plan", "preview", "dry-run"} {
		assert.Contains(t, help, "--"+flag)
		found := update.Flags().Lookup(flag) != nil || update.InheritedFlags().Lookup(flag) != nil
		assert.True(t, found, "auto update has no --%s flag", flag)
	}
	assert.Contains(t, help, "-y")
	assert.NotNil(t, update.Flags().ShorthandLookup("y"), "auto update has no -y shorthand")
}

func TestInitHelp_LongDocumentsTheRoleGroupChoice(t *testing.T) {
	t.Parallel()

	initCmd, _, err := NewRootCmd().Find([]string{"init"})
	require.NoError(t, err)
	help := flowText(initCmd.Long)
	for _, group := range composeRoleGroups {
		assert.Contains(t, help, group.Label("ko"))
	}
	for _, want := range []string{"역할별 직접 구성", "balanced", "y로 확인", "--yes", "--quality"} {
		assert.Contains(t, help, want)
	}
	found := initCmd.Flags().Lookup("yes") != nil && initCmd.InheritedFlags().Lookup("quality") != nil
	assert.True(t, found, "auto init must have --yes and inherit --quality")
}
