package cli

// SPEC-MODELUX-002 T7 (REQ-MUX2-07, REQ-MUX2-14, REQ-MUX2-16; acceptance
// S12, review F2): init's composition forms write a tool tab's binding into
// the config init saves and generates from the run's one catalog probe, and
// Ctrl+C cancels init before it writes any file.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestInitComposeForms_InterruptAtTheToolTabCancelsInit(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	stubInitWizard(t, tui.QualityRoleGroups)
	abortComposeFormAt(t, 2)

	dir, out, err := runComposeInit(t, formInput("2\n"))

	require.NoError(t, err, out)
	assert.Contains(t, out, "중단했습니다. 구성을 더 바꾸지 않습니다.\n")
	assert.Contains(t, out, "init cancelled.\n")
	for _, path := range []string{"autopus.yaml", "DESIGN.md", ".claude", ".codex"} {
		assert.NoFileExists(t, filepath.Join(dir, path))
		assert.NoDirExists(t, filepath.Join(dir, path))
	}
}

// The role-groups choice opens the forms; a Codex tab edit with 설계·판단 on
// opus lands in the autopus.yaml init saves as codex-custom, and the same
// run generates it from the catalog its table read, once.
func TestInitComposeForms_CodexTabEditIsSavedAndGenerated(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	stubInitWizard(t, tui.QualityRoleGroups)
	useAccessibleComposeForms(t)

	dir, out, err := runComposeInit(t, formInput(composeFormLines(composeFormS5, "2\n", "\n")...))

	require.NoError(t, err, out)
	assert.Contains(t, out, composeFormTabD)
	saved := composeBindLoad(t, dir).Quality
	assert.Equal(t, "codex-custom", saved.Providers["codex"])
	assert.Equal(t, "opus", saved.Presets["codex-custom"].Agents["planner"])
	assert.Equal(t, config.CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"}, readComposeCodexProfile(t, dir, "planner"))
	assert.Equal(t, composeTableClaudeCell{model: "claude-fable-5-1", effort: "max"}, readComposeTableClaudeProfile(t, dir, "planner"))
	assert.Len(t, *run.probes, 1)
	assert.False(t, run.adapterProbed(t))
}
