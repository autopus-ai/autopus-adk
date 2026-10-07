package config

// SPEC-EDITGUARD-001 REQ-EG-12 configuration oracles for hooks.edit_guard.
//
// The flag defaults to enabled, and an unset key must stay unset on disk:
// config decoding rejects unknown keys, so a saved `edit_guard` line would make
// every older binary fail to load the file it shares with this one.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestHooksConf_IsEditGuardEnabled_ResolvesUnsetToTheDefault pins the three
// states the pointer carries: unset resolves to enabled, an explicit value wins.
func TestHooksConf_IsEditGuardEnabled_ResolvesUnsetToTheDefault(t *testing.T) {
	t.Parallel()

	assert.True(t, DefaultEditGuard, "the SPEC assumes the guard is on by default")
	assert.True(t, HooksConf{}.IsEditGuardEnabled(), "unset")
	assert.True(t, HooksConf{EditGuard: new(true)}.IsEditGuardEnabled(), "explicit true")
	assert.False(t, HooksConf{EditGuard: new(false)}.IsEditGuardEnabled(), "explicit false")
}

// TestLoad_EditGuardKeyDecodesUnderStrictDecoding reads the key the way a user
// writes it. Absent stays nil, so the next Save writes nothing for it.
func TestLoad_EditGuardKeyDecodesUnderStrictDecoding(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, hooks string
		set, want   bool
	}{
		{"absent", "hooks:\n  pre_commit_arch: true\n", false, true},
		{"false", "hooks:\n  pre_commit_arch: true\n  edit_guard: false\n", true, false},
		{"true", "hooks:\n  edit_guard: true\n", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := Load(writeStrictConfig(t, strictBaseConfig+c.hooks))
			require.NoError(t, err)
			assert.Equal(t, c.set, cfg.Hooks.EditGuard != nil, "set")
			assert.Equal(t, c.want, cfg.Hooks.IsEditGuardEnabled(), "enabled")
		})
	}
}

// TestLoad_MisspelledEditGuardKeyIsStillRejected keeps the new key from
// becoming an opt-out: a typo next to it must fail like any other unknown key.
func TestLoad_MisspelledEditGuardKeyIsStillRejected(t *testing.T) {
	t.Parallel()

	_, err := Load(writeStrictConfig(t, strictBaseConfig+"hooks:\n  edit_gaurd: false\n"))

	require.ErrorContains(t, err, "edit_gaurd")
	assert.ErrorContains(t, err, "unknown keys are rejected")
}

// TestDefaultFullConfig_LeavesEditGuardUnset is the fresh-project half of the
// compatibility rule: `auto init` saves DefaultFullConfig, so a default written
// as an explicit value would reach disk.
func TestDefaultFullConfig_LeavesEditGuardUnset(t *testing.T) {
	t.Parallel()

	hooks := DefaultFullConfig("guard").Hooks
	assert.Nil(t, hooks.EditGuard)
	assert.True(t, hooks.IsEditGuardEnabled())
}

// TestSave_UnsetEditGuardWritesOnlyKeysOlderBinariesKnow saves a default
// config and reads back its hooks block: it must hold exactly the keys a binary
// without hooks.edit_guard decodes, and it must still load as enabled.
func TestSave_UnsetEditGuardWritesOnlyKeysOlderBinariesKnow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, Save(dir, DefaultFullConfig("guard")))

	assert.Equal(t, []string{"pre_commit_arch", "pre_commit_lore", "react_ci_failure", "react_review"},
		savedHookKeys(t, dir))
	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.True(t, cfg.Hooks.IsEditGuardEnabled())
}

// TestSave_ExplicitEditGuardFalseSurvivesTheRoundTrip keeps a user's opt-out:
// Save re-marshals the struct, so a value that did not survive would silently
// turn the guard back on at the next `auto update`.
func TestSave_ExplicitEditGuardFalseSurvivesTheRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := DefaultFullConfig("guard")
	cfg.Hooks.EditGuard = new(false)
	require.NoError(t, Save(dir, cfg))

	assert.Contains(t, savedHookKeys(t, dir), "edit_guard")
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.NotNil(t, loaded.Hooks.EditGuard)
	assert.False(t, loaded.Hooks.IsEditGuardEnabled())
}

// savedHookKeys returns the keys of the hooks mapping in the saved autopus.yaml,
// in file order.
func savedHookKeys(t *testing.T, dir string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, configFileName))
	require.NoError(t, err)
	var doc struct {
		Hooks yaml.Node `yaml:"hooks"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	require.Equal(t, yaml.MappingNode, doc.Hooks.Kind, "hooks must be a mapping")
	keys := make([]string, 0, len(doc.Hooks.Content)/2)
	for i := 0; i+1 < len(doc.Hooks.Content); i += 2 {
		keys = append(keys, doc.Hooks.Content[i].Value)
	}
	return keys
}
