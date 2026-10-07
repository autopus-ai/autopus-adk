package codex

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

func renderedPluginVersion(t *testing.T, a *Adapter, cfg *config.HarnessConfig) string {
	t.Helper()
	doc, err := a.renderPluginManifestJSON(cfg, "router-v1")
	require.NoError(t, err)
	var manifest pluginManifest
	require.NoError(t, json.Unmarshal([]byte(doc), &manifest))
	return manifest.Version
}

// The pinned generator version replaces only the semver base; the project slug
// and cache key that follow "+" are the same as without the pin.
func TestWithPluginBaseVersion_PinsOnlyTheSemverBase(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("Alpha Project")
	unpinned := renderedPluginVersion(t, New(), cfg)
	suffix := unpinned[strings.Index(unpinned, "+"):]

	cases := map[string]string{
		"v0.50.123":       "0.50.123",
		"0.50.124":        "0.50.124",
		"v1.2.3+build.7":  "1.2.3",
		"v0.51.0-rc.1":    "0.51.0-rc.1",
		"dev":             "0.0.0-dev",
		"":                "0.0.0-dev",
		"v0.50.123-dirty": "0.50.123-dirty",
	}
	for pin, base := range cases {
		pinned := renderedPluginVersion(t, NewWithRoot(t.TempDir(), WithPluginBaseVersion(pin)), cfg)
		assert.Equal(t, base+suffix, pinned, "pin %q", pin)
	}
}

// Generation with the pin writes the pinned base into the plugin manifest on
// disk, whatever version this test binary was built with.
func TestWithPluginBaseVersion_GeneratedPluginManifestCarriesPin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a := NewWithRoot(root, WithModelCatalog(nil), WithCLIVersion("codex-cli 0.160.0"),
		WithPluginBaseVersion("v0.50.123"))
	a.codexFallbackWriter = nil

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("pin-project"))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(root, ".autopus", "plugins", "auto", ".codex-plugin", "plugin.json"))
	require.NoError(t, err)
	var manifest pluginManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	assert.Regexp(t, `^0\.50\.123\+codex\.pin-project\.[0-9a-f]{12}$`, manifest.Version)
}
