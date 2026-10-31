package config

import "fmt"

// The bundled OMP registry outlives agent_source: native, which served it
// until v0.50.131: a role_model_policy written for it keys pins by the
// bundled names, the Route Contract's rule (2) routes a canonical agent on
// the pin of the bundled agent it collapsed onto, and task borrows the
// planner row.

// OMPNativeAgentNames follows the bundled OMP registry, not the ADK role catalog.
func OMPNativeAgentNames() []string {
	return []string{"scout", "reviewer", "security-reviewer", "task", "sonic"}
}

// OMPNativeAgentForRole maps a workflow responsibility to the bundled agent it
// collapsed onto. Reasoning, implementation, and validation never collapsed
// onto sonic.
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
	case "task", "annotator", "architect", "debugger", "deep-worker", "devops", "executor", "frontend-specialist", "perf-engineer", "planner", "reverse-engineer", "spec-writer", "tester", "ux-validator", "validator":
		return "task", nil
	default:
		return "", fmt.Errorf("unknown OMP workflow role %q", role)
	}
}

// OMPNativeAgentRepresentative selects the built-in quality profile row a
// bundled agent name borrows: task follows planner, and sonic's row does not
// authorize assigning it reasoning work.
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
