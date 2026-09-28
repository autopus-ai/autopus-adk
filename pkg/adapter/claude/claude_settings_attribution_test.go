package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestCommitAttributionForbidden_FollowsLorePolicy(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("probe")
	assert.True(t, commitAttributionForbidden(cfg), "default policy forbids Co-Authored-By")

	empty := []string{}
	cfg.Lore.ForbiddenTrailers = &empty
	assert.False(t, commitAttributionForbidden(cfg), "an explicit empty list allows it")

	cfg = config.DefaultFullConfig("probe")
	cfg.Lore.Enabled = false
	assert.False(t, commitAttributionForbidden(cfg), "disabled lore leaves attribution alone")
	assert.False(t, commitAttributionForbidden(nil))
}

func mapAttribution(t *testing.T, seed string, suppress bool) map[string]any {
	t.Helper()
	root := t.TempDir()
	writeSeed(t, root, seed)
	a := NewWithRoot(root)
	a.suppressCommitAttribution = suppress
	mapping, err := a.prepareSettingsMapping(nil, nil)
	require.NoError(t, err)
	return decodeSettings(t, mapping.Content)
}

func TestPrepareSettingsMapping_SuppressesOnlyCommitAttribution(t *testing.T) {
	t.Parallel()

	settings := mapAttribution(t, `{"attribution":{"pr":"custom","sessionUrl":false}}`, true)
	assert.Equal(t, map[string]any{"commit": "", "pr": "custom", "sessionUrl": false},
		settings["attribution"])

	settings = mapAttribution(t, `{"model":"opus"}`, true)
	assert.Equal(t, map[string]any{"commit": ""}, settings["attribution"])
	assert.Equal(t, "opus", settings["model"])
}

func TestPrepareSettingsMapping_KeepsAttributionThatAlreadyHidesAll(t *testing.T) {
	t.Parallel()

	settings := mapAttribution(t, `{"attribution":false}`, true)
	assert.Equal(t, false, settings["attribution"])
}

func TestPrepareSettingsMapping_LeavesAttributionWhenAllowed(t *testing.T) {
	t.Parallel()

	settings := mapAttribution(t, `{"attribution":{"commit":"Custom trailer"}}`, false)
	assert.Equal(t, map[string]any{"commit": "Custom trailer"}, settings["attribution"])

	settings = mapAttribution(t, `{}`, false)
	assert.NotContains(t, settings, "attribution")
}

func writeSeed(t *testing.T, root, seed string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(seed), 0o644))
}

func decodeSettings(t *testing.T, content []byte) map[string]any {
	t.Helper()
	var settings map[string]any
	require.NoError(t, json.Unmarshal(content, &settings))
	return settings
}
