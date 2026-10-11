package cli

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

func TestLoadComposeAgentSources_EmbeddedContent_CoversEveryCanonicalAgent(t *testing.T) {
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)

	require.Len(t, sources, len(config.CanonicalAgentNames()))
	for _, agent := range config.CanonicalAgentNames() {
		source, ok := sources[agent]
		require.True(t, ok, "canonical agent %q has no embedded source", agent)
		_, valid := config.NormalizeQualityTier(source.FallbackTier)
		assert.True(t, valid, "%s fallback tier %q", agent, source.FallbackTier)
	}
	// Known values from content/agents/*.md frontmatter.
	assert.Equal(t, composeAgentSource{FallbackTier: "opus", CodexEffort: "max"}, sources["planner"])
	assert.Equal(t, composeAgentSource{FallbackTier: "opus", CodexEffort: "max"}, sources["deep-worker"])
	assert.Equal(t, composeAgentSource{FallbackTier: "sonnet", CodexEffort: "high"}, sources["reviewer"])
	assert.Equal(t, composeAgentSource{FallbackTier: "sonnet", CodexEffort: "medium"}, sources["annotator"])
}

var (
	composeTemplateModelArgs  = regexp.MustCompile(`\{\{with \.CodexAgentModel "([^"]+)" "([^"]*)" "([^"]*)"\}\}`)
	composeTemplateEffortArgs = regexp.MustCompile(`\{\{with \.CodexAgentEffort "([^"]+)" "([^"]*)" "([^"]*)"\}\}`)
)

// The table reads fallback tiers and Codex efforts from content while Codex
// generation reads the literals generate-templates wrote into the agent
// templates. Both must name the same agents with the same arguments.
func TestLoadComposeAgentSources_CodexAgentTemplates_CarryTheSameLiterals(t *testing.T) {
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)

	entries, err := templates.FS.ReadDir("codex/agents")
	require.NoError(t, err)
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		agent := strings.TrimSuffix(entry.Name(), ".toml.tmpl")
		content, err := templates.FS.ReadFile("codex/agents/" + entry.Name())
		require.NoError(t, err)
		source, ok := sources[agent]
		require.True(t, ok, "template %s has no embedded agent source", entry.Name())
		want := []string{agent, source.FallbackTier, source.CodexEffort}
		for _, call := range []*regexp.Regexp{composeTemplateModelArgs, composeTemplateEffortArgs} {
			args := call.FindStringSubmatch(string(content))
			require.Len(t, args, 4, "%s lacks %s", entry.Name(), call)
			assert.Equal(t, want, args[1:], "%s literal arguments drifted from content/agents", entry.Name())
		}
		seen[agent] = true
	}
	for agent := range sources {
		assert.True(t, seen[agent], "agent source %q has no Codex template", agent)
	}
}

// Claude reads the fallback tier as the raw text after "model:" in the agent
// frontmatter; the table reads the decoded YAML value. They must agree.
func TestLoadComposeAgentSources_ClaudeFrontmatterModel_MatchesFallbackTier(t *testing.T) {
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)

	for agent, source := range sources {
		data, err := contentfs.FS.ReadFile("agents/" + agent + ".md")
		require.NoError(t, err)
		lines := strings.Split(string(data), "\n")
		require.Equal(t, "---", lines[0], "%s has no frontmatter", agent)
		claudeModel := ""
		for _, line := range lines[1:] {
			if line == "---" {
				break
			}
			if value, ok := strings.CutPrefix(line, "model:"); ok {
				claudeModel = strings.TrimSpace(value)
			}
		}
		assert.Equal(t, source.FallbackTier, claudeModel, agent)
	}
}

func TestLoadComposeAgentSourcesFrom_SourceVariants_MatchCodexTransform(t *testing.T) {
	fsys := fstest.MapFS{
		"agents/quiet.md":  {Data: []byte("---\nname: quiet\nmodel: haiku\n---\nbody\n")},
		"agents/loud.md":   {Data: []byte("---\nname: loud\nmodel: fable\neffort: XHigh\n---\nbody\n")},
		"agents/notes.txt": {Data: []byte("not an agent")},
	}

	sources, err := loadComposeAgentSourcesFrom(fsys)

	require.NoError(t, err)
	assert.Equal(t, map[string]composeAgentSource{
		"quiet": {FallbackTier: "haiku", CodexEffort: "medium"},
		"loud":  {FallbackTier: "fable", CodexEffort: "xhigh"},
	}, sources)
	parsed, err := pkgcontent.LoadAgentSourcesFromFS(fsys, "agents")
	require.NoError(t, err)
	for _, src := range parsed {
		args := composeTemplateModelArgs.FindStringSubmatch(pkgcontent.TransformAgentForCodex(src))
		require.Len(t, args, 4)
		assert.Equal(t, []string{sources[src.Meta.Name].FallbackTier, sources[src.Meta.Name].CodexEffort}, args[2:])
	}
}

func TestLoadComposeAgentSourcesFrom_MissingAgentsDir_ReturnsError(t *testing.T) {
	_, err := loadComposeAgentSourcesFrom(fstest.MapFS{})

	assert.ErrorContains(t, err, "agents")
}

// composeFullCatalog advertises low–max on every rung, so a declared effort
// survives resolution and the cell shows which effort the source declared.
const composeFullCatalog = `{"models":[` +
	`{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},` +
	`{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"}]},` +
	`{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"}]}]}`

// Every Codex cell the table computes — sources, the ForProvider(codex) view,
// and the run snapshot through codex.ResolveAgentProfile — equals what the real
// adapter writes when that run generates with the same snapshot.
func TestComposeAgentSources_CodexCells_MatchGeneratedAgentFiles(t *testing.T) {
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	stubComposeCatalogProbe(t, composeFullCatalog, nil)

	gapped := func(cfg *config.HarnessConfig) {
		// A partial custom preset: missing agents fall back to their source tier.
		cfg.Quality.Presets["custom"] = config.QualityPreset{Agents: map[string]string{
			"planner": "fable", "reviewer": "haiku", "annotator": "haiku", "executor": "opus",
		}}
		cfg.Quality.Providers = map[string]string{config.QualityProviderCodex: "custom"}
	}
	historicalUltra := func(cfg *config.HarnessConfig) {
		cfg.Quality.Default = "ultra"
		cfg.Quality.SupervisorModelPolicy = config.SupervisorModelPolicyQuality
		cfg.Quality.Presets["ultra"] = config.QualityPreset{Agents: map[string]string{
			"planner": "opus", "executor": "opus", "validator": "opus", "tester": "opus",
			"reviewer": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
			"debugger": "opus", "explorer": "opus", "devops": "opus",
		}}
	}
	fixtures := []struct {
		name  string
		edit  func(*config.HarnessConfig)
		known map[string]config.CodexProfile
	}{
		{
			name: "balanced default", edit: func(*config.HarnessConfig) {},
			known: map[string]config.CodexProfile{"planner": {Model: "gpt-6-astra", Effort: "max"}},
		},
		{
			name: "historical ultra with gaps", edit: historicalUltra,
			known: map[string]config.CodexProfile{
				"deep-worker": {Model: "gpt-6.1-sol", Effort: "xhigh"},
				"annotator":   {Model: "gpt-6.1-sol", Effort: "xhigh"},
			},
		},
		{
			name: "codex override to a partial custom preset", edit: gapped,
			known: map[string]config.CodexProfile{
				"reviewer":    {Model: "gpt-6-luna", Effort: "high"},
				"annotator":   {Model: "gpt-6-luna", Effort: "medium"},
				"deep-worker": {Model: "gpt-6.1-sol", Effort: "xhigh"},
			},
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			installComposeFakeCodex(t, composeCatalogA)
			dir := t.TempDir()
			cfg := composeSnapshotConfig("balanced", "codex")
			fixture.edit(cfg)
			snapshot := captureComposeSnapshot(context.Background(), cfg)

			supported, err := updateHarnessPlatformWith(context.Background(), dir, "codex", cfg, snapshot)
			require.True(t, supported)
			require.NoError(t, err)

			codexView := cfg.Quality.ForProvider(config.QualityProviderCodex)
			for _, agent := range config.CanonicalAgentNames() {
				source := sources[agent]
				cell, err := codex.ResolveAgentProfile(codexView, snapshot.codexCatalog, agent, source.FallbackTier, source.CodexEffort)
				require.NoError(t, err, agent)
				assert.Equal(t, cell.Effective, readComposeCodexProfile(t, dir, agent), agent)
				if want, ok := fixture.known[agent]; ok {
					assert.Equal(t, want, cell.Effective, agent)
				}
			}
		})
	}
}
