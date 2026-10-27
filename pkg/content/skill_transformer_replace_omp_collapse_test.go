package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// Every canonical role dispatches its own Autopus agent, so no retired
// per-role name collapses onto a bundled agent any more, and no role lands on
// the mechanical sonic agent or a disabled bundled agent.
func TestReplacePlatformReferencesOMP_DispatchesEveryCanonicalRoleUnderItsOwnName(t *testing.T) {
	t.Parallel()

	for _, role := range config.CanonicalAgentNames() {
		t.Run(role, func(t *testing.T) {
			t.Parallel()

			got := content.ReplacePlatformReferences(
				`Agent(subagent_type = "`+role+`", task = "Do the work.")`, "omp",
			)

			assert.Contains(t, got, `"agent": "`+role+`"`)
			for _, disabled := range config.OMPAutopusDisabledAgents() {
				assert.NotContains(t, got, `"agent": "`+disabled+`"`)
			}
		})
	}
}

// A name outside the retired role catalog may be a real project agent, so it
// is left alone rather than rewritten into a bundled name.
func TestReplacePlatformReferencesOMP_KeepsUnknownProjectAgentName(t *testing.T) {
	t.Parallel()

	got := content.ReplacePlatformReferences(
		`Agent(subagent_type = "release-captain", task = "Cut the release.")`, "omp",
	)

	assert.Contains(t, got, `"agent": "release-captain"`)
}
