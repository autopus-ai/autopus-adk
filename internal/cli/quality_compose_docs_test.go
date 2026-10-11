package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
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

// composeDocUpdateRules are the update question rules every document states:
// the flags that keep the step from reading stdin and the preview flags that
// skip it.
var composeDocUpdateRules = []string{
	"Enter", "`--yes`", "`-y`", "`--auto`", "`--check`", "`CI`", "`--workspace`",
	"`--plan`", "`--preview`", "`--dry-run`",
}

// TestComposeDocs_ReadmesDocumentGroupsTableBindingAndUpdate covers the README
// half of S18: README.md in English and docs/README.ko.md in Korean carry the
// role groups, the table columns, the binding, the update question rules, the
// GAP 1 classes, and the OMP requested-route scope, in the words the step
// prints in that locale.
func TestComposeDocs_ReadmesDocumentGroupsTableBindingAndUpdate(t *testing.T) {
	t.Parallel()

	for path, lang := range map[string]string{"README.md": "en", "docs/README.ko.md": "ko"} {
		doc := flowText(readRepoFile(t, strings.Split(path, "/")...))
		for _, want := range append(composeDocGroups(lang), composeDocLabels(lang)...) {
			assert.Contains(t, doc, want, "%s is missing %q", path, want)
		}
		for _, want := range append([]string{
			composeTextFor(lang).Question,
			"`agent=tier`",
			"`quality.presets.custom`",
			"`quality.default: custom`",
			"`auto quality show --agents`",
			"`auto quality --apply`",
			// The native balanced table names the models generation writes.
			"`" + config.ClaudeSonnetModel + "`",
			"`" + config.CodexLunaModel + "`",
		}, composeDocUpdateRules...) {
			assert.Contains(t, doc, want, "%s is missing %q", path, want)
		}
	}
}

// TestComposeDocs_GeneratedSkillsTellHowToChangeAgentModels covers the
// generated-surface half of S18: the skills that explain quality modes point
// to the composition step in their locale, and every generated template says
// what its content source says, so `make generate-templates` has nothing left
// to carry.
func TestComposeDocs_GeneratedSkillsTellHowToChangeAgentModels(t *testing.T) {
	t.Parallel()

	skills := map[string]struct {
		lang  string
		paths []string
	}{
		"adaptive-quality": {lang: "en", paths: []string{
			"content/skills/adaptive-quality.md",
			"templates/codex/skills/adaptive-quality.md.tmpl",
			"templates/gemini/skills/adaptive-quality/SKILL.md.tmpl",
		}},
		"using-autopus": {lang: "ko", paths: []string{
			"content/skills/using-autopus.md",
			"templates/codex/skills/using-autopus.md.tmpl",
			"templates/gemini/skills/using-autopus/SKILL.md.tmpl",
		}},
	}
	for name, skill := range skills {
		for _, path := range skill.paths {
			doc := flowText(readRepoFile(t, strings.Split(path, "/")...))
			for _, group := range composeRoleGroups {
				assert.Contains(t, doc, group.Label(skill.lang), "%s (%s) is missing a group label", path, name)
			}
			for _, want := range []string{
				"`agent=tier`", "`quality.presets.custom`", "`auto quality show --agents`",
			} {
				assert.Contains(t, doc, want, "%s (%s) is missing %q", path, name, want)
			}
		}
	}
	pipeline := flowText(readRepoFile(t, "content", "skills", "references", "agent-pipeline", "quality-modes.md"))
	for _, want := range []string{"`auto quality`", "`agent=tier`", "regeneration overwrites"} {
		assert.Contains(t, pipeline, want, "agent-pipeline quality-modes.md is missing %q", want)
	}
}

// TestComposeDocs_ChangelogDocumentsTheFeatureAndTheWriterChange covers the
// CHANGELOG half of S18 in the Korean the CHANGELOG is written in, including
// the line writer precheck that now refuses `auto quality <preset>` and
// `auto quality supervisor` on a quality block holding a YAML reference.
func TestComposeDocs_ChangelogDocumentsTheFeatureAndTheWriterChange(t *testing.T) {
	t.Parallel()

	changelog := flowText(readRepoFile(t, "CHANGELOG.md"))
	for _, want := range append(composeDocGroups("ko"), composeDocLabels("ko")...) {
		assert.Contains(t, changelog, want, "CHANGELOG.md is missing %q", want)
	}
	for _, want := range append([]string{
		"SPEC-MODELUX-001",
		composeTextFor("ko").Question,
		"역할별 직접 구성",
		"`agent=tier`",
		"`quality.presets.custom`",
		"`auto quality show --agents`",
		"`auto quality <preset>`",
		"`auto quality supervisor`",
		errQualityWriteRefused.Error(),
	}, composeDocUpdateRules...) {
		assert.Contains(t, changelog, want, "CHANGELOG.md is missing %q", want)
	}
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
