package config

import "slices"

// legacyAgentNames are the agents releases before the verification harness
// generated. A config written by such a release still names them in quality
// presets and efforts; loading accepts those keys instead of failing, since
// nothing is generated for them any more.
var legacyAgentNames = []string{
	"annotator", "architect", "debugger", "deep-worker", "devops", "executor",
	"explorer", "perf-engineer", "planner", "reverse-engineer",
	"security-auditor", "spec-writer", "tester", "validator",
}

// dispatchedLegacyRoles are the legacy roles the binary still routes work to
// without generating an agent for them, so the OMP role-model matrix and the
// balanced rungs keep a row for each: team workflow phases run planner,
// tester, executor, annotator, and security-auditor; `auto pipeline` runs
// planner, tester, executor, and validator; OMP's bundled task, scout, and
// sonic agents borrow the planner, explorer, and validator rows; and the
// REA analysis rung keeps reverse-engineer. The other legacy roles have no
// consumer and no row: a role_model_policy key that names one still resolves
// through the bundled agent it collapsed onto (OMPNativeAgentForRole).
var dispatchedLegacyRoles = []string{
	"annotator", "executor", "explorer", "planner", "reverse-engineer",
	"security-auditor", "tester", "validator",
}

// legacyRoleTiers keeps the tier each legacy role had in the last
// seventeen-agent built-in presets (HB8, HU4). The role names still route
// work the binary performs without generating an agent for it: OMP's default
// task dispatch borrows the planner row, legacy tier routes anchor on the
// capability representatives, and `auto pipeline` dispatches these roles.
// The presets written to autopus.yaml no longer list them, so without this
// table those routes would silently drop to the fallback tier.
var legacyRoleTiers = map[string]map[string]string{
	"balanced": {
		"architect": "opus", "debugger": "opus", "deep-worker": "opus", "planner": "opus",
		"security-auditor": "opus", "spec-writer": "opus", "reverse-engineer": "opus",
		"annotator": "sonnet", "devops": "sonnet", "executor": "sonnet", "explorer": "sonnet",
		"perf-engineer": "sonnet", "tester": "sonnet", "validator": "sonnet",
	},
	"ultra": {
		"architect": "fable", "debugger": "fable", "deep-worker": "fable", "planner": "fable",
		"security-auditor": "fable", "spec-writer": "fable",
		"annotator": "opus", "devops": "opus", "executor": "opus", "explorer": "opus",
		"perf-engineer": "opus", "reverse-engineer": "opus", "tester": "opus", "validator": "opus",
	},
}

// RoleNames returns the role vocabulary: the generated agents followed by
// the legacy roles. Work the binary dispatches by role, such as pipeline
// phases and their cost accounting, resolves over this set, while anything
// that installs or renders an agent definition uses CanonicalAgentNames.
func RoleNames() []string {
	return append(CanonicalAgentNames(), legacyAgentNames...)
}

// IsLegacyAgentName reports whether name is an agent an older release
// generated and this one no longer does.
func IsLegacyAgentName(name string) bool {
	return slices.Contains(legacyAgentNames, NormalizeAgentName(name))
}

// legacyRoleTier resolves a legacy role under a built-in preset written in the
// three-agent format. A preset that still names any legacy role was written
// by an older release, or by hand in its format, and keeps the old reading: an
// omitted agent takes the caller's fallback. Custom presets do the same.
func legacyRoleTier(presetName string, written map[string]string, agent string) (string, bool) {
	tier, ok := legacyRoleTiers[presetName][agent]
	if !ok {
		return "", false
	}
	for _, name := range legacyAgentNames {
		if _, named := written[name]; named {
			return "", false
		}
	}
	return tier, true
}
