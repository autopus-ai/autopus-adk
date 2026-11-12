package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// settledUpdateConfigDir writes a default autopus.yaml and lets the update
// migrations reach their fixed point, so a later run holds only what a test
// adds.
func settledUpdateConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, config.DefaultFullConfig("demo")))
	for range 2 {
		cfg, err := config.Load(dir)
		require.NoError(t, err)
		require.NoError(t, persistUpdateConfigMigrations(&bytes.Buffer{}, dir, cfg, false))
	}
	return dir
}

// insertUnderCommands adds raw YAML lines right after `  commands:`.
func insertUnderCommands(t *testing.T, dir, lines string) {
	t.Helper()
	path := filepath.Join(dir, "autopus.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	require.Contains(t, text, "\n    commands:\n")
	text = strings.Replace(text, "\n    commands:\n", "\n    commands:\n"+lines, 1)
	require.NoError(t, os.WriteFile(path, []byte(text), 0o644))
}

func runUpdateMigrations(t *testing.T, dir string) (string, string) {
	t.Helper()
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, persistUpdateConfigMigrations(&out, dir, cfg, false))
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	return out.String(), string(data)
}

// `auto update` drops the plan and brainstorm defaults earlier releases
// wrote, names them on one line, and keeps every other byte of the file.
func TestUpdate_RemovesRetiredCommandDefaultsAndKeepsComments(t *testing.T) {
	dir := settledUpdateConfigDir(t)
	insertUnderCommands(t, dir, "        # keep this comment\n"+
		"        plan:\n            strategy: consensus\n            providers: [claude, codex, gemini]\n"+
		"        brainstorm:\n            strategy: debate\n            providers: [claude, codex, gemini]\n")

	out, data := runUpdateMigrations(t, dir)

	assert.Equal(t, "  - removed retired orchestra command defaults from autopus.yaml: "+
		"orchestra.commands.brainstorm, orchestra.commands.plan\n", out)
	assert.NotContains(t, data, "plan:")
	assert.NotContains(t, data, "brainstorm:")
	assert.Contains(t, data, "# keep this comment")

	again, _ := runUpdateMigrations(t, dir)
	assert.Empty(t, again, "a second run has nothing to remove")
}

// A plan entry the operator changed is theirs and stays.
func TestUpdate_KeepsACustomizedRetiredCommand(t *testing.T) {
	dir := settledUpdateConfigDir(t)
	insertUnderCommands(t, dir, "        plan:\n            strategy: consensus\n            providers: [claude]\n")

	out, data := runUpdateMigrations(t, dir)

	assert.Empty(t, out)
	assert.Contains(t, data, "plan:")
}
