package omp

import "github.com/insajin/autopus-adk/pkg/config"

func integrationHarnessConfig(mode string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("integration")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1,
		Profile: "p1",
		Profiles: map[string]config.RoleModelProfileConf{
			"p1": {
				ConfigMode: mode,
				Capabilities: map[string]config.RoleCapabilityRouteConf{
					config.CapabilityDeepReasoning: {
						Required: true, Candidates: []config.RoleModelCandidateConf{{Selector: "anthropic/alpha-reasoner", Thinking: "xhigh", Family: "anthropic"}},
					},
					config.CapabilityCodingToolUse: {
						Required: true, Candidates: []config.RoleModelCandidateConf{{Selector: "openai/beta-coder", Thinking: "high", Family: "openai"}},
					},
					config.CapabilityFastValidation: {
						Required: true, Candidates: []config.RoleModelCandidateConf{{Selector: "openai/beta-coder", Thinking: "high", Family: "openai"}},
					},
					config.CapabilityVisionDesign: {
						Required: true, Candidates: []config.RoleModelCandidateConf{{Selector: "google/gamma-vision", Thinking: "high", Family: "google"}},
					},
					config.CapabilityIndependentDissent: {
						Required: true, Candidates: []config.RoleModelCandidateConf{{Selector: "openai/beta-coder", Thinking: "high", Family: "openai"}, {Selector: "anthropic/alpha-reasoner", Thinking: "high", Family: "anthropic"}},
					},
					config.CapabilityDeterministicTransform: {
						Required: true, Candidates: []config.RoleModelCandidateConf{{Selector: "openai/beta-coder", Thinking: "high", Family: "openai"}},
					},
				},
				FamilyDiversity: config.FamilyDiversityPolicyConf{
					Enabled: true,
					Roles:   []string{config.OMPAgentRoleName("reviewer"), config.OMPAgentRoleName("security-auditor")},
				},
			},
		},
	}
	if mode == config.RoleModelConfigModeProjectManaged {
		profile := cfg.RoleModelPolicy.Profiles["p1"]
		missing := OMPMissingManagedValueFingerprint()
		profile.ManagedKeys = map[string]config.RoleManagedKeyClaimConf{
			config.OMPNativeAgentModelOverridesKey: {PriorFingerprint: missing, Complete: true},
			"retry.fallbackChains": {
				PriorFingerprint: missing, Complete: true, FullArrayOwnership: true,
			},
			"retry.modelFallback": {PriorFingerprint: missing, Complete: true},
		}
		cfg.RoleModelPolicy.Profiles["p1"] = profile
	}
	return cfg
}
