package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeTableTestdata holds the acceptance fixtures of SPEC-MODELUX-001: the
// catalog snapshots CAT-A, CAT-B, and CAT-D, and R, a frozen copy of this
// repository's autopus.yaml (balanced HB2, ultra HU1, default ultra,
// supervisor_model_policy quality, five platforms, no role_model_policy).
const composeTableTestdata = "testdata/quality_compose"

// composeTableCatalogs lists the catalog fixtures in acceptance order. CAT-C
// is a failed probe, which freezes an empty snapshot.
var composeTableCatalogs = []string{"CAT-A", "CAT-B", "CAT-C", "CAT-D"}

// composeTableSnapshot returns the run snapshot one catalog fixture freezes,
// exactly as captureComposeSnapshot would after probing that catalog.
func composeTableSnapshot(t *testing.T, catalog string) driftGenerationSnapshot {
	t.Helper()
	if catalog == "CAT-C" {
		return driftGenerationSnapshot{codexCatalogFixed: true}
	}
	data, err := os.ReadFile(filepath.Join(composeTableTestdata, strings.ToLower(catalog)+".json"))
	require.NoError(t, err)
	return driftGenerationSnapshot{codexCatalog: data, codexCatalogFixed: true}
}

// Historical shipped layouts, written out from the SPEC registry rather than
// read from pkg/config, so a fixture cannot drift along with the code.
var (
	composeTableHB1 = map[string]string{
		"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet", "devops": "sonnet",
		"validator": "haiku", "explorer": "haiku",
	}
	composeTableHB2 = map[string]string{
		"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet", "devops": "sonnet",
		"validator": "sonnet", "explorer": "sonnet",
	}
	// composeTableK is the custom preset S6 saves from D with `g`, Enter, `3`,
	// Enter, `4`, `y`.
	composeTableK = map[string]string{
		"architect": "fable", "planner": "fable", "reviewer": "fable", "security-auditor": "fable", "spec-writer": "fable",
		"debugger": "sonnet", "deep-worker": "sonnet", "devops": "sonnet", "executor": "sonnet",
		"frontend-specialist": "sonnet", "perf-engineer": "sonnet", "tester": "sonnet", "ux-validator": "sonnet",
		"validator": "sonnet", "explorer": "haiku", "annotator": "haiku",
	}
)

// composeTableFixture builds one named acceptance fixture and loads it back
// through the real loader, so every table reads what a command run reads.
func composeTableFixture(t *testing.T, name string) *config.HarnessConfig {
	t.Helper()
	switch name {
	case "D", "B2", "H", "P", "O1", "K":
		cfg := config.DefaultFullConfig("p")
		cfg.Platforms = []string{"claude-code", "codex"}
		cfg.Quality.SupervisorModelPolicy = config.SupervisorModelPolicyInherit
		switch name {
		case "B2":
			cfg.Quality.Presets["balanced"] = withComposeTableAgents(cfg.Quality.Presets["balanced"], nil, composeTableHB2)
		case "H":
			cfg.Quality.Presets["balanced"] = withComposeTableAgents(cfg.Quality.Presets["balanced"], nil, composeTableHB1)
		case "P":
			haiku := make(map[string]string)
			for _, agent := range config.CanonicalAgentNames() {
				haiku[agent] = "haiku"
			}
			cfg.Quality.Presets["codex-custom"] = config.QualityPreset{Description: "codex", Agents: haiku}
			cfg.Quality.Providers = map[string]string{config.QualityProviderCodex: "codex-custom"}
		case "O1":
			cfg.Platforms = append(cfg.Platforms, "omp")
			cfg.RoleModelPolicy.Profile = "balanced"
		case "K":
			cfg.Quality.Presets["custom"] = config.QualityPreset{Description: "역할 그룹 구성", Agents: composeTableK}
			cfg.Quality.Default = "custom"
		}
		return roundTripComposeTableConfig(t, cfg)
	case "R", "R2", "O2", "X", "C":
		data, err := os.ReadFile(filepath.Join(composeTableTestdata, "R.yaml"))
		require.NoError(t, err)
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), data, 0o644))
		cfg, err := config.LoadPreview(dir)
		require.NoError(t, err)
		switch name {
		case "R2":
			cfg.Platforms = []string{"claude-code", "codex"}
		case "O2":
			cfg.RoleModelPolicy.Profile = "ultra"
		case "X":
			ultra := cfg.Quality.Presets["ultra"]
			cfg.Quality.Presets["ultra"] = withComposeTableAgents(ultra, ultra.Agents, map[string]string{"tester": "gpt"})
		case "C":
			ultra := cfg.Quality.Presets["ultra"]
			cfg.Quality.Presets["ultra"] = withComposeTableAgents(ultra, ultra.Agents, map[string]string{"devops": "sonnet"})
		}
		return cfg
	}
	t.Fatalf("unknown compose table fixture %q", name)
	return nil
}

// withComposeTableAgents returns preset whose agents are a fresh map holding
// base overlaid with overlay; a nil base replaces the layout outright.
func withComposeTableAgents(preset config.QualityPreset, base, overlay map[string]string) config.QualityPreset {
	agents := make(map[string]string, len(base)+len(overlay))
	for agent, tier := range base {
		agents[agent] = tier
	}
	for agent, tier := range overlay {
		agents[agent] = tier
	}
	preset.Agents = agents
	return preset
}

func roundTripComposeTableConfig(t *testing.T, cfg *config.HarnessConfig) *config.HarnessConfig {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, cfg))
	loaded, err := config.LoadPreview(dir)
	require.NoError(t, err)
	return loaded
}
