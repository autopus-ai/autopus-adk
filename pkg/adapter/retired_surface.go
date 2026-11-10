package adapter

// The verification harness stopped generating the planning and
// implementation surface that v0.50.133 installed. Update prunes those files
// through each manifest, but Clean and the legacy-shim cleanups still meet
// them on an install that was never updated, so every adapter recognizes the
// names below as its own rather than refusing the manifest or leaving them.

// RetiredWorkflowRoutes are the `/auto` routes no longer installed, by skill
// name.
var RetiredWorkflowRoutes = []string{
	"auto-setup", "auto-status", "auto-goal", "auto-plan", "auto-go", "auto-fix",
	"auto-idea", "auto-map", "auto-why", "auto-dev", "auto-canary",
	"auto-reverse",
}

// RetiredSkillNames are the reusable skills no longer installed.
var RetiredSkillNames = []string{
	"adaptive-quality", "agent-pipeline", "agent-presets", "agent-teams", "api-design",
	"ast-refactoring", "ax-annotation", "brainstorming", "ci-cd", "codebase-design",
	"competitive-analysis", "context-search", "database", "ddd", "debugging", "docker",
	"domain-modeling", "double-diamond", "entropy-scan", "experiment", "frontend-skill",
	"git-worktrees", "grilling", "harness-workflow", "hash-anchored-edit", "idea",
	"korean-writing-refiner", "make-interfaces-feel-better", "metrics", "migration",
	"monitor-patterns", "performance", "planning", "prd", "product-discovery",
	"refactoring", "resolving-merge-conflicts", "security-audit", "spec-review",
	"subagent-dev", "tdd", "using-autopus", "wait-what", "worktree-isolation",
	"writing-skills",
}

// RetiredSkillResources are the reference bodies retired skills installed
// beside their SKILL.md, relative to the skill directory.
var RetiredSkillResources = map[string][]string{
	"agent-pipeline": {
		"references/completion.md", "references/coordination.md", "references/delegation.md",
		"references/gates.md", "references/phases.md", "references/quality-modes.md",
		"references/review.md", "references/verification.md",
	},
}

// RetiredAgentNames are the agents no longer generated.
var RetiredAgentNames = []string{
	"annotator", "architect", "debugger", "deep-worker", "devops", "executor",
	"explorer", "perf-engineer", "planner", "reverse-engineer", "security-auditor",
	"spec-writer", "tester", "validator",
}

// RetiredRuleNames are the rules no longer installed, without extension.
var RetiredRuleNames = []string{
	"branding", "context7-docs", "deferred-tools", "doc-storage", "objective-reasoning",
	"project-identity", "shell-portability", "spec-quality", "subagent-delegation",
	"techstack-freshness", "worktree-safety",
}
