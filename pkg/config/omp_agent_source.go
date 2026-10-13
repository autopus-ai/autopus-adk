package config

import "fmt"

// OMP agent sources. Autopus is the default: OMP is served the 16 canonical
// Autopus agents as generated .omp/agents files, and its bundled scout,
// security-reviewer, and sonic are disabled. Native restores the v0.50.125
// surface, where every role collapses onto the five bundled agents. Native is
// kept for v0.50.126 only and is removed in v0.50.127.
const (
	OMPAgentSourceAutopus = "autopus"
	OMPAgentSourceNative  = "native"

	// OMPAutopusDefaultAgent is the bundled agent OMP runs a dispatch that
	// names no agent on. It stays enabled in autopus mode, otherwise every
	// unnamed dispatch would fail OMP's preflight.
	OMPAutopusDefaultAgent = "task"
)

// EffectiveOMPAgentSource maps a declared agent_source onto the source OMP is
// served with: anything but an explicit native is autopus.
func EffectiveOMPAgentSource(declared string) string {
	if declared == OMPAgentSourceNative {
		return OMPAgentSourceNative
	}
	return OMPAgentSourceAutopus
}

// OMPAgentSource returns the policy's effective OMP agent source.
func (c RoleModelPolicyConf) OMPAgentSource() string {
	return EffectiveOMPAgentSource(c.AgentSource)
}

// OMPNativeAgentSource reports whether a resolved profile serves the bundled
// agents. A profile that was not resolved through a policy is autopus.
func (c RoleModelProfileConf) OMPNativeAgentSource() bool {
	return c.AgentSource == OMPAgentSourceNative
}

// OMPAgentNames returns the agent names a source routes, in rank order: the
// canonical agents and then task for autopus, the bundled registry for native.
func OMPAgentNames(source string) []string {
	if EffectiveOMPAgentSource(source) == OMPAgentSourceNative {
		return OMPNativeAgentNames()
	}
	return OMPAutopusAgentNames()
}

// OMPAutopusAgentNames returns the 16 canonical agents followed by task.
func OMPAutopusAgentNames() []string {
	return append(CanonicalAgentNames(), OMPAutopusDefaultAgent)
}

// OMPAutopusDisabledAgents is the task.disabledAgents value autopus mode
// claims: the bundled agents an Autopus agent replaces. reviewer is not in it
// because the generated reviewer takes the bundled one's name.
func OMPAutopusDisabledAgents() []string {
	return []string{"scout", "security-reviewer", "sonic"}
}

func validateOMPAgentSource(declared string) error {
	switch declared {
	case "", OMPAgentSourceAutopus, OMPAgentSourceNative:
		return nil
	}
	return fmt.Errorf("role_model_policy.agent_source_invalid: %q", declared)
}

// validateOMPAgentRoutes runs the route check of the profile's own source.
func validateOMPAgentRoutes(profile RoleModelProfileConf) error {
	if profile.OMPNativeAgentSource() {
		return validateOMPNativeAgentCollapse(profile)
	}
	return validateOMPAutopusAgentRoutes(profile)
}

// OMPAgentNames lists the agents this resolved profile routes.
func (c RoleModelProfileConf) OMPAgentNames() []string {
	return OMPAgentNames(c.AgentSource)
}

// OMPAgentRoute resolves one served agent's route under the profile's source:
// the Route Contract in autopus mode, the native collapse otherwise.
func (c RoleModelProfileConf) OMPAgentRoute(agent string) (OMPPolicyAgentRoute, RoleCapabilityRouteConf, error) {
	if c.OMPNativeAgentSource() {
		return c.OMPNativeAgentRoute(agent)
	}
	return c.OMPAutopusAgentRoute(agent)
}

// OMPAgentOperatorPinned reports a route the operator chose explicitly, which
// must never fall back to the runtime default.
func (c RoleModelProfileConf) OMPAgentOperatorPinned(agent string) bool {
	if c.OMPNativeAgentSource() {
		return c.OMPNativeAgentOperatorPinned(agent)
	}
	return c.OMPAutopusAgentOperatorPinned(agent)
}
