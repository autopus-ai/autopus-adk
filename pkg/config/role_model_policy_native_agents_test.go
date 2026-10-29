package config

import (
	"strings"
	"testing"
)

func TestResolveOMPPolicyAgent_CollapsesADKAndBundledKeys(t *testing.T) {
	t.Parallel()

	tests := map[string]OMPPolicyAgentRoute{
		"planner":   {Key: "planner", Native: "task", Role: "autopus_planner", Capability: CapabilityDeepReasoning},
		"executor":  {Key: "executor", Native: "task", Role: "autopus_executor", Capability: CapabilityCodingToolUse},
		"validator": {Key: "validator", Native: "task", Role: "autopus_validator", Capability: CapabilityDeterministicTransform},
		"explorer":  {Key: "explorer", Native: "scout", Role: "autopus_explorer", Capability: CapabilityFastValidation},
		"security-auditor": {Key: "security-auditor", Native: "security-reviewer",
			Role: "autopus_security_auditor", Capability: CapabilityIndependentDissent},
		// Bundled names borrow their representative's semantic identity, so
		// a policy written for the bundled agents keeps meaning the same
		// thing: scout and explorer configure the same route the same way.
		"scout":    {Key: "scout", Native: "scout", Role: "autopus_explorer", Capability: CapabilityFastValidation},
		"task":     {Key: "task", Native: "task", Role: "autopus_planner", Capability: CapabilityDeepReasoning},
		"sonic":    {Key: "sonic", Native: "sonic", Role: "autopus_validator", Capability: CapabilityDeterministicTransform},
		"reviewer": {Key: "reviewer", Native: "reviewer", Role: "autopus_reviewer", Capability: CapabilityIndependentDissent},
		"security-reviewer": {Key: "security-reviewer", Native: "security-reviewer",
			Role: "autopus_security_auditor", Capability: CapabilityIndependentDissent},
	}
	for key, want := range tests {
		got, err := ResolveOMPPolicyAgent(key)
		if err != nil || got != want {
			t.Errorf("ResolveOMPPolicyAgent(%q) = %+v, %v; want %+v", key, got, err, want)
		}
	}

	for _, key := range []string{"", "future-agent", "autopus_planner", "smol", "default"} {
		if _, err := ResolveOMPPolicyAgent(key); err == nil || !strings.Contains(err.Error(), "agent_role_unmapped") {
			t.Errorf("ResolveOMPPolicyAgent(%q) error = %v, want agent_role_unmapped", key, err)
		}
	}
}

// Every ADK work role names the bundled agent it collapsed onto, or the Route
// Contract's rule (2) could not route it on a bundled-name pin. None may land
// on sonic: it is the mechanical tier and is reachable only by naming it
// explicitly.
func TestResolveOMPPolicyAgent_CoversEveryCanonicalAgent(t *testing.T) {
	t.Parallel()

	natives := make(map[string]bool, len(OMPNativeAgentNames()))
	for _, native := range OMPNativeAgentNames() {
		natives[native] = true
	}
	for _, agent := range CanonicalAgentNames() {
		resolved, err := ResolveOMPPolicyAgent(agent)
		if err != nil {
			t.Fatalf("ResolveOMPPolicyAgent(%q): %v", agent, err)
		}
		if !natives[resolved.Native] {
			t.Fatalf("agent %q collapsed onto unknown native agent %q", agent, resolved.Native)
		}
		if resolved.Native == "sonic" {
			t.Fatalf("ADK agent %q must not route to the mechanical sonic agent", agent)
		}
	}
}

// A custom profile's own agents map is operator intent: two entries that
// native mode refused as a conflict on its one task slot each pin their own
// agent, and the agents without an entry keep the profile route.
func TestOMPAgentRoute_CustomProfileOverridesAreOperatorIntent(t *testing.T) {
	t.Parallel()

	fast := RoleModelCandidateConf{Selector: "acme/fast", Thinking: "low", Family: "acme"}
	slow := RoleModelCandidateConf{Selector: "acme/slow", Thinking: "high", Family: "acme"}
	policy := validRoleModelPolicyFixture()
	profile := policy.Profiles["p1"]
	profile.Agents = map[string]RoleAgentOverrideConf{
		"tester":   {Candidates: []RoleModelCandidateConf{fast}},
		"debugger": {Candidates: []RoleModelCandidateConf{slow}},
	}
	policy.Profiles["p1"] = profile
	if err := policy.Validate(); err != nil {
		t.Fatalf("custom profile overrides rejected: %v", err)
	}
	_, resolved, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	if !ok {
		t.Fatal("custom profile unavailable")
	}
	for agent, want := range map[string]RoleModelCandidateConf{"tester": fast, "debugger": slow} {
		identity, route, err := resolved.OMPAgentRoute(agent)
		if err != nil || identity.Key != agent || len(route.Candidates) != 1 || route.Candidates[0] != want {
			t.Fatalf("%s route = %+v %#v, %v; want its own pin %+v", agent, identity, route.Candidates, err, want)
		}
		if !resolved.OMPAgentOperatorPinned(agent) {
			t.Fatalf("%s pin is not operator intent", agent)
		}
	}
	if resolved.OMPAgentOperatorPinned("executor") {
		t.Fatal("an agent without an entry is not pinned")
	}
}
