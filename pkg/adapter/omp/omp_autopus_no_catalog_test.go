package omp

// SPEC-MODELUX-003 S21 through adapter Update. A built-in route whose
// providers the catalog does not serve falls back to the runtime default: its
// agent is left out of the overrides and its file keeps its own thinking. An
// operator pin never degrades and fails the run with nothing written.
//
// The built-in balanced openai profile routes every agent through
// openai-codex, so on NO, the catalog without openai-codex, every route
// degrades and the activation has no override (task.agentModelOverrides must
// not be empty, in native mode as well). The degrade oracle therefore runs
// the same rule on the ultra anthropic ladder without its anthropic login,
// whose cross-family routes stay on openai-codex.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ompNOCatalog is NO: the balanced catalog without its openai-codex models.
const ompNOCatalog = `{"models":[
{"provider":"anthropic","id":"claude-fable-5-1","family":"anthropic","capabilities":["deep_reasoning","coding_tool_use","independent_dissent"],"thinking":["high","max"],"auth_enabled":true},
{"provider":"anthropic","id":"claude-sonnet-5-5","family":"anthropic","capabilities":["coding_tool_use","fast_validation","vision_design","deterministic_transform","independent_dissent"],"thinking":["high","max"],"auth_enabled":true}]}`

// ompOPConfig is OP, role_model_policy {v1, balanced, openai}, with pins.
func ompOPConfig(pins map[string]config.RoleModelCandidateConf) *config.HarnessConfig {
	cfg := configForOMP()
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai"}
	for agent, candidate := range pins {
		if cfg.RoleModelPolicy.Agents == nil {
			cfg.RoleModelPolicy.Agents = map[string]config.RoleAgentOverrideConf{}
		}
		cfg.RoleModelPolicy.Agents[agent] = config.RoleAgentOverrideConf{Candidates: []config.RoleModelCandidateConf{candidate}}
	}
	return cfg
}

// ompOPXPins are OPX's executor and tester pins.
var ompOPXPins = map[string]config.RoleModelCandidateConf{
	"executor": {Selector: "openai-codex/gpt-6-astra", Thinking: "max", Family: "openai"},
	"tester":   {Selector: "openai-codex/gpt-6.1-sol", Thinking: "xhigh", Family: "openai"},
}

func ompWrittenOverrides(t *testing.T, p *claimProject) map[string]string {
	t.Helper()
	var document struct {
		Task struct {
			AgentModelOverrides map[string]string `yaml:"agentModelOverrides"`
		} `yaml:"task"`
	}
	for _, path := range []string{DefaultOMPModelOverlayPath, configFile} {
		if data, ok := p.read(path); ok {
			require.NoError(t, yaml.Unmarshal([]byte(data), &document))
			if len(document.Task.AgentModelOverrides) > 0 {
				return document.Task.AgentModelOverrides
			}
		}
	}
	return nil
}

func TestOMPUpdate_S21_UnservedBuiltInRoutesDegradeAndKeepTheirThinking(t *testing.T) {
	p := newClaimProject(t)
	p.runner = withoutProvider(newBuiltinTierIntegrationRunner(), "anthropic")
	cfg := builtinIntegrationConfig("ultra")

	p.mustRun("update", cfg)

	overrides := ompWrittenOverrides(t, p)
	profile := resolvedIntegrationProfile(t, cfg)
	var degraded, routed []string
	for _, agent := range config.CanonicalAgentNames() {
		_, route, err := profile.OMPAgentRoute(agent)
		require.NoError(t, err, agent)
		served := false
		for _, candidate := range route.Candidates {
			served = served || strings.HasPrefix(candidate.Selector, "openai-codex/")
		}
		body, ok := p.read(OMPAgentDefinitionPath(agent))
		require.True(t, ok, agent)
		if served {
			routed = append(routed, agent)
			assert.Contains(t, overrides, agent)
			assert.NotContains(t, body, "\nthinkingLevel: ", "%s takes the override's thinking", agent)
			continue
		}
		degraded = append(degraded, agent)
		assert.NotContains(t, overrides, agent, "%s fell back to the runtime default", agent)
		assert.Contains(t, body, "\nthinkingLevel: ", "%s is not routed, so its file keeps its thinking", agent)
	}
	assert.NotEmpty(t, degraded)
	assert.NotEmpty(t, routed)
}

func TestOMPUpdate_S21_NOCatalogRefusesAnUnservedPin(t *testing.T) {
	p := newClaimProject(t)
	p.mustRun("generate", configForOMP())
	before := claimSnapshot(t, p)
	p.runner = &modelIntegrationFakeRunner{catalog: []byte(ompNOCatalog)}

	err := p.run("update", ompOPConfig(ompOPXPins))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "required_route_unresolved")
	assert.Contains(t, err.Error(), "executor")
	assert.Equal(t, before, claimSnapshot(t, p), "the OMP files are as before the run")
}
