package omp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// reaFallbackIntegration prepares the autopus-mode built-in balanced
// integration of family over catalog edited by edit.
func reaFallbackIntegration(t *testing.T, family string, cfgEdit func(*config.HarnessConfig), edit func(string) string) (*ompModelIntegration, error) {
	t.Helper()
	cfg := builtinIntegrationConfigForFamily("balanced", family)
	if cfgEdit != nil {
		cfgEdit(cfg)
	}
	require.NoError(t, cfg.Validate())
	runner := newBuiltinTierIntegrationRunner()
	runner.catalog = []byte(edit(string(runner.catalog)))
	return NewWithRoot(t.TempDir()).WithModelIntegrationRunner(runner).prepareModelIntegration(context.Background(), cfg)
}

func withoutCatalogModel(model string) func(string) string {
	return func(catalog string) string {
		var raw struct {
			Models []map[string]any `json:"models"`
		}
		if err := json.Unmarshal([]byte(catalog), &raw); err != nil {
			panic(err)
		}
		kept := raw.Models[:0]
		for _, entry := range raw.Models {
			if entry["id"] != model {
				kept = append(kept, entry)
			}
		}
		raw.Models = kept
		data, err := json.Marshal(raw)
		if err != nil {
			panic(err)
		}
		return string(data)
	}
}

func unchangedCatalog(catalog string) string { return catalog }

// S11 K1: a catalog that serves the analysis rung routes reverse-engineer.
func TestOMPReaFallback_ServedAnalysisRungIsRouted(t *testing.T) {
	t.Parallel()
	integration, err := reaFallbackIntegration(t, "openai", nil, unchangedCatalog)
	require.NoError(t, err)
	selectors := builtinSelectorsByAgent(integration.projection)
	assert.Contains(t, selectors["reverse-engineer"], "openai-codex/"+config.CodexSolModel)
	assert.True(t, ompRoutedAgentNames(integration)["reverse-engineer"])
}

// S11 K2 and F-002: a missing analysis model, or a present one without the
// rung's thinking level, leaves only reverse-engineer unrouted.
func TestOMPReaFallback_UnservedAnalysisRungInheritsTheSessionModel(t *testing.T) {
	t.Parallel()
	served, err := reaFallbackIntegration(t, "openai", nil, unchangedCatalog)
	require.NoError(t, err)
	for name, run := range map[string]func() (*ompModelIntegration, error){
		"model absent (openai)": func() (*ompModelIntegration, error) {
			return reaFallbackIntegration(t, "openai", nil, withoutCatalogModel(config.CodexSolModel))
		},
		// The fixture lists Opus at xhigh only; the anthropic rung asks high.
		"thinking unsupported (anthropic)": func() (*ompModelIntegration, error) {
			return reaFallbackIntegration(t, "anthropic", nil, unchangedCatalog)
		},
	} {
		integration, err := run()
		require.NoError(t, err, name)
		selectors := builtinSelectorsByAgent(integration.projection)
		assert.NotContains(t, selectors, "reverse-engineer", name)
		assert.False(t, ompRoutedAgentNames(integration)["reverse-engineer"], name)
		assert.Len(t, selectors, len(config.OMPAutopusAgentNames())-1, name)
		if strings.HasPrefix(name, "model absent") {
			want := builtinSelectorsByAgent(served.projection)
			delete(want, "reverse-engineer")
			assert.Equal(t, want, selectors, "every other route stays as served")
		}
	}
}

// S11 K3 and the operator pin: an existing rung and a pinned route keep
// failing closed.
func TestOMPReaFallback_ExistingRungsAndPinsKeepTheBlocker(t *testing.T) {
	t.Parallel()
	_, err := reaFallbackIntegration(t, "openai", nil, withoutCatalogModel(config.CodexLunaModel))
	require.ErrorContains(t, err, "required_route_unresolved")

	pin := func(cfg *config.HarnessConfig) {
		cfg.RoleModelPolicy.Agents = map[string]config.RoleAgentOverrideConf{
			"reverse-engineer": {Candidates: []config.RoleModelCandidateConf{{
				Selector: "openai-codex/" + config.CodexSolModel, Thinking: "xhigh", Family: "openai",
			}}},
		}
	}
	_, err = reaFallbackIntegration(t, "openai", pin, withoutCatalogModel(config.CodexSolModel))
	require.ErrorContains(t, err, "reverse-engineer")
}

// The adapter reports a structured fact; the CLI prints it in the run's
// language, and a run that another route blocks reports none.
func TestOMPReaFallback_ReportsAStructuredFactOnlyOnSuccess(t *testing.T) {
	t.Parallel()
	collect := func(edit func(string) string) ([]adapter.ReaFallback, error) {
		cfg := builtinIntegrationConfigForFamily("balanced", "openai")
		runner := newBuiltinTierIntegrationRunner()
		runner.catalog = []byte(edit(string(runner.catalog)))
		var facts []adapter.ReaFallback
		ctx := adapter.WithReaFallbackSink(context.Background(), func(fact adapter.ReaFallback) {
			facts = append(facts, fact)
		})
		_, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(runner).prepareModelIntegration(ctx, cfg)
		return facts, err
	}
	facts, err := collect(withoutCatalogModel(config.CodexSolModel))
	require.NoError(t, err)
	assert.Equal(t, []adapter.ReaFallback{{
		Catalog: "OMP", Agent: "reverse-engineer", Requested: "openai-codex/" + config.CodexSolModel,
	}}, facts)

	facts, err = collect(func(catalog string) string {
		return withoutCatalogModel(config.CodexLunaModel)(withoutCatalogModel(config.CodexSolModel)(catalog))
	})
	require.ErrorContains(t, err, "required_route_unresolved")
	require.ErrorContains(t, err, "omp_profile_candidate_unavailable")
	require.ErrorContains(t, err, "openai-codex/"+config.CodexLunaModel)
	assert.Empty(t, facts)
}

// Doctor keeps every route; the one generation drops reads as the
// session-model fallback, which does not block the role summary.
func TestMarkOMPReaAnalysisFallbacks_DoctorReportsTheSessionModel(t *testing.T) {
	t.Parallel()
	profile, ok := config.BuiltinRoleModelProfile("balanced", config.QualityConf{}, "openai", "")
	require.True(t, ok)
	runner := newBuiltinTierIntegrationRunner()
	runner.catalog = []byte(withoutCatalogModel(config.CodexSolModel)(string(runner.catalog)))
	probe, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(runner).
		probeIntegratedModelCatalog(context.Background(), profile)
	require.NoError(t, err)
	routes, err := bridgeOMPIntegrationRoutes(profile)
	require.NoError(t, err)
	compilation := CompileOMPModelRouting(OMPModelRoutingInput{
		Catalog: probe.Catalog, CatalogReason: "catalog_ready", Routes: routes,
	})
	MarkOMPReaAnalysisFallbacks(profile, &compilation)
	rows := projectOMPModelDoctorRoles(compilation.Resolutions)
	for _, row := range rows {
		if row.Agent == "reverse-engineer" {
			assert.Equal(t, OMPReaSessionModelStatus, row.Status)
		} else {
			assert.NotEqual(t, OMPReaSessionModelStatus, row.Status, row.Agent)
		}
	}
	status, _ := ompModelDoctorRoleSummary(rows)
	assert.NotEqual(t, "blocked", status)
}
