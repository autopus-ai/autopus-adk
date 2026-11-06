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
