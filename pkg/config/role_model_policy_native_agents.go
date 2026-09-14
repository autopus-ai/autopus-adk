package config

import (
	"fmt"
	"sort"
	"strings"
)

// OMPNativeAgentModelOverridesKey is the only OMP setting that binds a model
// to a bundled agent: an exact, case-sensitive agent name mapped to a concrete
// model selector. It replaces the retired autopus_* modelRoles aliases, which
// only existed to bind generated agent definitions that no longer exist.
const OMPNativeAgentModelOverridesKey = "task.agentModelOverrides"

// OMP registers exactly one agent definition per name, so the 16 ADK work
// roles cannot each own a registry entry. This file is the single owner of the
// collapse: which role_model_policy.agents key governs a native agent, and
// when two operator-written keys landing on the same native agent have to be
// refused instead of silently discarding one of them.

// OMPPolicyAgentRoute is the identity one role_model_policy.agents key carries
// after it collapses onto a native OMP agent. Key is the configuration key the
// route came from, so an operator can be told exactly what to edit. Native
// names the bundled agent OMP actually registers. Role and Capability keep the
// semantic ADK identity so capability matching, family diversity, and audit
// rows stay real.
type OMPPolicyAgentRoute struct {
	Key        string
	Native     string
	Role       string
	Capability string
}

// ResolveOMPPolicyAgent accepts either one of the 16 ADK role names or one of
// the five native OMP agent names. An ADK name keeps its own role and
// capability; a native name borrows its representative's, because the native
// registry carries no capability of its own.
func ResolveOMPPolicyAgent(key string) (OMPPolicyAgentRoute, error) {
	native, err := OMPNativeAgentForRole(key)
	if err != nil {
		return OMPPolicyAgentRoute{}, fmt.Errorf("agent_role_unmapped: %q", key)
	}
	source := key
	if _, matrix := capabilityBySourceAgent[key]; !matrix {
		source, err = OMPNativeAgentRepresentative(native)
		if err != nil {
			return OMPPolicyAgentRoute{}, err
		}
	}
	capability, err := OMPAgentCapability(source)
	if err != nil {
		return OMPPolicyAgentRoute{}, err
	}
	return OMPPolicyAgentRoute{
		Key: key, Native: native, Role: OMPAgentRoleName(source), Capability: capability,
	}, nil
}

// ErrOMPNativeAgentConflict reports several operator-written routes landing on
// one bundled OMP agent. Keeping either one would discard a model the operator
// chose on purpose, so the policy fails with the migration the operator has to
// make instead.
func ErrOMPNativeAgentConflict(native string, keys []string) error {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	return fmt.Errorf(
		"omp_native_agent_conflict: native agent %q receives conflicting model routes from "+
			"role_model_policy.agents entries [%s]; OMP registers one bundled %q agent, "+
			"so keep exactly one entry (prefer %q) and delete the rest",
		native, strings.Join(sorted, " "), native, native)
}

// OMPNativeAgentRoute returns the route that governs one native OMP agent: the
// single operator-written route that collapses onto it, or the representative
// ADK role's route when the operator wrote none. Built-in per-agent routes are
// derived from a tier ladder rather than written, so they never conflict.
func (c RoleModelProfileConf) OMPNativeAgentRoute(
	native string,
) (OMPPolicyAgentRoute, RoleCapabilityRouteConf, error) {
	key, err := c.ompNativeAgentRouteKey(native)
	if err != nil {
		return OMPPolicyAgentRoute{}, RoleCapabilityRouteConf{}, err
	}
	resolved, err := ResolveOMPPolicyAgent(key)
	if err != nil {
		return OMPPolicyAgentRoute{}, RoleCapabilityRouteConf{}, err
	}
	route, err := c.AgentRoute(key)
	if err != nil {
		return OMPPolicyAgentRoute{}, RoleCapabilityRouteConf{}, err
	}
	if !OMPNativeAgentGovernedBy(key, native) {
		return OMPPolicyAgentRoute{}, RoleCapabilityRouteConf{}, fmt.Errorf(
			"agent_role_unmapped: %q does not govern bundled agent %q", key, native)
	}
	// A representative key can name a role that is dispatched elsewhere
	// (`validator` work goes to `task`, but it is the row `sonic` borrows), so
	// the row belongs to the requested agent while Key records its origin.
	resolved.Native = native
	return resolved, route, nil
}

func (c RoleModelProfileConf) ompNativeAgentRouteKey(native string) (string, error) {
	keys := make([]string, 0, len(c.OperatorAgents))
	for agent := range c.OperatorAgents {
		if len(c.Agents[agent].Candidates) == 0 {
			continue
		}
		resolved, err := ResolveOMPPolicyAgent(agent)
		if err != nil {
			return "", err
		}
		if resolved.Native == native {
			keys = append(keys, agent)
		}
	}
	if len(keys) == 0 {
		return OMPNativeAgentRepresentative(native)
	}
	sort.Strings(keys)
	agreed := true
	for _, key := range keys[1:] {
		if !equalRoleModelCandidates(c.Agents[keys[0]].Candidates, c.Agents[key].Candidates) {
			agreed = false
			break
		}
	}
	if agreed {
		return keys[0], nil
	}
	// A full per-role policy - what the quality wizard writes and what every
	// pre-cutover config holds - names all 16 roles with different models, so
	// disagreement is the normal case rather than an operator mistake. The
	// representative row is the declared tie-break, and the preview and receipt
	// report it as the governing key, so the choice is visible instead of
	// arbitrary. Only a set with no representative is genuinely undecidable.
	representative, err := OMPNativeAgentRepresentative(native)
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		if key == representative {
			return representative, nil
		}
	}
	return "", ErrOMPNativeAgentConflict(native, keys)
}

// validateOMPNativeAgentCollapse resolves every native agent so a policy that
// cannot project is rejected where it is declared rather than at generation.
func validateOMPNativeAgentCollapse(profile RoleModelProfileConf) error {
	for _, native := range OMPNativeAgentNames() {
		if _, _, err := profile.OMPNativeAgentRoute(native); err != nil {
			return err
		}
	}
	return nil
}

func equalRoleModelCandidates(left, right []RoleModelCandidateConf) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
