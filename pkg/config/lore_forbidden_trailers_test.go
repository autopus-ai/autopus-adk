package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

func writeLoreConfig(t *testing.T, lore string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(
		"mode: full\nproject_name: probe\nplatforms:\n  - claude-code\nlore:\n"+lore,
	), 0o600))
	return dir
}

func TestLoreForbiddenTrailers_OmittedKeyUsesDefault(t *testing.T) {
	t.Parallel()

	loaded, err := config.LoadLoreSection(writeLoreConfig(t, "  enabled: true\n"))
	require.NoError(t, err)
	assert.Nil(t, loaded.ForbiddenTrailers)
	assert.Equal(t, config.DefaultForbiddenTrailers, loaded.EffectiveForbiddenTrailers())
	assert.True(t, loaded.ForbidsTrailer("co-authored-by"))

	missing, err := config.LoadLoreSection(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, config.DefaultForbiddenTrailers, missing.EffectiveForbiddenTrailers())
}

func TestLoreForbiddenTrailers_ExplicitEmptyListDisablesCheck(t *testing.T) {
	t.Parallel()

	loaded, err := config.LoadLoreSection(writeLoreConfig(t, "  forbidden_trailers: []\n"))
	require.NoError(t, err)
	assert.Empty(t, loaded.EffectiveForbiddenTrailers())
	assert.False(t, loaded.ForbidsTrailer("Co-Authored-By"))
}

func TestLoreForbiddenTrailers_CustomListReplacesDefault(t *testing.T) {
	t.Parallel()

	dir := writeLoreConfig(t, "  forbidden_trailers: [Signed-off-by]\n")
	loaded, err := config.LoadLoreSection(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"Signed-off-by"}, loaded.EffectiveForbiddenTrailers())
	assert.False(t, loaded.ForbidsTrailer("Co-Authored-By"))

	full, err := config.LoadPreview(dir)
	require.NoError(t, err, "strict decoding must accept the new key")
	assert.Equal(t, []string{"Signed-off-by"}, full.Lore.EffectiveForbiddenTrailers())
}

// Older binaries decode autopus.yaml strictly, so a config that never set the
// key must not gain it on save; an explicit empty list must survive a save.
func TestLoreForbiddenTrailers_OnlyExplicitValuesAreSerialized(t *testing.T) {
	t.Parallel()

	omitted, err := yaml.Marshal(config.DefaultFullConfig("probe").Lore)
	require.NoError(t, err)
	assert.NotContains(t, string(omitted), "forbidden_trailers")

	empty := []string{}
	explicit, err := yaml.Marshal(config.LoreConf{Enabled: true, ForbiddenTrailers: &empty})
	require.NoError(t, err)
	assert.Contains(t, string(explicit), "forbidden_trailers: []")
}
