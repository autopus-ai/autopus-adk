package workflow

import "testing"

func TestIsSafeAgentModel_CurrentAndLegacyTopModelsAreAllowed(t *testing.T) {
	t.Parallel()

	for _, model := range []string{
		"claude-fable-5-1",
		"claude-fable-5",
		"claude-opus-5-5",
		"claude-opus-5",
		"claude-opus-4-8",
	} {
		if !isSafeAgentModel(model) {
			t.Errorf("isSafeAgentModel(%q) = false, want true", model)
		}
	}
	if isSafeAgentModel(`claude-fable-5-1");evil((`) {
		t.Error("Fable 5.1 injection payload must be rejected")
	}
}
