package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// nativeBalancedCodexTOML is the model/effort pair each managed agent file must
// carry under the standard native balanced placement.
var nativeBalancedCodexTOML = map[string]config.CodexProfile{
	"architect.toml":        {Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
	"debugger.toml":         {Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
	"deep-worker.toml":      {Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
	"planner.toml":          {Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
	"reviewer.toml":         {Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
	"security-auditor.toml": {Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
	"spec-writer.toml":      {Model: config.CodexAstraModel, Effort: config.CodexEffortMax},

	"annotator.toml":           {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
	"devops.toml":              {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
	"executor.toml":            {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
	"explorer.toml":            {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
	"frontend-specialist.toml": {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
	"perf-engineer.toml":       {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
	"tester.toml":              {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
	"ux-validator.toml":        {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
	"validator.toml":           {Model: config.CodexLunaModel, Effort: config.CodexEffortMax},
}

func TestPrepareAgentFiles_NativeBalancedRendersPolicyPlacement(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	useFullCodexCatalogForTest(a)
	var warnings bytes.Buffer
	a.codexFallbackWriter = &warnings

	files, err := a.prepareAgentFiles(config.DefaultFullConfig("native-balanced"))
	require.NoError(t, err)
	require.Len(t, files, len(nativeBalancedCodexTOML))

	for _, file := range files {
		name := filepath.Base(file.TargetPath)
		want, ok := nativeBalancedCodexTOML[name]
		require.True(t, ok, "unexpected managed agent %q", name)
		assertCodexRenderedProfile(t, string(file.Content), want)
	}
	assert.Empty(t, warnings.String(), "a catalog that advertises the placement needs no diagnostic")
}

// The catalog probe fails on an unauthenticated or offline workstation. The
// placement still lands verbatim there — silently installing a weaker model
// would be worse than installing an unverified one — but the operator has to
// learn the profile was never confirmed.
func TestPrepareAgentFiles_NativeBalancedCatalogUnknownKeepsExactProfile(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	a.codexCatalogProbed = true
	a.codexCatalogJSON = nil
	var warnings bytes.Buffer
	a.codexFallbackWriter = &warnings

	files, err := a.prepareAgentFiles(config.DefaultFullConfig("offline-balanced"))
	require.NoError(t, err)

	for _, file := range files {
		name := filepath.Base(file.TargetPath)
		assertCodexRenderedProfile(t, string(file.Content), nativeBalancedCodexTOML[name])
	}

	report := warnings.String()
	assert.Contains(t, report, string(config.CodexResolutionCatalogUnknown))
	assert.Contains(t, report, config.CodexAstraModel+"/"+config.CodexEffortMax)
	assert.Contains(t, report, config.CodexLunaModel+"/"+config.CodexEffortMax)
	// Two distinct profiles across sixteen agents, each reported once.
	assert.Equal(t, 2, strings.Count(report, "\n"))
}

func TestPrepareAgentFiles_NativeBalancedRefusesCatalogSubstitution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		catalog string
	}{
		{
			// Luna is present but tops out below the placement's effort.
			name: "effort below placement",
			catalog: `{"models":[
				{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"max"}]},
				{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"medium"},{"effort":"high"}]},
				{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
			]}`,
		},
		{
			// Luna is absent, so the ordered fallback would reach for a weaker
			// model the placement never named.
			name: "model absent",
			catalog: `{"models":[
				{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"max"}]},
				{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"max"}]},
				{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
			]}`,
		},
		{
			// Nothing in the catalog resembles the placement at all.
			name: "no compatible model",
			catalog: `{"models":[
				{"slug":"other-model","supported_reasoning_levels":[{"effort":"medium"}]}
			]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := NewWithRoot(t.TempDir())
			a.codexCatalogProbed = true
			a.codexCatalogJSON = []byte(tt.catalog)
			a.codexFallbackWriter = nil

			_, err := a.prepareAgentFiles(config.DefaultFullConfig("rejected-balanced"))
			require.ErrorIs(t, err, ErrCodexNativePlacementUnavailable)
		})
	}
}

// A rejected placement has to stop the whole install, not leave a half-written
// surface behind for the next run to inherit.
func TestGenerate_NativeBalancedCatalogRejectionWritesNothing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	a := NewWithRoot(root, WithModelCatalog([]byte(
		`{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"max"}]}]}`,
	)))
	a.codexFallbackWriter = nil

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("blocked-install"))
	require.ErrorIs(t, err, ErrCodexNativePlacementUnavailable)

	_, statErr := os.Stat(filepath.Join(root, ".codex"))
	assert.True(t, os.IsNotExist(statErr), "no Codex surface may exist after a rejected placement")
}

// Ultra and hand-edited tiers are preferences resolved against the catalog, not
// a named placement, so they keep the ordered availability fallback.
func TestPrepareAgentFiles_NonPlacementProfilesKeepCatalogFallback(t *testing.T) {
	t.Parallel()

	legacyOnly := []byte(`{"models":[
		{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"max"}]},
		{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"max"}]},
		{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
	]}`)

	t.Run("ultra", func(t *testing.T) {
		t.Parallel()

		a := NewWithRoot(t.TempDir())
		a.codexCatalogProbed = true
		a.codexCatalogJSON = legacyOnly
		a.codexFallbackWriter = nil
		cfg := config.DefaultFullConfig("ultra-fallback")
		cfg.Quality.Default = "ultra"

		files, err := a.prepareAgentFiles(cfg)
		require.NoError(t, err)
		assertCodexRenderedProfile(t,
			codexAgentMappingContent(t, files, "executor.toml"),
			config.CodexProfile{Model: config.CodexLegacyModel, Effort: config.CodexEffortXHigh},
		)
	})

	t.Run("custom tier", func(t *testing.T) {
		t.Parallel()

		a := NewWithRoot(t.TempDir())
		a.codexCatalogProbed = true
		a.codexCatalogJSON = legacyOnly
		a.codexFallbackWriter = nil
		cfg := config.DefaultFullConfig("custom-tier-fallback")
		cfg.Quality.Presets["balanced"] = withCodexAgentTier(cfg.Quality.Presets["balanced"], "executor", "opus")

		files, err := a.prepareAgentFiles(cfg)
		require.NoError(t, err)
		assertCodexRenderedProfile(t,
			codexAgentMappingContent(t, files, "executor.toml"),
			config.CodexProfile{Model: config.CodexLegacyModel, Effort: config.CodexEffortXHigh},
		)
		// The sibling keeps the placement, which the catalog still advertises.
		assertCodexRenderedProfile(t,
			codexAgentMappingContent(t, files, "tester.toml"),
			nativeBalancedCodexTOML["tester.toml"],
		)
	})
}

// The root profile is not part of the agent placement, so a managed supervisor
// keeps its own lenient resolution even while the agents refuse substitution.
func TestPrepareConfigFile_SupervisorKeepsCatalogFallback(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	a.codexCatalogProbed = true
	a.codexCatalogJSON = []byte(`{"models":[{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`)
	a.codexFallbackWriter = nil
	cfg := config.DefaultFullConfig("supervisor-fallback")
	cfg.Quality.SupervisorModelPolicy = config.SupervisorModelPolicyQuality

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, `model = "`+config.CodexLegacyModel+`"`)
	assert.Contains(t, root, `model_reasoning_effort = "`+config.CodexEffortXHigh+`"`)
}

// A user's own root model survives the placement pass. Agents are managed; the
// root under the default inherit policy is the user's to choose.
func TestPrepareFiles_NativeBalancedPreservesUserRootModel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	a := NewWithRoot(root)
	useFullCodexCatalogForTest(a)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".codex"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".codex", "config.toml"),
		[]byte("model = \"user/private-model\"\nmodel_reasoning_effort = \"low\"\n"),
		0o644,
	))

	files, err := a.prepareFiles(config.DefaultFullConfig("preserve-root"))
	require.NoError(t, err)

	byPath := make(map[string]string, len(files))
	for _, file := range files {
		byPath[file.TargetPath] = string(file.Content)
	}
	rootConfig := strings.SplitN(byPath[codexConfigRelPath], "[agents]", 2)[0]
	assert.Contains(t, rootConfig, `model = "user/private-model"`)
	assert.Contains(t, rootConfig, `model_reasoning_effort = "low"`)
	assertCodexRenderedProfile(t,
		byPath[filepath.Join(".codex", "agents", "executor.toml")],
		nativeBalancedCodexTOML["executor.toml"],
	)
}

// withCodexAgentTier returns a detached copy of preset whose role map differs
// from the shipped default in exactly one entry.
func withCodexAgentTier(preset config.QualityPreset, agent, tier string) config.QualityPreset {
	agents := make(map[string]string, len(preset.Agents)+1)
	for name, value := range preset.Agents {
		agents[name] = value
	}
	agents[agent] = tier
	preset.Agents = agents
	return preset
}
