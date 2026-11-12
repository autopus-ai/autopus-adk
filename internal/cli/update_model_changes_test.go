package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// modelChangeProject writes a settled autopus.yaml from cfg and returns its
// directory with the config loaded back.
func modelChangeProject(t *testing.T, mutate func(*config.HarnessConfig)) (string, *config.HarnessConfig) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("demo")
	mutate(cfg)
	require.NoError(t, config.Save(dir, cfg))
	loaded, err := config.Load(dir)
	require.NoError(t, err)
	return dir, loaded
}

func lineAnswerCmd(t *testing.T, answer string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	previous := composeFormsProbe
	composeFormsProbe = func(*cobra.Command) bool { return false }
	t.Cleanup(func() { composeFormsProbe = previous })
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader(answer))
	return cmd, &out
}

// An ultra project still on the shipped Opus review model, with the old
// review list: update names both moves, and y applies them in the file.
func TestUpdateModelChanges_NamesWhatChangedAndAppliesOnYes(t *testing.T) {
	dir, cfg := modelChangeProject(t, func(cfg *config.HarnessConfig) {
		cfg.Quality.Default = "ultra"
		cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
		cfg.Orchestra.Commands["review"] = config.CommandEntry{Strategy: "debate", Providers: []string{"claude", "codex", "gemini"}}
	})
	cmd, out := lineAnswerCmd(t, "y\n")

	require.NoError(t, runUpdateModelChanges(cmd, dir, cfg, "en"))

	assert.Equal(t, "  Model defaults changed:\n"+
		"    - orchestra review providers claude, codex, gemini → review gate list claude, codex (spec.review_gate.providers)\n"+
		"    - claude review model claude-opus-5-5 · max → claude-fable-5-1 · max (the preset's reviewer model)\n"+
		"  Apply these changes? [y/N]   Applied.\n", out.String())
	reloaded, err := config.Load(dir)
	require.NoError(t, err)
	model, _ := config.ClaudeProviderModel(reloaded.Orchestra.Providers["claude"])
	assert.Equal(t, config.ClaudeFableModel, model)
	assert.Equal(t, []string{"claude", "codex"}, config.ReviewGateOrchestraView(reloaded).Commands["review"].Providers)

	again, _ := lineAnswerCmd(t, "")
	var quiet bytes.Buffer
	again.SetOut(&quiet)
	require.NoError(t, runUpdateModelChanges(again, dir, reloaded, "en"))
	assert.Empty(t, quiet.String(), "nothing left to ask")
}

// Any other answer keeps the file byte for byte and points at the editor.
func TestUpdateModelChanges_KeepsTheFileWithoutYes(t *testing.T) {
	dir, cfg := modelChangeProject(t, func(cfg *config.HarnessConfig) { cfg.Quality.Default = "ultra" })
	before, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	cmd, out := lineAnswerCmd(t, "\n")

	require.NoError(t, runUpdateModelChanges(cmd, dir, cfg, "ko"))

	assert.Contains(t, out.String(), "claude 리뷰 model claude-opus-5-5 · max → claude-fable-5-1 · max")
	assert.Contains(t, out.String(), "그대로 둡니다. 모델 전체 편집: auto update --configure-models")
	after, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

// A balanced project on the defaults has nothing to say.
func TestUpdateModelChanges_SilentOnCurrentDefaults(t *testing.T) {
	dir, cfg := modelChangeProject(t, func(*config.HarnessConfig) {})
	cmd, out := lineAnswerCmd(t, "y\n")
	require.NoError(t, runUpdateModelChanges(cmd, dir, cfg, "en"))
	assert.Empty(t, out.String())
	var notice bytes.Buffer
	assert.False(t, writeUpdateModelNotice(&notice, dir, cfg, "en"))
	assert.Empty(t, notice.String())
}

// The same CLIs either way: the line says only the duplicate list goes.
func TestUpdateModelChanges_SameReviewCLIsNameTheDuplicateOnly(t *testing.T) {
	dir, cfg := modelChangeProject(t, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Providers = []string{"claude", "codex", "gemini"}
		cfg.Orchestra.Commands["review"] = config.CommandEntry{Strategy: "debate", Providers: []string{"gemini", "claude", "codex"}}
	})
	var out bytes.Buffer
	assert.True(t, writeUpdateModelNotice(&out, dir, cfg, "en"))
	assert.Contains(t, out.String(), "    - orchestra review drops its own provider list and follows the review gate list "+
		"claude, codex, gemini (spec.review_gate.providers, same CLIs)\n")
}

// Non-interactive runs only name the changes.
func TestWriteUpdateModelNotice_NamesWithoutWriting(t *testing.T) {
	dir, cfg := modelChangeProject(t, func(cfg *config.HarnessConfig) { cfg.Quality.Default = "ultra" })
	var out bytes.Buffer
	assert.True(t, writeUpdateModelNotice(&out, dir, cfg, "en"))
	assert.Equal(t, "  Note: model changes are available; run auto update in a terminal to apply them:\n"+
		"    - claude review model claude-opus-5-5 · max → claude-fable-5-1 · max (the preset's reviewer model)\n", out.String())
}
