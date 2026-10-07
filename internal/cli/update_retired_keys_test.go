package cli

// SPEC-PANERM-001 T13 (REQ-10, S7 b): `auto update` removes the retired
// orchestra keys from autopus.yaml. Without another migration it cuts them
// from the raw file, so comments and reserved blocks stay; with one, the
// migration's config.Save drops them. Either way it names them once, and
// `--plan` announces the rewrite without writing.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const c1RetiredLine = "  - removed retired orchestra keys from autopus.yaml: features.cc21.monitor_pattern_timeout_ms, " +
	"orchestra.providers.claude.pane_args, orchestra.providers.codex.pane_args, " +
	"orchestra.providers.gemini.interactive_input\n"

// withoutRetiredKeyLines drops every line that holds one of C1's group K keys;
// it reads the text only, independent of the node walk under test.
func withoutRetiredKeyLines(data string) string {
	var kept []string
	for _, line := range strings.SplitAfter(data, "\n") {
		key, _, _ := strings.Cut(strings.TrimSpace(line), ":")
		switch key {
		case "pane_args", "interactive_input", "working_patterns", "monitor_pattern_timeout_ms":
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "")
}

func runRetiredKeyUpdate(t *testing.T, dir string, extra ...string) string {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"update", "--dir", dir, "--yes"}, extra...))
	require.NoError(t, root.Execute(), out.String())
	return out.String()
}

func isolateUpdateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
}

func TestUpdate_CutsRetiredKeysOnceWhenNoOtherMigrationHolds(t *testing.T) {
	isolateUpdateEnv(t)
	input := readLegacyPane(t, "c1.yaml")
	dir := legacyPaneDir(t, input)
	path := filepath.Join(dir, "autopus.yaml")

	first := runRetiredKeyUpdate(t, dir)
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(first, c1RetiredLine), "first update:\n%s", first)
	assert.Equal(t, withoutRetiredKeyLines(string(input)), string(written), "only the group K lines go")

	second := runRetiredKeyUpdate(t, dir)
	again, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, second, "removed retired orchestra keys")
	assert.Equal(t, string(written), string(again), "a second update leaves autopus.yaml byte-identical")
}

func TestUpdate_ReportsRetiredKeysThatAMigrationSaveDrops(t *testing.T) {
	isolateUpdateEnv(t)
	// An unlisted provider is a migration condition: update adds it to every
	// command and writes the file through config.Save.
	input := strings.Replace(string(readLegacyPane(t, "c1.yaml")), "    commands:\n",
		"        extra:\n            binary: extra\n            pane_args: [--x]\n    commands:\n", 1)
	dir := legacyPaneDir(t, []byte(input))

	out := runRetiredKeyUpdate(t, dir)
	written, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	requireNoRetiredKeys(t, written)
	assert.Contains(t, string(written), "providers: [claude, codex, gemini, extra]", "the migration ran")
	assert.Equal(t, 1, strings.Count(out, "  - removed retired orchestra keys from autopus.yaml: "+
		"features.cc21.monitor_pattern_timeout_ms, orchestra.providers.claude.pane_args, "+
		"orchestra.providers.codex.pane_args, orchestra.providers.extra.pane_args, "+
		"orchestra.providers.gemini.interactive_input\n"), out)
}

func TestUpdatePreview_AnnouncesTheRetiredKeyRemovalWithoutWriting(t *testing.T) {
	isolateUpdateEnv(t)
	input := readLegacyPane(t, "c1.yaml")
	dir := legacyPaneDir(t, input)

	out := runRetiredKeyUpdate(t, dir, "--plan")
	assert.Contains(t, out, "retired orchestra keys would be removed from autopus.yaml: "+
		"features.cc21.monitor_pattern_timeout_ms, orchestra.providers.claude.pane_args, "+
		"orchestra.providers.codex.pane_args, orchestra.providers.gemini.interactive_input")
	written, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.Equal(t, input, written, "a preview writes nothing")

	clean := legacyPaneDir(t, []byte(withoutRetiredKeyLines(string(input))))
	assert.NotContains(t, runRetiredKeyUpdate(t, clean, "--plan"), "retired orchestra keys")
}
