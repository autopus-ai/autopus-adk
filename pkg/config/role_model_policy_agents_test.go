package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestRoleModelPolicy_NativeRolesAreUnknown(t *testing.T) {
	t.Parallel()

	for _, native := range []string{"default", "smol", "slow", "plan", "vision", "designer", "commit", "tiny", "task", "advisor"} {
		if _, err := OMPRoleCapability(native); err == nil || !strings.Contains(err.Error(), "role_unknown") {
			t.Errorf("OMPRoleCapability(%q) = %v, want role_unknown", native, err)
		}
		if _, err := OMPRoleAgent(native); err == nil || !strings.Contains(err.Error(), "role_unknown") {
			t.Errorf("OMPRoleAgent(%q) = %v, want role_unknown", native, err)
		}
	}
	if _, err := OMPAgentCapability("future-agent"); err == nil || !strings.Contains(err.Error(), "agent_role_unmapped") {
		t.Fatalf("unmapped capability error = %v", err)
	}
	if got := OMPAgentRoleName("future-agent"); got != "autopus_future_agent" {
		t.Fatalf("lexical role name = %q", got)
	}
	if err := ValidateRoleCapabilityPair("autopus_reviewer", CapabilityCodingToolUse); err == nil || !strings.Contains(err.Error(), "role_capability_mismatch") {
		t.Fatalf("mismatched role/capability error = %v", err)
	}
}

func TestCanonicalOMPRoleForCapability_NamesRepresentativeAgent(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		CapabilityDeepReasoning:          "autopus_planner",
		CapabilityCodingToolUse:          "autopus_executor",
		CapabilityFastValidation:         "autopus_explorer",
		CapabilityVisionDesign:           "autopus_ux_validator",
		CapabilityIndependentDissent:     "autopus_reviewer",
		CapabilityDeterministicTransform: "autopus_validator",
	}
	for capability, wantRole := range want {
		gotRole, err := CanonicalOMPRoleForCapability(capability)
		if err != nil || gotRole != wantRole {
			t.Errorf("canonical role for %q = %q, %v; want %q", capability, gotRole, err, wantRole)
		}
		if gotCapability, err := OMPRoleCapability(gotRole); err != nil || gotCapability != capability {
			t.Errorf("representative role %q maps back to %q, %v", gotRole, gotCapability, err)
		}
	}
	if _, err := CanonicalOMPRoleForCapability("vendor_magic"); err == nil || !strings.Contains(err.Error(), "capability_unknown") {
		t.Fatalf("unknown capability error = %v", err)
	}
}

func TestRoleModelProfile_AgentCandidates_OverrideWinsPerAgent(t *testing.T) {
	t.Parallel()

	opus := RoleModelCandidateConf{Selector: "anthropic/claude-opus-5", Thinking: "xhigh", Family: "anthropic"}
	sonnet := RoleModelCandidateConf{Selector: "anthropic/claude-sonnet-5", Thinking: "medium", Family: "anthropic"}
	policy := validRoleModelPolicyFixture()
	profile := policy.Profiles[policy.Profile]
	profile.Capabilities[CapabilityCodingToolUse] = RoleCapabilityRouteConf{
		Candidates: []RoleModelCandidateConf{opus}, Required: true, DegradedAction: "runtime_default",
	}
	profile.Agents["executor"] = RoleAgentOverrideConf{Candidates: []RoleModelCandidateConf{sonnet}}
	policy.Profiles[policy.Profile] = profile
	if err := policy.Validate(); err != nil {
		t.Fatalf("Validate() rejected an agent candidate override: %v", err)
	}

	executor, err := profile.AgentCandidates("executor")
	if err != nil || !reflect.DeepEqual(executor, []RoleModelCandidateConf{sonnet}) {
		t.Fatalf("executor candidates = %#v, %v; want override", executor, err)
	}
	tester, err := profile.AgentCandidates("tester")
	if err != nil || !reflect.DeepEqual(tester, []RoleModelCandidateConf{opus}) {
		t.Fatalf("tester candidates = %#v, %v; want capability route", tester, err)
	}
	reviewer, err := profile.AgentCandidates("reviewer")
	if err != nil || !reflect.DeepEqual(reviewer, profile.Capabilities[CapabilityIndependentDissent].Candidates) {
		t.Fatalf("assertion-only override changed reviewer candidates: %#v, %v", reviewer, err)
	}

	route, err := profile.AgentRoute("executor")
	if err != nil {
		t.Fatalf("AgentRoute(executor): %v", err)
	}
	want := RoleCapabilityRouteConf{
		Candidates: []RoleModelCandidateConf{sonnet}, Required: true, DegradedAction: "runtime_default",
	}
	if !reflect.DeepEqual(route, want) {
		t.Fatalf("AgentRoute(executor) = %#v, want %#v", route, want)
	}
	route.Candidates[0].Thinking = "low"
	if profile.Agents["executor"].Candidates[0].Thinking != "medium" {
		t.Fatal("AgentRoute aliased the profile's override candidates")
	}
}

func TestRoleModelProfile_AgentCandidates_FailClosed(t *testing.T) {
	t.Parallel()

	profile := validRoleModelPolicyFixture().Profiles["p1"]
	if _, err := profile.AgentCandidates("future-agent"); err == nil || !strings.Contains(err.Error(), "agent_role_unmapped") {
		t.Fatalf("unmapped agent error = %v", err)
	}
	if _, err := profile.AgentCandidates("autopus_planner"); err == nil || !strings.Contains(err.Error(), "agent_role_unmapped") {
		t.Fatalf("role name as agent error = %v", err)
	}
	delete(profile.Capabilities, CapabilityVisionDesign)
	if _, err := profile.AgentCandidates("ux-validator"); err == nil || !strings.Contains(err.Error(), "capability_missing: vision_design") {
		t.Fatalf("missing capability error = %v", err)
	}
}

// A bundled OMP agent name is a valid agents-map key: it routes on its
// representative ADK role's capability while keeping its own override.
func TestRoleModelProfile_AgentRoute_AcceptsBundledAgentNames(t *testing.T) {
	t.Parallel()

	profile := validRoleModelPolicyFixture().Profiles["p1"]
	for agent, wantCapability := range map[string]string{
		"scout":             CapabilityFastValidation,
		"reviewer":          CapabilityIndependentDissent,
		"security-reviewer": CapabilityIndependentDissent,
		"task":              CapabilityDeepReasoning,
		"sonic":             CapabilityDeterministicTransform,
	} {
		route, err := profile.AgentRoute(agent)
		if err != nil {
			t.Fatalf("AgentRoute(%q): %v", agent, err)
		}
		if !reflect.DeepEqual(route.Candidates, profile.Capabilities[wantCapability].Candidates) {
			t.Fatalf("AgentRoute(%q) candidates = %#v", agent, route.Candidates)
		}
	}

	pinned := RoleModelCandidateConf{Selector: "anthropic/" + ClaudeHaikuModel, Thinking: "low", Family: "anthropic"}
	profile.Agents = map[string]RoleAgentOverrideConf{"sonic": {Candidates: []RoleModelCandidateConf{pinned}}}
	route, err := profile.AgentRoute("sonic")
	if err != nil || !reflect.DeepEqual(route.Candidates, []RoleModelCandidateConf{pinned}) {
		t.Fatalf("native override ignored: %#v, %v", route.Candidates, err)
	}
}
