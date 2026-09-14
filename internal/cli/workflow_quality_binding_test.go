package cli

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// S20: resolveTeamQualityBinding + serializeTeamQualityBinding produce the bare
// phase map the generated JS reads, with the ultra implementation/review values.
func TestResolveTeamQualityBinding_SerializesBarePhaseMap(t *testing.T) {
	t.Parallel()

	if teamQualityArgsKey != "quality" {
		t.Fatalf("teamQualityArgsKey = %q, want quality", teamQualityArgsKey)
	}

	b := resolveTeamQualityBinding("ultra", "")
	s, err := serializeTeamQualityBinding(b)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	var phases map[string]map[string]any
	if err := json.Unmarshal([]byte(s), &phases); err != nil {
		t.Fatalf("unmarshal bare phase map %q: %v", s, err)
	}

	planning, ok := phases["planning"]
	if !ok {
		t.Fatalf("missing planning entry in %q", s)
	}
	if planning["model"] != "claude-fable-5-1" || planning["effort"] != "max" {
		t.Fatalf("planning binding = %v, want claude-fable-5-1 + max", planning)
	}

	impl, ok := phases["implementation"]
	if !ok {
		t.Fatalf("missing implementation entry in %q", s)
	}
	if impl["model"] != "claude-opus-5" {
		t.Fatalf("implementation model = %v, want claude-opus-5", impl["model"])
	}
	if impl["effort"] != "max" {
		t.Fatalf("implementation effort = %v, want max", impl["effort"])
	}

	review, ok := phases["review"]
	if !ok {
		t.Fatalf("missing review entry in %q", s)
	}
	if vv, _ := review["verify_votes"].(float64); vv != 3 {
		t.Fatalf("review verify_votes = %v, want 3", review["verify_votes"])
	}
	if review["synthesis"] != true {
		t.Fatalf("review synthesis = %v, want true", review["synthesis"])
	}
}

func TestResolveTeamQualityBindingPreservesUltraAndUsesBalancedPlacement(t *testing.T) {
	t.Parallel()
	ultra := resolveTeamQualityBinding("ultra", "")
	impl := ultra.Phases["implementation"]
	if impl.Model != "claude-opus-5" || impl.Effort != "max" {
		t.Fatalf("ultra implementation changed: %+v", impl)
	}
	balanced := resolveTeamQualityBinding("balanced", "")
	for phase, want := range map[string]struct{ model, effort string }{
		"planning":       {"claude-fable-5-1", "max"},
		"implementation": {"claude-sonnet-5", "max"},
		"test_scaffold":  {"claude-sonnet-5", "max"},
		"testing":        {"claude-sonnet-5", "max"},
		"review":         {"claude-fable-5-1", "max"},
	} {
		got := balanced.Phases[phase]
		if got.Model != want.model || got.Effort != want.effort {
			t.Errorf("%s = %+v, want %s/%s", phase, got, want.model, want.effort)
		}
	}
	review := balanced.Phases["review"]
	if review.VerifyVotes != 1 || review.Synthesis {
		t.Fatalf("balanced review depth changed: %+v", review)
	}
}

func TestResolveWorkflowBinding_ExplicitEffortOverridesAllAgentPhases(t *testing.T) {
	t.Parallel()

	canonical := resolveTeamQualityBinding("balanced", "")
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		effort := effort
		t.Run(effort, func(t *testing.T) {
			t.Parallel()
			got := resolveWorkflowBinding(workflowBindingOptions{
				quality: "balanced", riskTier: "high", effort: effort,
			}, nil)
			phases := decodeBindingPhases(t, got.Quality)
			for phase, want := range canonical.Phases {
				want.Effort = effort
				if !reflect.DeepEqual(phases[phase], want) {
					t.Fatalf("phase %q = %+v, want %+v", phase, phases[phase], want)
				}
			}
			if got.SelectionReason != bindingReasonBalanced {
				t.Fatalf("selection reason = %q, want %q", got.SelectionReason, bindingReasonBalanced)
			}
		})
	}
}

func TestResolveWorkflowBinding_UltracodeNormalizesToXHigh(t *testing.T) {
	t.Parallel()

	got := resolveWorkflowBinding(workflowBindingOptions{
		quality: "balanced", riskTier: "high", effort: "ultracode",
	}, nil)
	for phase, binding := range decodeBindingPhases(t, got.Quality) {
		if binding.Effort != "xhigh" {
			t.Fatalf("phase %q effort = %q, want xhigh", phase, binding.Effort)
		}
	}
	if strings.Contains(string(got.Quality), "ultracode") {
		t.Fatalf("workflow quality contains session-only ultracode: %s", got.Quality)
	}
}

func TestResolveWorkflowBinding_InvalidBalancedEffortFailsClosedBeforeQualitySelection(t *testing.T) {
	t.Parallel()

	got := resolveWorkflowBinding(workflowBindingOptions{
		quality: "balanced", riskTier: "low", effort: "future-value",
	}, nil)
	want, err := serializeTeamQualityBinding(resolveTeamQualityBinding("ultra", ""))
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectionReason != bindingReasonValidationFailed {
		t.Fatalf("selection reason = %q, want %q", got.SelectionReason, bindingReasonValidationFailed)
	}
	if got.ReviewVotes != 3 || string(got.Quality) != want {
		t.Fatalf("invalid effort binding = votes %d quality %s, want canonical Full Ultra %s", got.ReviewVotes, got.Quality, want)
	}
	if strings.Contains(string(got.Quality), "future-value") {
		t.Fatalf("invalid effort leaked into quality: %s", got.Quality)
	}
}

func TestResolveWorkflowBinding_EmptyBalancedEffortPreservesCanonicalBytes(t *testing.T) {
	t.Parallel()

	got := resolveWorkflowBinding(workflowBindingOptions{
		quality: "balanced", riskTier: "critical", effort: "",
	}, nil)
	want, err := serializeTeamQualityBinding(resolveTeamQualityBinding("balanced", ""))
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectionReason != bindingReasonBalanced || string(got.Quality) != want {
		t.Fatalf("empty Balanced binding = reason %q quality %s, want %q / %s",
			got.SelectionReason, got.Quality, bindingReasonBalanced, want)
	}
}

func TestWorkflowBinding_CommandThreadsGlobalEffortFromContext(t *testing.T) {
	t.Parallel()

	cmd := newWorkflowBindingCmd(nil)
	cmd.SetContext(withGlobalFlags(context.Background(), globalFlags{Effort: "high"}))
	got := executeBindingCommand(t, cmd, "--quality", "balanced", "--risk-tier", "high")
	for phase, binding := range decodeBindingPhases(t, got.Quality) {
		if binding.Effort != "high" {
			t.Fatalf("phase %q effort = %q, want high", phase, binding.Effort)
		}
	}
}
