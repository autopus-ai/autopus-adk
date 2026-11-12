package claude

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// An install that predates the ledger holds the whole block the generator
// wrote; update retires the block and keeps every entry the user added.
func TestUpdate_RetiresFingerprintedLegacyDefaultAllows(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	settingsPath := filepath.Join(root, ".claude", "settings.json")
	writeJSONFile(t, settingsPath, map[string]any{"permissions": map[string]any{
		"allow": []any{"Bash(auto *)", "Bash(auto:*)", "Bash(git *)", "Bash(git:*)", "Bash(make:*)",
			"Bash(find:*)", "Bash(gh:*)", "UserAllow", "Bash(npx *)"},
	}})

	_, err := NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("retired"))
	require.NoError(t, err)

	allow := permissionValues(readJSONObject(t, settingsPath), "allow")
	for _, retired := range []string{"Bash(auto *)", "Bash(auto:*)", "Bash(git *)", "Bash(git:*)",
		"Bash(make:*)", "Bash(find:*)", "Bash(gh:*)"} {
		assert.NotContains(t, allow, retired)
	}
	assert.Contains(t, allow, "UserAllow")
	assert.Contains(t, allow, "Bash(npx *)", "a lone retired entry reads as hand-added")
	assert.Contains(t, allow, "Bash(git status:*)")
}

// A retired entry the ledger claims leaves settings.json and the ledger on
// update; an unclaimed copy of another retired entry stays with the user.
func TestUpdate_RetiresLedgerClaimedDefaultAllows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("retired")
	cfg.Hooks.Permissions.ExtraAllow = []string{"Bash(gh:*)"}
	_, err := NewWithRoot(root).Generate(ctx, cfg)
	require.NoError(t, err)
	settingsPath := filepath.Join(root, ".claude", "settings.json")
	settings := readJSONObject(t, settingsPath)
	permissions := settings["permissions"].(map[string]any)
	permissions["allow"] = append(permissions["allow"].([]any), "Bash(git *)")
	writeJSONFile(t, settingsPath, settings)

	cfg.Hooks.Permissions.ExtraAllow = nil
	_, err = NewWithRoot(root).Update(ctx, cfg)
	require.NoError(t, err)

	allow := permissionValues(readJSONObject(t, settingsPath), "allow")
	assert.NotContains(t, allow, "Bash(gh:*)")
	assert.Contains(t, allow, "Bash(git *)")
	ledger := readJSONObject(t, filepath.Join(root, filepath.FromSlash(claudePermissionLedgerPath)))
	assert.NotContains(t, toStringSlice(ledger["allow"]), "Bash(gh:*)")
	assert.NotContains(t, toStringSlice(ledger["allow"]), "Bash(git *)")
}

// The user's ExtraAllow keeps a retired entry in place.
func TestUpdate_ExtraAllowKeepsRetiredEntry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("retired")
	cfg.Hooks.Permissions.ExtraAllow = []string{"Bash(gh:*)"}
	_, err := NewWithRoot(root).Generate(ctx, cfg)
	require.NoError(t, err)
	_, err = NewWithRoot(root).Update(ctx, cfg)
	require.NoError(t, err)

	allow := permissionValues(readJSONObject(t, filepath.Join(root, ".claude", "settings.json")), "allow")
	assert.Contains(t, allow, "Bash(gh:*)")
}
