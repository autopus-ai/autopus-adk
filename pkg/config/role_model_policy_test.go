package config

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRoleModelPolicy_ZeroValue_OmitsYAMLAndPreservesValidation(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("zero-policy")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if strings.Contains(string(data), "role_model_policy:") {
		t.Fatalf("zero-value policy changed default YAML:\n%s", data)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero-value policy invalidated default config: %v", err)
	}
}

func TestRoleModelPolicy_SelectedProfile_RoundTrips(t *testing.T) {
	t.Parallel()

	want := validRoleModelPolicyFixture()
	cfg := DefaultFullConfig("routing")
	cfg.RoleModelPolicy = want
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var got HarnessConfig
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if !reflect.DeepEqual(got.RoleModelPolicy, want) {
		t.Fatalf("policy round-trip mismatch\ngot:  %#v\nwant: %#v", got.RoleModelPolicy, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("round-tripped config invalid: %v", err)
	}
	name, profile, ok := got.RoleModelPolicy.SelectedRoleModelProfile()
	if !ok || name != "p1" || profile.ConfigMode != RoleModelConfigModeOverlay {
		t.Fatalf("selected profile = %q %#v %v", name, profile, ok)
	}
}

// policyContractMatrix mirrors the SPEC-OMP-005 Policy Contract table.
var policyContractMatrix = []struct {
	agent, role, capability string
}{
	{"annotator", "autopus_annotator", CapabilityFastValidation},
	{"architect", "autopus_architect", CapabilityDeepReasoning},
	{"debugger", "autopus_debugger", CapabilityCodingToolUse},
	{"deep-worker", "autopus_deep_worker", CapabilityCodingToolUse},
	{"devops", "autopus_devops", CapabilityCodingToolUse},
	{"executor", "autopus_executor", CapabilityCodingToolUse},
	{"explorer", "autopus_explorer", CapabilityFastValidation},
	{"frontend-specialist", "autopus_frontend_specialist", CapabilityVisionDesign},
	{"perf-engineer", "autopus_perf_engineer", CapabilityCodingToolUse},
	{"planner", "autopus_planner", CapabilityDeepReasoning},
	{"reverse-engineer", "autopus_reverse_engineer", CapabilityDeepReasoning},
	{"reviewer", "autopus_reviewer", CapabilityIndependentDissent},
	{"security-auditor", "autopus_security_auditor", CapabilityIndependentDissent},
	{"spec-writer", "autopus_spec_writer", CapabilityDeepReasoning},
	{"tester", "autopus_tester", CapabilityCodingToolUse},
	{"ux-validator", "autopus_ux_validator", CapabilityVisionDesign},
	{"validator", "autopus_validator", CapabilityDeterministicTransform},
}

func TestRoleModelPolicy_ProjectionMatrix_IsExact(t *testing.T) {
	t.Parallel()

	wantRoles := make(map[string]string, len(policyContractMatrix))
	wantCapabilities := make(map[string]string, len(policyContractMatrix))
	for _, row := range policyContractMatrix {
		wantRoles[row.agent] = row.role
		wantCapabilities[row.agent] = row.capability
		if got := OMPAgentRoleName(row.agent); got != row.role {
			t.Errorf("OMPAgentRoleName(%q) = %q, want %q", row.agent, got, row.role)
		}
		if got, err := OMPAgentRole(row.agent); err != nil || got != row.role {
			t.Errorf("OMPAgentRole(%q) = %q, %v; want %q", row.agent, got, err, row.role)
		}
		if got, err := OMPAgentCapability(row.agent); err != nil || got != row.capability {
			t.Errorf("OMPAgentCapability(%q) = %q, %v; want %q", row.agent, got, err, row.capability)
		}
		if got, err := OMPRoleAgent(row.role); err != nil || got != row.agent {
			t.Errorf("OMPRoleAgent(%q) = %q, %v; want %q", row.role, got, err, row.agent)
		}
		if got, err := OMPRoleCapability(row.role); err != nil || got != row.capability {
			t.Errorf("OMPRoleCapability(%q) = %q, %v; want %q", row.role, got, err, row.capability)
		}
		if err := ValidateRoleCapabilityPair(row.role, row.capability); err != nil {
			t.Errorf("ValidateRoleCapabilityPair(%q, %q) = %v", row.role, row.capability, err)
		}
	}
	if got := OMPAgentRoleMapping(); !reflect.DeepEqual(got, wantRoles) {
		t.Fatalf("agent role mapping mismatch\ngot:  %#v\nwant: %#v", got, wantRoles)
	}
	if got := OMPAgentCapabilityMapping(); !reflect.DeepEqual(got, wantCapabilities) {
		t.Fatalf("agent capability mapping mismatch\ngot:  %#v\nwant: %#v", got, wantCapabilities)
	}
	// The matrix is the role vocabulary: the generated agents plus the legacy
	// roles the binary still dispatches without generating an agent for them.
	canonical := append(CanonicalAgentNames(), legacyAgentNames...)
	sort.Strings(canonical)
	if got := mapKeys(wantRoles); !reflect.DeepEqual(got, canonical) {
		t.Fatalf("matrix agents %v differ from the role vocabulary %v", got, canonical)
	}
	if err := ValidateOMPAgentRoleSet(mapKeys(wantRoles)); err != nil {
		t.Fatalf("exact agent set rejected: %v", err)
	}
	if err := ValidateOMPAgentRoleSet(append(mapKeys(wantRoles), "future-agent")); err == nil || !strings.Contains(err.Error(), "agent_role_unmapped") {
		t.Fatalf("unmapped agent error = %v", err)
	}
}

func TestLegacyTierRoute_V1_FailsClosedForUnknownTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier       string
		capability string
		role       string
	}{
		{"fable", CapabilityDeepReasoning, "autopus_planner"},
		{"opus", CapabilityDeepReasoning, "autopus_planner"},
		{"sonnet", CapabilityCodingToolUse, "autopus_executor"},
		{"haiku", CapabilityDeterministicTransform, "autopus_validator"},
	}
	for _, tt := range tests {
		got, err := LegacyTierRoute(RoleModelPolicyVersionV1, tt.tier)
		if err != nil {
			t.Fatalf("route %q: %v", tt.tier, err)
		}
		if got.Capability != tt.capability || got.Role != tt.role || got.LegacySource != tt.tier {
			t.Errorf("route %q = %#v", tt.tier, got)
		}
	}
	if _, err := LegacyTierRoute(RoleModelPolicyVersionV1, "turbo"); err == nil || !strings.Contains(err.Error(), "legacy_tier_unknown") {
		t.Fatalf("unknown legacy tier error = %v", err)
	}
	if _, err := LegacyTierRoute("v2", "opus"); err == nil || !strings.Contains(err.Error(), "policy_version_unknown") {
		t.Fatalf("unknown policy version error = %v", err)
	}
}

func TestRoleModelPolicy_Validation_FailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*HarnessConfig)
		code   string
	}{
		{"missing profile", func(c *HarnessConfig) { c.RoleModelPolicy.Profile = "missing" }, "profile_unknown"},
		{"unknown capability", func(c *HarnessConfig) {
			p := c.RoleModelPolicy.Profiles["p1"]
			p.Capabilities["vendor_magic"] = RoleCapabilityRouteConf{Candidates: []RoleModelCandidateConf{{Selector: "acme/model"}}}
			c.RoleModelPolicy.Profiles["p1"] = p
		}, "capability_unknown"},
		{"empty candidate", func(c *HarnessConfig) {
			p := c.RoleModelPolicy.Profiles["p1"]
			p.Capabilities[CapabilityDeepReasoning] = RoleCapabilityRouteConf{Candidates: []RoleModelCandidateConf{{}}}
			c.RoleModelPolicy.Profiles["p1"] = p
		}, "selector_invalid"},
		{"unknown agent", func(c *HarnessConfig) {
			p := c.RoleModelPolicy.Profiles["p1"]
			p.Agents["future-agent"] = RoleAgentOverrideConf{}
			c.RoleModelPolicy.Profiles["p1"] = p
		}, "agent_role_unmapped"},
		{"override role off matrix", func(c *HarnessConfig) {
			p := c.RoleModelPolicy.Profiles["p1"]
			p.Agents["executor"] = RoleAgentOverrideConf{Role: "autopus_planner"}
			c.RoleModelPolicy.Profiles["p1"] = p
		}, "agents[executor].role_capability_mismatch"},
		{"override capability off matrix", func(c *HarnessConfig) {
			p := c.RoleModelPolicy.Profiles["p1"]
			p.Agents["executor"] = RoleAgentOverrideConf{Capability: CapabilityDeepReasoning}
			c.RoleModelPolicy.Profiles["p1"] = p
		}, "agents[executor].role_capability_mismatch"},
		{"override native role", func(c *HarnessConfig) {
			p := c.RoleModelPolicy.Profiles["p1"]
			p.Agents["executor"] = RoleAgentOverrideConf{Role: "task", Capability: CapabilityCodingToolUse}
			c.RoleModelPolicy.Profiles["p1"] = p
		}, "agents[executor].role_capability_mismatch"},
		{"override candidate selector", func(c *HarnessConfig) {
			p := c.RoleModelPolicy.Profiles["p1"]
			p.Agents["executor"] = RoleAgentOverrideConf{Candidates: []RoleModelCandidateConf{{Selector: "acme"}}}
			c.RoleModelPolicy.Profiles["p1"] = p
		}, "agents[executor].candidates[0].selector_invalid"},
		{"override candidate thinking", func(c *HarnessConfig) {
			p := c.RoleModelPolicy.Profiles["p1"]
			p.Agents["executor"] = RoleAgentOverrideConf{Candidates: []RoleModelCandidateConf{{Selector: "acme/model", Thinking: "turbo"}}}
			c.RoleModelPolicy.Profiles["p1"] = p
		}, "agents[executor].candidates[0].metadata_invalid"},
		{"omp quality provider", func(c *HarnessConfig) {
			c.Quality.Providers = map[string]string{"omp": "balanced"}
		}, "quality.providers provider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultFullConfig("invalid-policy")
			cfg.RoleModelPolicy = validRoleModelPolicyFixture()
			tt.mutate(cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tt.code) {
				t.Fatalf("Validate() error = %v, want code %q", err, tt.code)
			}
		})
	}
}

func validRoleModelPolicyFixture() RoleModelPolicyConf {
	routes := make(map[string]RoleCapabilityRouteConf, len(OMPProviderNeutralCapabilities()))
	for _, capability := range OMPProviderNeutralCapabilities() {
		routes[capability] = RoleCapabilityRouteConf{
			Required:   true,
			Candidates: []RoleModelCandidateConf{{Selector: "acme/model", Thinking: "high", Family: "acme"}},
		}
	}
	return RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1,
		Profile: "p1",
		Profiles: map[string]RoleModelProfileConf{
			"p1": {
				ConfigMode:   RoleModelConfigModeOverlay,
				Capabilities: routes,
				Agents: map[string]RoleAgentOverrideConf{
					"reviewer": {Role: "autopus_reviewer", Capability: CapabilityIndependentDissent},
				},
				FamilyDiversity: FamilyDiversityPolicyConf{Enabled: true, Roles: []string{"autopus_reviewer"}},
				Safety:          RoleSafetyPolicyConf{ApprovalMode: "write", IsolationMode: "auto"},
			},
		},
	}
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
