package config

// Standard balanced placement shares this matrix across OMP and the native
// Claude/Codex adapters. OMP projects one exact candidate per agent; native
// adapters use NativeBalancedAgentCandidate while preserving custom tiers.
// Every standard role follows its selected family, including ordinary review.
// Multi-provider review remains an independent orchestra setting.

// balancedRoleRung groups the canonical agents by the depth their work needs.
type balancedRoleRung int

const (
	balancedRungTop balancedRoleRung = iota
	balancedRungImplementation
	balancedRungRoutine
	// balancedRungAnalysis is the rung of reverse-engineer (SPEC-REA-001) and
	// qa-explorer: the opus tier, at the exact model and thinking the native
	// ladders give opus.
	balancedRungAnalysis
)

// balancedRungByAgent covers every canonical agent exactly once. Debugger and
// deep-worker share the top rung with the planning and review roles: both
// routinely have to hold a whole failing system in context at once.
var balancedRungByAgent = map[string]balancedRoleRung{
	"architect":        balancedRungTop,
	"debugger":         balancedRungTop,
	"deep-worker":      balancedRungTop,
	"planner":          balancedRungTop,
	"reviewer":         balancedRungTop,
	"security-auditor": balancedRungTop,
	"spec-writer":      balancedRungTop,

	"devops":              balancedRungImplementation,
	"executor":            balancedRungImplementation,
	"frontend-specialist": balancedRungImplementation,
	"perf-engineer":       balancedRungImplementation,
	"tester":              balancedRungImplementation,

	"annotator":    balancedRungRoutine,
	"explorer":     balancedRungRoutine,
	"ux-validator": balancedRungRoutine,
	"validator":    balancedRungRoutine,

	"qa-explorer":      balancedRungAnalysis,
	"reverse-engineer": balancedRungAnalysis,
}

// balancedRungCandidates pins one exact candidate per anchor family and rung.
// The Anthropic top rung is Opus at max; Fable is reserved for Ultra.
// A rung never carries a second candidate: ordered availability fallback stays
// available to hand-written profiles, and this profile declines it so an
// unavailable model surfaces as a named blocker.
var balancedRungCandidates = map[string]map[balancedRoleRung]RoleModelCandidateConf{
	builtinRoleModelFamilyAnthropic: {
		balancedRungTop: {
			Selector: "anthropic/" + ClaudeOpusModel, Thinking: "max", Family: "anthropic",
		},
		balancedRungImplementation: {
			Selector: "anthropic/" + ClaudeSonnetModel, Thinking: "max", Family: "anthropic",
		},
		balancedRungRoutine: {
			Selector: "anthropic/" + ClaudeSonnetModel, Thinking: "high", Family: "anthropic",
		},
		balancedRungAnalysis: {
			Selector: "anthropic/" + ClaudeOpusModel, Thinking: "high", Family: "anthropic",
		},
	},
	builtinRoleModelFamilyOpenAI: {
		balancedRungTop: {
			Selector: "openai-codex/" + CodexAstraModel, Thinking: "max", Family: "openai",
		},
		balancedRungImplementation: {
			Selector: "openai-codex/" + CodexLunaModel, Thinking: "max", Family: "openai",
		},
		balancedRungRoutine: {
			Selector: "openai-codex/" + CodexLunaModel, Thinking: "max", Family: "openai",
		},
		balancedRungAnalysis: {
			Selector: "openai-codex/" + CodexSolModel, Thinking: "xhigh", Family: "openai",
		},
	},
}

// balancedRoleModelProfile builds the balanced profile for one anchor family.
func balancedRoleModelProfile(family, mode string) RoleModelProfileConf {
	rungs := balancedRungCandidates[family]
	agents := make(map[string]RoleAgentOverrideConf, len(balancedRungByAgent))
	for agent, rung := range balancedRungByAgent {
		agents[agent] = RoleAgentOverrideConf{
			Candidates: []RoleModelCandidateConf{rungs[rung]},
		}
	}
	capabilities := make(map[string]RoleCapabilityRouteConf, len(providerNeutralCapabilities))
	for _, capability := range providerNeutralCapabilities {
		capabilities[capability] = RoleCapabilityRouteConf{
			Candidates: []RoleModelCandidateConf{rungs[balancedCapabilityRung(capability)]},
			Required:   true,
		}
	}
	return RoleModelProfileConf{
		ConfigMode:   mode,
		CatalogTrust: RoleModelCatalogTrustOperatorAttested,
		Capabilities: capabilities,
		Agents:       agents,
		ManagedKeys:  builtinRoleModelManagedKeys(mode),
	}
}

// balancedCapabilityRung gives a capability the rung of its representative
// agent. Capability routes are defaults for a hand-written profile that copies
// them without the per-agent overrides, so coding_tool_use deliberately keeps
// the implementation rung: folding it onto the highest rung among its agents
// would put every execution role behind debugger's top-rung model.
func balancedCapabilityRung(capability string) balancedRoleRung {
	return balancedRungByAgent[canonicalAgentByCapability[capability]]
}
