package omp

// SPEC-MODELUX-003 REQ-MUX3-22 through the adapter: a pins-only policy
// activates overrides for the pinned agents and task only on the
// metadata-light catalog an installed omp prints, attested by the pins' own
// families, and fails closed when a pinned model is not installed. (Like a
// built-in profile, a pins-only profile is operator-attested, so a strict
// catalog's availability evidence does not satisfy its receipt.)

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func pinsOnlyConfig(pins map[string]config.RoleModelCandidateConf) *config.HarnessConfig {
	cfg := configForOMP()
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Agents: map[string]config.RoleAgentOverrideConf{}}
	for agent, candidate := range pins {
		cfg.RoleModelPolicy.Agents[agent] = config.RoleAgentOverrideConf{Candidates: []config.RoleModelCandidateConf{candidate}}
	}
	return cfg
}

func TestOMPModelIntegration_PinsOnlyRoutesOnlyThePinnedAgents(t *testing.T) {
	for name, row := range map[string]struct {
		runner *modelIntegrationFakeRunner
		pins   map[string]config.RoleModelCandidateConf
	}{
		"metadata-light catalog": {newBuiltinTierIntegrationRunner(), map[string]config.RoleModelCandidateConf{
			"planner":  {Selector: "anthropic/" + config.ClaudeOpusModel, Thinking: "xhigh", Family: "anthropic"},
			"executor": {Selector: "openai-codex/" + config.CodexAstraModel, Thinking: "max", Family: "openai"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			p := newClaimProject(t)
			p.runner = row.runner

			p.mustRun("generate", pinsOnlyConfig(row.pins))

			overrides := ompWrittenOverrides(t, p)
			keys := make([]string, 0, len(overrides))
			for agent := range overrides {
				keys = append(keys, agent)
			}
			sort.Strings(keys)
			assert.Equal(t, []string{"executor", "planner", "task"}, keys)
			assert.Equal(t, overrides["planner"], overrides["task"], "task follows the planner pin")
			assert.Equal(t, row.pins["planner"].Selector+":"+row.pins["planner"].Thinking, overrides["planner"])
			receiptData, ok := p.read(OMPModelReceiptRelativePath)
			require.True(t, ok)
			var receipt OMPModelResolutionReceipt
			require.NoError(t, json.Unmarshal([]byte(receiptData), &receipt))
			assert.Equal(t, config.PinsOnlyRoleModelProfileName, receipt.Profile)
			for agent, routed := range map[string]bool{"planner": true, "executor": true, "explorer": false, "reviewer": false} {
				body, _ := p.read(OMPAgentDefinitionPath(agent))
				if routed {
					assert.NotContains(t, body, "\nthinkingLevel: ", agent)
				} else {
					assert.Contains(t, body, "\nthinkingLevel: ", agent)
				}
			}
		})
	}
}

func TestOMPModelIntegration_PinsOnlyFailsClosedOnAMissingModel(t *testing.T) {
	p := newClaimProject(t)
	p.mustRun("generate", configForOMP())
	before := claimSnapshot(t, p)

	err := p.run("update", pinsOnlyConfig(map[string]config.RoleModelCandidateConf{
		"executor": {Selector: "openai-codex/" + config.CodexAstraModel, Thinking: "max", Family: "openai"},
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "required_route_unresolved")
	assert.Contains(t, err.Error(), "executor")
	assert.Equal(t, before, claimSnapshot(t, p))
}
