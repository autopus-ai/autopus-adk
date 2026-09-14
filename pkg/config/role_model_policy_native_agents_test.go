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
		// scout and explorer configure the same agent the same way.
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

// Every ADK work role must land on some bundled agent, or generation would
// silently drop that role's model. None may land on sonic: it is the
// mechanical tier and is reachable only by naming it explicitly.
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

// The built-in profiles write one derived route per ADK agent. Those are
// defaults, not operator intent, so they must collapse through the
// representative instead of colliding.
func TestOMPNativeAgentRoute_BuiltinProfilesCollapseViaRepresentative(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"balanced", "ultra"} {
		for _, family := range RoleModelFamilies() {
			policy := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: name, Family: family}
			if err := policy.Validate(); err != nil {
				t.Fatalf("%s/%s policy rejected: %v", name, family, err)
			}
			_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
			if !ok {
				t.Fatalf("%s/%s profile unavailable", name, family)
			}
			if len(profile.OperatorAgents) != 0 {
				t.Fatalf("%s/%s marked derived routes as operator-written: %v", name, family, profile.OperatorAgents)
			}
			for _, native := range OMPNativeAgentNames() {
				resolved, route, err := profile.OMPNativeAgentRoute(native)
				if err != nil {
					t.Fatalf("%s/%s native route %q: %v", name, family, native, err)
				}
				representative, _ := OMPNativeAgentRepresentative(native)
				if resolved.Key != representative {
					t.Fatalf("%s/%s native route %q key = %q, want %q", name, family, native, resolved.Key, representative)
				}
				if len(route.Candidates) == 0 {
					t.Fatalf("%s/%s native route %q has no candidates", name, family, native)
				}
			}
		}
	}
}

// One explicit override wins over the built-in default, including an override
// written under an ADK role that is not the representative.
func TestOMPNativeAgentRoute_SingleOperatorOverrideWins(t *testing.T) {
	t.Parallel()

	pinned := RoleModelCandidateConf{Selector: "anthropic/" + ClaudeHaikuModel, Thinking: "low", Family: "anthropic"}
	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced",
		Agents: map[string]RoleAgentOverrideConf{
			"executor": {Candidates: []RoleModelCandidateConf{pinned}},
		},
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("single override rejected: %v", err)
	}
	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	if !ok {
		t.Fatal("balanced profile unavailable")
	}

	resolved, route, err := profile.OMPNativeAgentRoute("task")
	if err != nil {
		t.Fatalf("task native route: %v", err)
	}
	if resolved.Key != "executor" || resolved.Role != "autopus_executor" ||
		resolved.Capability != CapabilityCodingToolUse {
		t.Fatalf("override provenance lost: %+v", resolved)
	}
	if len(route.Candidates) != 1 || route.Candidates[0] != pinned {
		t.Fatalf("task route did not take the override: %#v", route.Candidates)
	}

	// Agents outside the override's group keep their representative default.
	scout, _, err := profile.OMPNativeAgentRoute("scout")
	if err != nil || scout.Key != "explorer" {
		t.Fatalf("scout route = %+v, %v", scout, err)
	}
}

// A full per-role policy names all 16 roles with different models, so two keys
// on one bundled agent is ordinary. The representative role decides, and the
// resolved key records which entry governs.
func TestOMPNativeAgentRoute_RepresentativeBreaksOperatorOverrideTie(t *testing.T) {
	t.Parallel()

	planned := RoleModelCandidateConf{
		Selector: "anthropic/" + ClaudeOpusModel, Thinking: "xhigh", Family: "anthropic",
	}
	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced",
		Agents: map[string]RoleAgentOverrideConf{
			"executor": {Candidates: []RoleModelCandidateConf{
				{Selector: "anthropic/" + ClaudeHaikuModel, Thinking: "low", Family: "anthropic"},
			}},
			"planner": {Candidates: []RoleModelCandidateConf{planned}},
		},
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("a policy with a representative row must resolve: %v", err)
	}

	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	if !ok {
		t.Fatal("balanced profile did not resolve")
	}
	resolved, route, err := profile.OMPNativeAgentRoute("task")
	if err != nil {
		t.Fatalf("task native route: %v", err)
	}
	if resolved.Key != "planner" || resolved.Role != "autopus_planner" {
		t.Fatalf("the representative row must govern: %+v", resolved)
	}
	if len(route.Candidates) != 1 || route.Candidates[0] != planned {
		t.Fatalf("task route did not take the representative model: %#v", route.Candidates)
	}
}

// Without the representative row nothing ranks the disagreeing entries, so the
// policy is refused instead of dropping one model at random.
func TestOMPNativeAgentRoute_RefusesConflictingOperatorOverrides(t *testing.T) {
	t.Parallel()

	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced",
		Agents: map[string]RoleAgentOverrideConf{
			"executor": {Candidates: []RoleModelCandidateConf{
				{Selector: "anthropic/" + ClaudeHaikuModel, Thinking: "low", Family: "anthropic"},
			}},
			"tester": {Candidates: []RoleModelCandidateConf{
				{Selector: "anthropic/" + ClaudeOpusModel, Thinking: "xhigh", Family: "anthropic"},
			}},
		},
	}

	err := policy.Validate()
	if err == nil {
		t.Fatal("conflicting overrides for one bundled agent were accepted")
	}
	for _, want := range []string{
		"omp_native_agent_conflict", `native agent "task"`, "[executor tester]",
		"keep exactly one entry", `prefer "task"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("conflict error %q is missing %q", err, want)
		}
	}
}

// Two keys that collapse onto one agent are fine when they ask for the same
// model: the operator has not expressed two incompatible intents.
func TestOMPNativeAgentRoute_AcceptsAgreeingOperatorOverrides(t *testing.T) {
	t.Parallel()

	pinned := RoleModelCandidateConf{Selector: "anthropic/" + ClaudeHaikuModel, Thinking: "low", Family: "anthropic"}
	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced",
		Agents: map[string]RoleAgentOverrideConf{
			"scout":    {Candidates: []RoleModelCandidateConf{pinned}},
			"explorer": {Candidates: []RoleModelCandidateConf{pinned}},
		},
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("agreeing overrides rejected: %v", err)
	}
	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	if !ok {
		t.Fatal("balanced profile unavailable")
	}
	resolved, route, err := profile.OMPNativeAgentRoute("scout")
	if err != nil {
		t.Fatalf("scout native route: %v", err)
	}
	if resolved.Key != "explorer" {
		t.Fatalf("agreeing keys must resolve deterministically, got %q", resolved.Key)
	}
	if len(route.Candidates) != 1 || route.Candidates[0] != pinned {
		t.Fatalf("scout route did not take the override: %#v", route.Candidates)
	}
}

// A custom profile's own agents map is operator intent, so it conflicts the
// same way the root overlay does.
func TestOMPNativeAgentRoute_CustomProfileOverridesAreOperatorIntent(t *testing.T) {
	t.Parallel()

	policy := validRoleModelPolicyFixture()
	profile := policy.Profiles["p1"]
	profile.Agents = map[string]RoleAgentOverrideConf{
		"tester": {Candidates: []RoleModelCandidateConf{
			{Selector: "acme/fast", Thinking: "low", Family: "acme"},
		}},
		"debugger": {Candidates: []RoleModelCandidateConf{
			{Selector: "acme/slow", Thinking: "high", Family: "acme"},
		}},
	}
	policy.Profiles["p1"] = profile

	err := policy.Validate()
	if err == nil || !strings.Contains(err.Error(), "omp_native_agent_conflict") {
		t.Fatalf("custom profile conflict error = %v", err)
	}
	if !strings.Contains(err.Error(), "[debugger tester]") {
		t.Fatalf("conflict error does not name both keys: %v", err)
	}
}

func TestErrOMPNativeAgentConflict_SortsKeysForStableGuidance(t *testing.T) {
	t.Parallel()

	err := ErrOMPNativeAgentConflict("task", []string{"planner", "executor", "tester"})
	want := `omp_native_agent_conflict: native agent "task" receives conflicting model routes ` +
		`from role_model_policy.agents entries [executor planner tester]; ` +
		`OMP registers one bundled "task" agent, so keep exactly one entry (prefer "task") ` +
		`and delete the rest`
	if err.Error() != want {
		t.Fatalf("conflict message =\n%s\nwant\n%s", err, want)
	}
}
