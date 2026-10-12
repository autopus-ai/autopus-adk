package config

// Security I4: a selector is passed to the OMP CLI as its own argv element
// after --model, so neither half may start with '-' and read as a flag.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoleModelPolicyValidate_RefusesASelectorHalfStartingWithADash(t *testing.T) {
	policy := func(selector string) RoleModelPolicyConf {
		return RoleModelPolicyConf{
			Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
			Agents: map[string]RoleAgentOverrideConf{"task": {Candidates: []RoleModelCandidateConf{
				{Selector: selector, Thinking: "max", Family: "openai"},
			}}},
		}
	}

	assert.NoError(t, policy("openai-codex/gpt-6-astra").Validate())
	for _, selector := range []string{"-c/gpt-6-astra", "openai-codex/-gpt", "--config/x"} {
		err := policy(selector).Validate()
		if assert.Error(t, err, selector) {
			assert.Contains(t, err.Error(), "selector_invalid", selector)
		}
	}
}
