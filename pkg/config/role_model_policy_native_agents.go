package config

import "fmt"

// OMPNativeAgentModelOverridesKey is the only OMP setting that binds a model
// to an agent: an exact, case-sensitive agent name mapped to a concrete model
// selector. It replaces the retired autopus_* modelRoles aliases.
const OMPNativeAgentModelOverridesKey = "task.agentModelOverrides"

// OMPPolicyAgentRoute is the identity one role_model_policy.agents key carries.
// Key is the configuration key the route came from, so an operator can be
// told exactly what to edit. Native names the agent OMP registers and
// task.agentModelOverrides keys. Role and Capability keep the semantic ADK
// identity so capability matching, family diversity, and audit rows stay real.
type OMPPolicyAgentRoute struct {
	Key        string
	Native     string
	Role       string
	Capability string
}

// ResolveOMPPolicyAgent accepts either one of the 17 ADK role names or one of
// the five bundled OMP agent names a policy written before v0.50.127 keys its
// pins by. An ADK name keeps its own role and capability; a bundled name
// borrows its representative's, because the bundled registry carries no
// capability of its own. Native is the bundled agent the key collapsed onto,
// which the Route Contract's rule (2) reads.
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
