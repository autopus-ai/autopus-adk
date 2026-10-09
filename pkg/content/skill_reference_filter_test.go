package content

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The Codex agent line keeps installed skills in source order, keeps a
// project-authored name the catalog does not own, and disappears when nothing
// it named is installed.
func TestFilterSkillsReferenceLine_KeepsOnlyInstalledSkills(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("skills-reference")
	body := "intro\n- Skills reference: tdd, ddd, debugging, ast-refactoring, my-project-skill\n" +
		"- Source tool contract: Read\n- Skills reference: ddd, ast-refactoring\nend"

	got := FilterSkillsReferenceLine(body, "codex", cfg)

	assert.Equal(t, "intro\n- Skills reference: tdd, debugging, my-project-skill\n"+
		"- Source tool contract: Read\nend", got)
	assert.Equal(t, "no skills line", FilterSkillsReferenceLine("no skills line", "codex", cfg))

	cfg.Skills.Compiler.ExplicitSkills = []string{"ddd"}
	assert.Equal(t, []string{"tdd", "ddd"}, FilterInstalledSkillNames([]string{"tdd", "ddd", "ast-refactoring"}, "opencode", cfg))
}
