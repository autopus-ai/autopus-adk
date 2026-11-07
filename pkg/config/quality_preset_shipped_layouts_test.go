package config

// The v0.50.125 defaults: sixteen agents without reverse-engineer. They are
// written out here instead of derived from the current defaults so the
// registry rows cannot drift with them (SPEC-REA-001 Preset Contract).
var (
	shippedBalancedHB6 = map[string]string{
		"architect": "fable", "debugger": "fable", "deep-worker": "fable", "planner": "fable",
		"reviewer": "fable", "security-auditor": "fable", "spec-writer": "fable",
		"annotator": "sonnet", "devops": "sonnet", "executor": "sonnet", "explorer": "sonnet",
		"frontend-specialist": "sonnet", "perf-engineer": "sonnet", "tester": "sonnet",
		"ux-validator": "sonnet", "validator": "sonnet",
	}
	shippedUltraHU3 = map[string]string{
		"architect": "fable", "debugger": "fable", "deep-worker": "fable", "planner": "fable",
		"reviewer": "fable", "security-auditor": "fable", "spec-writer": "fable",
		"annotator": "opus", "devops": "opus", "executor": "opus", "explorer": "opus",
		"frontend-specialist": "opus", "perf-engineer": "opus", "tester": "opus",
		"ux-validator": "opus", "validator": "opus",
	}
)

// The v0.50.134 defaults: the last seventeen-agent layouts before the
// verification harness cut the agents to reviewer and the two UX roles.
var (
	shippedBalancedHB8 = map[string]string{
		"architect": "opus", "debugger": "opus", "deep-worker": "opus", "planner": "opus",
		"reviewer": "opus", "security-auditor": "opus", "spec-writer": "opus",
		"reverse-engineer": "opus",
		"annotator":        "sonnet", "devops": "sonnet", "executor": "sonnet", "explorer": "sonnet",
		"frontend-specialist": "sonnet", "perf-engineer": "sonnet", "tester": "sonnet",
		"ux-validator": "sonnet", "validator": "sonnet",
	}
	shippedUltraHU4 = map[string]string{
		"architect": "fable", "debugger": "fable", "deep-worker": "fable", "planner": "fable",
		"reviewer": "fable", "security-auditor": "fable", "spec-writer": "fable",
		"annotator": "opus", "devops": "opus", "executor": "opus", "explorer": "opus",
		"frontend-specialist": "opus", "perf-engineer": "opus", "reverse-engineer": "opus",
		"tester": "opus", "ux-validator": "opus", "validator": "opus",
	}
)
