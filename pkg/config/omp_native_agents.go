package config

import "fmt"

// OMPNativeAgentNames follows the bundled OMP registry, not the ADK role catalog.
func OMPNativeAgentNames() []string {
	return []string{"scout", "reviewer", "security-reviewer", "task", "sonic"}
}

// OMPNativeAgentForRole maps a workflow responsibility to an existing native
// agent. Reasoning, implementation, and validation are not delegated to sonic.
func OMPNativeAgentForRole(role string) (string, error) {
	switch role {
	case "scout", "explorer":
		return "scout", nil
	case "reviewer":
		return "reviewer", nil
	case "security-reviewer", "security-auditor":
		return "security-reviewer", nil
	case "sonic":
		return "sonic", nil
	case "task", "annotator", "architect", "debugger", "deep-worker", "devops", "executor", "frontend-specialist", "perf-engineer", "planner", "spec-writer", "tester", "ux-validator", "validator":
		return "task", nil
	default:
		return "", fmt.Errorf("unknown OMP workflow role %q", role)
	}
}

// OMPNativeAgentRepresentative selects the built-in quality profile row used
// for a native agent. General work retains the strong planning model; sonic's
// model selection does not authorize assigning it reasoning work.
func OMPNativeAgentRepresentative(agent string) (string, error) {
	switch agent {
	case "scout":
		return "explorer", nil
	case "reviewer":
		return "reviewer", nil
	case "security-reviewer":
		return "security-auditor", nil
	case "task":
		return "planner", nil
	case "sonic":
		return "validator", nil
	default:
		return "", fmt.Errorf("unknown native OMP agent %q", agent)
	}
}

// OMPNativeAgentGovernedBy reports whether a role_model_policy.agents key may
// carry the model of one bundled agent. A key governs the agent its own role
// collapses onto, and a representative key additionally governs the agent it
// stands in for: `validator` work is dispatched to `task`, yet `validator` is
// also the tier row `sonic` borrows, because no workflow role collapses onto
// sonic. Both sites that bind a model to a bundled agent - the profile route
// lookup and the projection validator - decide it here, so a row can never be
// accepted in one and rejected in the other.
func OMPNativeAgentGovernedBy(key, native string) bool {
	if collapsed, err := OMPNativeAgentForRole(key); err == nil && collapsed == native {
		return true
	}
	representative, err := OMPNativeAgentRepresentative(native)
	return err == nil && representative == key
}
