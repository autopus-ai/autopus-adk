package omp

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// withoutProvider drops every catalog row of one provider, which is what OMP
// reports when that provider's login is missing or has expired.
func withoutProvider(runner *modelIntegrationFakeRunner, provider string) *modelIntegrationFakeRunner {
	var kept []string
	body := strings.TrimSuffix(strings.TrimPrefix(string(runner.catalog), `{"models":[`), `]}`)
	for _, row := range strings.Split(body, "},\n") {
		row = strings.TrimSuffix(strings.TrimSpace(row), "}")
		if !strings.Contains(row, `"provider":"`+provider+`"`) {
			kept = append(kept, row+"}")
		}
	}
	runner.catalog = []byte(`{"models":[` + strings.Join(kept, ",\n") + `]}`)
	return runner
}

// A user who never logged in to the anthropic provider must still be able to
// generate with the anthropic built-in ladder: routes that only name anthropic
// models fall back to OMP's default, the rest keep their selection.
func TestOMPModelIntegration_BuiltinDegradesRoutesWhoseProviderIsUnserved(t *testing.T) {
	t.Parallel()

	cfg := builtinIntegrationConfig("ultra")
	require.NoError(t, cfg.Validate())
	runner := withoutProvider(newBuiltinTierIntegrationRunner(), "anthropic")
	integration, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(runner).
		prepareModelIntegration(context.Background(), cfg)
	require.NoError(t, err, "a missing provider login must not block generation")

	degraded := map[string]bool{}
	for _, resolution := range integration.routing.Resolutions {
		if resolution.Status != "selected" {
			assert.Equal(t, "degraded", resolution.Status, resolution.Agent)
			assert.Equal(t, "explicit_runtime_default", resolution.DegradedReason, resolution.Agent)
			degraded[resolution.Agent] = true
		}
	}
	require.NotEmpty(t, degraded, "anthropic-only routes must degrade")
	for _, agent := range integration.projection.Agents {
		assert.False(t, degraded[agent.Agent], "degraded agent %s must not be pinned", agent.Agent)
		assert.True(t, strings.HasPrefix(agent.EffectiveSelector, "openai-codex/"), agent.EffectiveSelector)
	}

	routes, err := bridgeOMPIntegrationRoutes(integration.profile)
	require.NoError(t, err)
	DegradeUnservedDerivedOMPRoutes(integration.profile, integration.probe.Catalog, routes)
	notice := ompDegradedRoutesNotice(integration.probe.Catalog, routes, integration.routing)
	assert.Contains(t, notice, "use the OMP default model")
	assert.Contains(t, notice, "omp auth-broker login anthropic")
}

// An agent route the operator wrote is an explicit choice, so a missing
// provider still fails closed, now with the login hint that explains it.
func TestOMPModelIntegration_OperatorPinnedRouteStillFailsClosedWithHint(t *testing.T) {
	t.Parallel()

	cfg := builtinIntegrationConfig("ultra")
	cfg.RoleModelPolicy.Agents = map[string]config.RoleAgentOverrideConf{
		"explorer": {Candidates: []config.RoleModelCandidateConf{
			{Selector: "anthropic/" + config.ClaudeSonnetModel, Thinking: "high", Family: "anthropic"},
		}},
	}
	require.NoError(t, cfg.Validate())
	runner := withoutProvider(newBuiltinTierIntegrationRunner(), "anthropic")
	_, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(runner).
		prepareModelIntegration(context.Background(), cfg)
	require.ErrorContains(t, err, "required_route_unresolved")
	require.ErrorContains(t, err, "omp auth-broker login anthropic")
}

func TestDegradeUnservedDerivedOMPRoutes_OnlyTouchesBuiltinRoutesWithNoServedProvider(t *testing.T) {
	t.Parallel()

	catalog := OMPModelCatalog{Models: []OMPModelMetadata{
		{Provider: "openai-codex", Model: "m", AuthEnabled: true},
		{Provider: "google", Model: "g", Disabled: true},
	}}
	route := func(selectors ...string) OMPModelRouteRequest {
		request := OMPModelRouteRequest{Required: true}
		for _, selector := range selectors {
			request.Candidates = append(request.Candidates, OMPRoutingCandidate{Selector: selector})
		}
		return request
	}
	routes := map[string]OMPModelRouteRequest{
		"scout":    route("anthropic/a"),
		"task":     route("anthropic/a", "openai-codex/m"),
		"reviewer": route("google/g"),
	}
	DegradeUnservedDerivedOMPRoutes(config.RoleModelProfileConf{Builtin: true}, catalog, routes)
	assert.Equal(t, ompRuntimeDefaultAction, routes["scout"].DegradedAction)
	assert.Empty(t, routes["task"].DegradedAction, "a served provider keeps the route fail-closed")
	assert.Equal(t, ompRuntimeDefaultAction, routes["reviewer"].DegradedAction, "a disabled provider is unserved")

	custom := map[string]OMPModelRouteRequest{"scout": route("anthropic/a")}
	DegradeUnservedDerivedOMPRoutes(config.RoleModelProfileConf{}, catalog, custom)
	assert.Empty(t, custom["scout"].DegradedAction, "an operator-defined profile never degrades")
}
