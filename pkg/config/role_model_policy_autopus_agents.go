package config

import "fmt"

// In autopus mode OMP registers one generated definition per canonical agent,
// so a route needs no collapse: every agent is routed under its own name and
// task, OMP's default for an unnamed dispatch, borrows the planner row. This
// file owns the Route Contract that replaces the native collapse:
//
//	agent A: (1) agents.A pin (2) agents.N pin, where N is the bundled agent A
//	         collapses onto in native mode and N != A (3) the profile's own route
//	task:    (1) agents.task pin (2) agents.planner pin (3) the planner route
//
// A pin is an operator-written override with candidates. Rules (1) and (2) are
// operator choices and never degrade; only (3) may fall back to the runtime
// default. Every other bundled agent is disabled (OMPAutopusDisabledAgents)
// and gets no route.
// A pins-only profile has neither rule (2) for the roles nor rule (3): an
// agent without a pin has no route (ErrOMPAgentUnrouted).

// OMPAutopusAgentRoute resolves one agent served in autopus mode. Native in
// the result is the agent itself, the task.agentModelOverrides key OMP reads;
// Key is the agents entry or profile row the candidates came from; Role and
// Capability are the agent's own identity, so capability matching, family
// diversity, and audit rows stay real.
func (c RoleModelProfileConf) OMPAutopusAgentRoute(
	agent string,
) (OMPPolicyAgentRoute, RoleCapabilityRouteConf, error) {
	key, err := c.ompAutopusAgentRouteKey(agent)
	if err != nil {
		return OMPPolicyAgentRoute{}, RoleCapabilityRouteConf{}, err
	}
	identity, err := ResolveOMPPolicyAgent(agent)
	if err != nil {
		return OMPPolicyAgentRoute{}, RoleCapabilityRouteConf{}, err
	}
	if c.PinsOnly {
		identity.Key, identity.Native = key, agent
		candidates := append([]RoleModelCandidateConf(nil), c.Agents[key].Candidates...)
		return identity, RoleCapabilityRouteConf{Required: true, Candidates: candidates}, nil
	}
	route, err := c.AgentRoute(agent)
	if err != nil {
		return OMPPolicyAgentRoute{}, RoleCapabilityRouteConf{}, err
	}
	if key != agent {
		// The governing row supplies the models only; whether the route is
		// required stays with the agent's own capability.
		candidates, candidateErr := c.AgentCandidates(key)
		if candidateErr != nil {
			return OMPPolicyAgentRoute{}, RoleCapabilityRouteConf{}, candidateErr
		}
		route.Candidates = candidates
	}
	identity.Key = key
	identity.Native = agent
	return identity, route, nil
}

// OMPAutopusAgentOperatorPinned reports whether the agent's route is rule (1)
// or (2) of the Route Contract, an explicit operator choice.
func (c RoleModelProfileConf) OMPAutopusAgentOperatorPinned(agent string) bool {
	key, err := c.ompAutopusAgentRouteKey(agent)
	return err == nil && c.ompOperatorPinned(key)
}

func (c RoleModelProfileConf) ompAutopusAgentRouteKey(agent string) (string, error) {
	if agent == OMPAutopusDefaultAgent {
		for _, key := range []string{OMPAutopusDefaultAgent, "planner"} {
			if c.ompOperatorPinned(key) {
				return key, nil
			}
		}
		if c.PinsOnly {
			return "", ErrOMPAgentUnrouted
		}
		return "planner", nil
	}
	if _, canonical := capabilityBySourceAgent[agent]; !canonical {
		return "", fmt.Errorf("agent_role_unmapped: %q is not an Autopus OMP agent", agent)
	}
	if c.ompOperatorPinned(agent) {
		return agent, nil
	}
	if c.PinsOnly {
		return "", ErrOMPAgentUnrouted
	}
	if native, err := OMPNativeAgentForRole(agent); err == nil && native != agent && c.ompOperatorPinned(native) {
		return native, nil
	}
	return agent, nil
}

func (c RoleModelProfileConf) ompOperatorPinned(key string) bool {
	return c.OperatorAgents[key] && len(c.Agents[key].Candidates) > 0
}

// validateOMPAutopusAgentRoutes accepts any agents key the policy can name and
// requires a route for every agent the profile routes: all of autopus mode's
// agents, or the pinned ones of a pins-only profile. It never reports a
// native conflict: no two agents share a registry slot here.
func validateOMPAutopusAgentRoutes(profile RoleModelProfileConf) error {
	for _, agent := range sortedRoleAgentOverrides(profile.Agents) {
		if _, err := ResolveOMPPolicyAgent(agent); err != nil {
			return err
		}
	}
	for _, agent := range profile.OMPAgentNames() {
		if _, _, err := profile.OMPAutopusAgentRoute(agent); err != nil {
			return err
		}
	}
	return nil
}
