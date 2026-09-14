package config

import (
	"fmt"
	"sort"
)

// Per-agent override validation is shared by two declaration sites: the
// agents map inside a profile, and the root policy overlay that pins one
// agent on top of the selected profile. Both reuse the same matrix and
// candidate rules; only the reported scope differs.

// validateRootAgentOverlay checks the root per-agent overlay on its own terms:
// it needs a profile to overlay, one matrix agent per key, and candidates that
// satisfy the shared route rules. Trust-level rules run afterwards against the
// overlaid profile, where the effective catalog trust is known.
func (c RoleModelPolicyConf) validateRootAgentOverlay() error {
	if len(c.Agents) == 0 {
		return nil
	}
	if c.Profile == "" {
		return fmt.Errorf("role_model_policy.agents_require_profile")
	}
	for _, agent := range sortedRoleAgentOverrides(c.Agents) {
		override := c.Agents[agent]
		scope := fmt.Sprintf("role_model_policy.agents[%s]", agent)
		if err := validateRoleAgentOverride(scope, agent, override); err != nil {
			return err
		}
		// A root override exists to replace a route. An empty candidate list
		// would silently keep the profile's own route instead.
		if len(override.Candidates) == 0 {
			return fmt.Errorf("%s.candidates_required", scope)
		}
	}
	return nil
}

// sortedRoleAgentOverrides orders override validation so error reports are stable.
func sortedRoleAgentOverrides(overrides map[string]RoleAgentOverrideConf) []string {
	agents := make([]string, 0, len(overrides))
	for agent := range overrides {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	return agents
}

// validateAgentOverride validates one override declared inside a profile.
func validateAgentOverride(profile, agent string, override RoleAgentOverrideConf) error {
	if _, err := ResolveOMPPolicyAgent(agent); err != nil {
		return fmt.Errorf("role_model_policy.profiles[%s].%w", profile, err)
	}
	scope := fmt.Sprintf("role_model_policy.profiles[%s].agents[%s]", profile, agent)
	return validateRoleAgentOverride(scope, agent, override)
}

// validateRoleAgentOverride requires an agent the policy can route, rejects
// role or capability assertions that disagree with the resolved identity, and
// applies the capability candidate rules to override candidates. The scope
// prefixes every message so profile-scoped and root-scoped overlays report
// where they were declared.
func validateRoleAgentOverride(scope, agent string, override RoleAgentOverrideConf) error {
	resolved, err := ResolveOMPPolicyAgent(agent)
	if err != nil {
		return fmt.Errorf("%s.%w", scope, err)
	}
	if override.Role != "" && override.Role != resolved.Role {
		return fmt.Errorf("%s.role_capability_mismatch: role %q, want %q", scope, override.Role, resolved.Role)
	}
	if override.Capability != "" && override.Capability != resolved.Capability {
		return fmt.Errorf("%s.role_capability_mismatch: capability %q, want %q",
			scope, override.Capability, resolved.Capability)
	}
	return validateRouteCandidates(scope, override.Candidates)
}
