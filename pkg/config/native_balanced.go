package config

import "maps"

// NativeBalancedAgentCandidate projects the same role matrix used by OMP onto
// the native Claude/Codex agent defaults. A different explicit tier retains
// the existing custom-tier path; an override never changes a sibling's route.
// Historical complete default layouts remain standard without rewriting YAML.
func (q QualityConf) NativeBalancedAgentCandidate(provider, agent string) (RoleModelCandidateConf, bool) {
	provider, ok := NormalizeQualityProvider(provider)
	if !ok || q.EffectiveMode(provider) != "balanced" {
		return RoleModelCandidateConf{}, false
	}
	agent = NormalizeAgentName(agent)
	rung, ok := balancedRungByAgent[agent]
	if !ok {
		return RoleModelCandidateConf{}, false
	}
	agents := q.Presets["balanced"].Agents
	if raw, explicit := agents[agent]; explicit {
		tier, valid := NormalizeQualityTier(raw)
		if !valid || (tier != balancedTierForRung(rung) && !isHistoricalBalancedDefault(agents)) {
			return RoleModelCandidateConf{}, false
		}
	}
	family := builtinRoleModelFamilyAnthropic
	if provider == QualityProviderCodex {
		family = builtinRoleModelFamilyOpenAI
	}
	return balancedRungCandidates[family][rung], true
}

func balancedTierForRung(rung balancedRoleRung) string {
	if rung == balancedRungTop {
		return "fable"
	}
	return "sonnet"
}

// defaultBalancedAgentTiers returns a fresh map for caller-owned quality
// settings. Model IDs and thinking remain defined in the shared role matrix.
func defaultBalancedAgentTiers() map[string]string {
	agents := make(map[string]string, len(balancedRungByAgent))
	for agent, rung := range balancedRungByAgent {
		agents[agent] = balancedTierForRung(rung)
	}
	return agents
}

// These exact layouts were shipped as complete default presets. A partial
// map or any different value is user customization, not an upgrade marker.
var historicalBalancedDefaults = []map[string]string{
	{
		"architect": "fable", "planner": "fable", "security-auditor": "fable",
		"debugger": "opus", "deep-worker": "opus", "executor": "opus",
		"reviewer": "opus", "spec-writer": "opus",
		"annotator": "sonnet", "devops": "sonnet", "explorer": "sonnet",
		"frontend-specialist": "sonnet", "perf-engineer": "sonnet",
		"tester": "sonnet", "ux-validator": "sonnet", "validator": "sonnet",
	},
	{
		"architect": "opus", "debugger": "sonnet", "devops": "sonnet",
		"executor": "sonnet", "explorer": "sonnet", "planner": "opus",
		"reviewer": "sonnet", "security-auditor": "opus", "spec-writer": "opus",
		"tester": "sonnet", "validator": "sonnet",
	},
}

func isHistoricalBalancedDefault(agents map[string]string) bool {
	for _, previous := range historicalBalancedDefaults {
		if maps.Equal(agents, previous) {
			return true
		}
	}
	return false
}
