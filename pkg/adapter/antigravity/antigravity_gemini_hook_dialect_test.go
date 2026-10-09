package antigravity

// The legacy Gemini CLI entries in .gemini/settings.json moved from the Claude
// matcher "Bash" with second timeouts, which Gemini CLI never ran, to
// ^run_shell_command$ with millisecond timeouts. A settings file the previous
// release wrote must converge on the corrected entries without keeping the old
// ones beside them.

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

const (
	dialectArchCommand  = "auto check --hygiene --arch --quiet --staged --warn-only"
	dialectReactCommand = "auto react check --quiet"
)

// writePreviousReleaseGeminiSettings generates the surface and rewrites the
// shell-tool entries of .gemini/settings.json to the shapes the previous
// release wrote, then adds a user handler under the same matcher.
func writePreviousReleaseGeminiSettings(t *testing.T, a *Adapter, cfg *config.HarnessConfig) string {
	t.Helper()
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	path := filepath.Join(a.root, ".gemini", "settings.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	previous := strings.NewReplacer(
		`"matcher": "^run_shell_command$"`, `"matcher": "Bash"`,
		`"timeout": 30000`, `"timeout": 30`,
		`"timeout": 60000`, `"timeout": 60`,
	).Replace(string(raw))
	require.NotEqual(t, string(raw), previous, "the generated settings carry the corrected shapes")

	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(previous), &settings))
	hooks := settings["hooks"].(map[string]any)
	hooks["BeforeTool"] = append(hooks["BeforeTool"].([]any), map[string]any{
		"matcher": "Bash",
		"hooks":   []any{map[string]any{"type": "command", "command": "user-check", "timeout": 17}},
	})
	data, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// geminiHandlers returns every handler of the event as "matcher timeout command".
func geminiHandlers(t *testing.T, path, event string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var settings struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(raw, &settings))
	var handlers []string
	for _, entry := range settings.Hooks[event] {
		for _, handler := range entry.Hooks {
			if strings.HasPrefix(handler.Command, "out=$(auto guard edit") {
				continue
			}
			handlers = append(handlers, entry.Matcher+" "+itoa(handler.Timeout)+" "+handler.Command)
		}
	}
	return handlers
}

func itoa(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}

func TestUpdate_ReplacesPreviousReleaseGeminiShellHookEntries(t *testing.T) {
	root := t.TempDir()
	a := NewWithRoot(root)
	cfg := config.DefaultFullConfig("gemini-dialect")
	path := writePreviousReleaseGeminiSettings(t, a, cfg)

	for range 2 {
		_, err := a.Update(context.Background(), cfg)
		require.NoError(t, err)
	}

	assert.ElementsMatch(t, []string{
		"^run_shell_command$ 30000 " + dialectArchCommand,
		"Bash 17 user-check",
	}, geminiHandlers(t, path, "BeforeTool"), "the retired arch entry is replaced and the user handler stays once")
	assert.Equal(t, []string{"^run_shell_command$ 60000 " + dialectReactCommand},
		geminiHandlers(t, path, "AfterTool"), "the retired react entry is replaced, not kept beside the new one")
}

func TestClean_RemovesPreviousReleaseGeminiShellHookEntries(t *testing.T) {
	root := t.TempDir()
	a := NewWithRoot(root)
	path := writePreviousReleaseGeminiSettings(t, a, config.DefaultFullConfig("gemini-dialect"))

	require.NoError(t, a.Clean(context.Background()))

	assert.Equal(t, []string{"Bash 17 user-check"}, geminiHandlers(t, path, "BeforeTool"))
	assert.Empty(t, geminiHandlers(t, path, "AfterTool"))
}
