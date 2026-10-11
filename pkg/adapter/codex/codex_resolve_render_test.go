package codex

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/templates"
)

var codexTemplateAgentArgs = regexp.MustCompile(`\{\{with \.CodexAgentModel "([^"]+)" "([^"]*)" "([^"]*)"\}\}`)

// The composition table computes a Codex cell by calling ResolveAgentProfile
// with the run's snapshot; generation renders the agent templates with the same
// snapshot. Every rendered model and effort must equal the resolver's answer,
// and a placement the resolver refuses must stop generation.
func TestResolveAgentProfile_RenderedAgentTemplates_CarryTheSameResolution(t *testing.T) {
	t.Parallel()

	qualities := map[string]func() config.QualityConf{
		"balanced":        resolveBalancedQuality,
		"ultra":           resolveUltraQuality,
		"custom executor": resolveCustomExecutorQuality,
		"codex override":  resolveCodexOverrideQuality,
	}
	catalogs := map[string]func(*testing.T) []byte{
		"CAT-A": resolveCatalogA,
		"CAT-B": resolveCatalogB,
		"CAT-C": func(*testing.T) []byte { return nil },
		"CAT-D": resolveCatalogD,
	}
	for qualityName, qualityFor := range qualities {
		for catalogName, catalogFor := range catalogs {
			t.Run(qualityName+"/"+catalogName, func(t *testing.T) {
				t.Parallel()

				quality := qualityFor()
				snapshot := catalogFor(t)
				cfg := config.DefaultFullConfig("resolve-render")
				cfg.Quality = quality
				expected, refused := resolvedTemplateProfiles(t, quality, snapshot)

				a := NewWithRoot(t.TempDir(), WithModelCatalog(snapshot))
				a.codexFallbackWriter = nil
				files, err := a.prepareAgentFiles(cfg)

				if refused {
					require.ErrorIs(t, err, ErrCodexNativePlacementUnavailable)
					return
				}
				require.NoError(t, err)
				require.Len(t, files, len(expected))
				for _, file := range files {
					name := filepath.Base(file.TargetPath)
					want, ok := expected[name]
					require.True(t, ok, "unexpected managed agent %q", name)
					assertCodexRenderedProfile(t, string(file.Content), want)
				}
			})
		}
	}
}

// resolvedTemplateProfiles resolves every agent template's literal arguments
// through ResolveAgentProfile, keyed by the generated file name.
func resolvedTemplateProfiles(
	t *testing.T,
	quality config.QualityConf,
	snapshot []byte,
) (map[string]config.CodexProfile, bool) {
	t.Helper()
	entries, err := templates.FS.ReadDir(agentsTemplateDir)
	require.NoError(t, err)
	expected := make(map[string]config.CodexProfile, len(entries))
	refused := false
	for _, entry := range entries {
		content, err := templates.FS.ReadFile(agentsTemplateDir + "/" + entry.Name())
		require.NoError(t, err)
		args := codexTemplateAgentArgs.FindStringSubmatch(string(content))
		require.Len(t, args, 4, "%s carries no CodexAgentModel call", entry.Name())
		resolution, err := ResolveAgentProfile(quality, snapshot, args[1], args[2], args[3])
		if err != nil {
			require.ErrorIs(t, err, ErrCodexNativePlacementUnavailable)
			refused = true
			continue
		}
		expected[strings.TrimSuffix(entry.Name(), ".tmpl")] = resolution.Effective
	}
	return expected, refused
}
