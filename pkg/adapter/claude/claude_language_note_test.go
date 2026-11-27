package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func countNoteHandlers(t *testing.T, root string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	return strings.Count(string(data), "auto verify language --note")
}

// The note check is one handler however often update runs, and switching
// the language check off removes it.
func TestUpdate_LanguageNoteHookStaysSingleAndSwitchesOff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("note")
	cfg.Language.AIResponses = "ko"
	a := NewWithRoot(root)
	_, err := a.Generate(ctx, cfg)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, err = a.Update(ctx, cfg)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, countNoteHandlers(t, root))

	off := false
	cfg.Hooks.LanguageStop = &off
	_, err = a.Update(ctx, cfg)
	require.NoError(t, err)
	assert.Zero(t, countNoteHandlers(t, root), "hooks.language_stop: false removes it")
	var doc map[string]any
	data, _ := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, json.Unmarshal(data, &doc))
}
