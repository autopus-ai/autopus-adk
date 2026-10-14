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
	"strings"
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

// Security review P2: on a strict catalog the catalog's own report attests a
// pins-only pin, so the receipt records strict trust and availability
// evidence instead of failing on an operator-attested receipt.
func TestOMPModelIntegration_PinsOnlyActivatesOnAStrictCatalog(t *testing.T) {
	p := newClaimProject(t)
	p.runner = newModelIntegrationRunner()

	p.mustRun("generate", pinsOnlyConfig(map[string]config.RoleModelCandidateConf{
		"planner":  {Selector: "anthropic/alpha-reasoner", Thinking: "high", Family: "anthropic"},
		"executor": {Selector: "openai/beta-coder", Thinking: "high", Family: "openai"},
	}))

	overrides := ompWrittenOverrides(t, p)
	assert.Equal(t, "anthropic/alpha-reasoner:high", overrides["planner"])
	assert.Equal(t, "openai/beta-coder:high", overrides["executor"])
	assert.Equal(t, overrides["planner"], overrides["task"])
	receiptData, ok := p.read(OMPModelReceiptRelativePath)
	require.True(t, ok)
	var receipt OMPModelResolutionReceipt
	require.NoError(t, json.Unmarshal([]byte(receiptData), &receipt))
	assert.Equal(t, config.RoleModelCatalogTrustStrict, receipt.CatalogTrust)
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

// Route Contract rule (2) on a metadata-light catalog: a task pin folds onto
// the roles native mode collapses into task, and the attestation declares it
// under each of those roles' capabilities, so the fold activates.
func TestOMPModelIntegration_AttestsATaskPinFoldedOntoTheRoles(t *testing.T) {
	p := newClaimProject(t)
	p.runner = newBuiltinTierIntegrationRunner()
	cfg := builtinIntegrationConfigForFamily("balanced", "openai")
	cfg.RoleModelPolicy.Agents = map[string]config.RoleAgentOverrideConf{"task": {Candidates: []config.RoleModelCandidateConf{
		{Selector: "openai-codex/" + config.CodexAstraModel, Thinking: "max", Family: "openai"},
	}}}

	p.mustRun("generate", cfg)

	overrides := ompWrittenOverrides(t, p)
	for _, agent := range []string{"annotator", "executor", "validator", "task"} {
		assert.Equal(t, "openai-codex/"+config.CodexAstraModel+":max", overrides[agent], agent)
	}
}

// Security review P1: on a catalog without family metadata a pins-only pin
// vouches for nothing. Only a model a built-in profile names is attested, and
// under the built-in family, so a hand-written unknown model or a wrong
// family fails closed with its reason.
func TestOMPModelIntegration_PinsOnlyPinsDoNotAttestThemselves(t *testing.T) {
	for name, row := range map[string]struct {
		pin    config.RoleModelCandidateConf
		reason string
	}{
		"unknown model": {config.RoleModelCandidateConf{Selector: "example/foo-1", Thinking: "high", Family: "example"}, "pin_model_unattested"},
		"wrong family":  {config.RoleModelCandidateConf{Selector: "openai-codex/" + config.CodexAstraModel, Thinking: "max", Family: "anthropic"}, "pin_family_mismatch"},
	} {
		t.Run(name, func(t *testing.T) {
			p := newClaimProject(t)
			p.runner = newBuiltinTierIntegrationRunner()
			p.runner.catalog = []byte(strings.TrimSuffix(string(p.runner.catalog), `]}`) +
				`,
{"provider":"example","id":"foo-1","selector":"example/foo-1","thinking":["high"]}]}`)
			p.mustRun("generate", configForOMP())
			before := claimSnapshot(t, p)

			err := p.run("update", pinsOnlyConfig(map[string]config.RoleModelCandidateConf{"executor": row.pin}))

			require.Error(t, err)
			assert.Contains(t, err.Error(), row.reason)
			assert.Equal(t, before, claimSnapshot(t, p))
		})
	}
}
