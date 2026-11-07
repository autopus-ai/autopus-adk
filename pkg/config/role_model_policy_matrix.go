package config

import (
	"fmt"
	"sort"
	"strings"
)

const (
	CapabilityDeepReasoning          = "deep_reasoning"
	CapabilityCodingToolUse          = "coding_tool_use"
	CapabilityFastValidation         = "fast_validation"
	CapabilityVisionDesign           = "vision_design"
	CapabilityIndependentDissent     = "independent_dissent"
	CapabilityDeterministicTransform = "deterministic_transform"

	// ompAgentRolePrefix namespaces every projected OMP model role so Autopus
	// never owns an OMP native role key.
	ompAgentRolePrefix = "autopus_"
)

var providerNeutralCapabilities = []string{
	CapabilityDeepReasoning,
	CapabilityCodingToolUse,
	CapabilityFastValidation,
	CapabilityVisionDesign,
	CapabilityIndependentDissent,
	CapabilityDeterministicTransform,
}

var ompNativeThinkingLevels = map[string]struct{}{
	"off": {}, "none": {}, "minimal": {}, "low": {}, "medium": {},
	"high": {}, "xhigh": {}, "max": {}, "auto": {},
}

// capabilityBySourceAgent is the SPEC-OMP-005 Policy Contract: every generated
// agent owns exactly one provider-neutral capability and one role derived by
// OMPAgentRoleName.
var capabilityBySourceAgent = map[string]string{
	"annotator":           CapabilityFastValidation,
	"architect":           CapabilityDeepReasoning,
	"debugger":            CapabilityCodingToolUse,
	"deep-worker":         CapabilityCodingToolUse,
	"devops":              CapabilityCodingToolUse,
	"executor":            CapabilityCodingToolUse,
	"explorer":            CapabilityFastValidation,
	"frontend-specialist": CapabilityVisionDesign,
	"perf-engineer":       CapabilityCodingToolUse,
	"planner":             CapabilityDeepReasoning,
	"qa-explorer":         CapabilityIndependentDissent,
	"reverse-engineer":    CapabilityDeepReasoning,
	"reviewer":            CapabilityIndependentDissent,
	"security-auditor":    CapabilityIndependentDissent,
	"spec-writer":         CapabilityDeepReasoning,
	"tester":              CapabilityCodingToolUse,
	"ux-validator":        CapabilityVisionDesign,
	"validator":           CapabilityDeterministicTransform,
}

// canonicalAgentByCapability names the representative agent whose role stands
// in for a capability in legacy tier routes.
var canonicalAgentByCapability = map[string]string{
	CapabilityDeepReasoning:          "planner",
	CapabilityCodingToolUse:          "executor",
	CapabilityFastValidation:         "explorer",
	CapabilityVisionDesign:           "ux-validator",
	CapabilityIndependentDissent:     "reviewer",
	CapabilityDeterministicTransform: "validator",
}

// agentByOMPRole inverts the role naming rule for every matrix agent.
var agentByOMPRole = func() map[string]string {
	result := make(map[string]string, len(capabilityBySourceAgent))
	for agent := range capabilityBySourceAgent {
		result[OMPAgentRoleName(agent)] = agent
	}
	return result
}()

var legacyTierCapability = map[string]string{
	"fable":  CapabilityDeepReasoning,
	"opus":   CapabilityDeepReasoning,
	"sonnet": CapabilityCodingToolUse,
	"haiku":  CapabilityDeterministicTransform,
}

// OMPProviderNeutralCapabilities returns the canonical v1 capability order.
func OMPProviderNeutralCapabilities() []string {
	return append([]string(nil), providerNeutralCapabilities...)
}

// IsOMPNativeThinkingLevel reports whether OMP accepts a thinking value in
// model catalogs, role projections, and agent frontmatter.
func IsOMPNativeThinkingLevel(value string) bool {
	_, ok := ompNativeThinkingLevels[value]
	return ok
}

// OMPAgentRoleName derives the OMP model role owned by an agent. The rule is
// purely lexical; OMPAgentRole additionally requires a matrix agent.
func OMPAgentRoleName(agent string) string {
	return ompAgentRolePrefix + strings.ReplaceAll(agent, "-", "_")
}

// OMPAgentRole returns the OMP model role of a matrix agent.
func OMPAgentRole(agent string) (string, error) {
	if _, ok := capabilityBySourceAgent[agent]; !ok {
		return "", fmt.Errorf("agent_role_unmapped: %q", agent)
	}
	return OMPAgentRoleName(agent), nil
}

// OMPAgentCapability returns the provider-neutral capability of a matrix agent.
func OMPAgentCapability(agent string) (string, error) {
	capability, ok := capabilityBySourceAgent[agent]
	if !ok {
		return "", fmt.Errorf("agent_role_unmapped: %q", agent)
	}
	return capability, nil
}

// OMPRoleAgent returns the matrix agent that owns an autopus role.
func OMPRoleAgent(role string) (string, error) {
	agent, ok := agentByOMPRole[role]
	if !ok {
		return "", fmt.Errorf("role_unknown: %q", role)
	}
	return agent, nil
}

// OMPRoleCapability returns the capability behind an autopus role. OMP native
// role names are unknown here by design.
func OMPRoleCapability(role string) (string, error) {
	agent, err := OMPRoleAgent(role)
	if err != nil {
		return "", err
	}
	return capabilityBySourceAgent[agent], nil
}

// OMPAgentRoleMapping returns a detached agent-to-role copy of the matrix.
func OMPAgentRoleMapping() map[string]string {
	result := make(map[string]string, len(capabilityBySourceAgent))
	for agent := range capabilityBySourceAgent {
		result[agent] = OMPAgentRoleName(agent)
	}
	return result
}

// OMPAgentCapabilityMapping returns a detached agent-to-capability copy of the matrix.
func OMPAgentCapabilityMapping() map[string]string {
	result := make(map[string]string, len(capabilityBySourceAgent))
	for agent, capability := range capabilityBySourceAgent {
		result[agent] = capability
	}
	return result
}

// CanonicalOMPRoleForCapability returns the representative agent role of a capability.
func CanonicalOMPRoleForCapability(capability string) (string, error) {
	agent, ok := canonicalAgentByCapability[capability]
	if !ok {
		return "", fmt.Errorf("capability_unknown: %q", capability)
	}
	return OMPAgentRoleName(agent), nil
}

// ValidateRoleCapabilityPair rejects a role/capability pair that disagrees with the matrix.
func ValidateRoleCapabilityPair(role, capability string) error {
	want, err := OMPRoleCapability(role)
	if err != nil {
		return err
	}
	if want != capability {
		return fmt.Errorf("role_capability_mismatch: role %q requires %q, got %q", role, want, capability)
	}
	return nil
}

// ValidateOMPAgentRoleSet requires exact equality with the 17 generated agents.
func ValidateOMPAgentRoleSet(agents []string) error {
	seen := make(map[string]bool, len(agents))
	for _, agent := range agents {
		if _, err := OMPAgentRole(agent); err != nil {
			return err
		}
		if seen[agent] {
			return fmt.Errorf("agent_role_duplicate: %q", agent)
		}
		seen[agent] = true
	}
	missing := make([]string, 0)
	for agent := range capabilityBySourceAgent {
		if !seen[agent] {
			missing = append(missing, agent)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("agent_role_missing: %v", missing)
	}
	return nil
}

// LegacyTierRoute maps the v1 fable/opus/sonnet/haiku compatibility vocabulary
// onto a capability and that capability's representative agent role.
func LegacyTierRoute(version, tier string) (LegacyRoleRoute, error) {
	if version != RoleModelPolicyVersionV1 {
		return LegacyRoleRoute{}, fmt.Errorf("policy_version_unknown: %q", version)
	}
	capability, ok := legacyTierCapability[tier]
	if !ok {
		return LegacyRoleRoute{}, fmt.Errorf("legacy_tier_unknown: %q", tier)
	}
	role, err := CanonicalOMPRoleForCapability(capability)
	if err != nil {
		return LegacyRoleRoute{}, err
	}
	return LegacyRoleRoute{Capability: capability, Role: role, LegacySource: tier}, nil
}
