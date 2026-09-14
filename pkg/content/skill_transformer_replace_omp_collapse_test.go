package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/content"
)

// Every retired ADK role must resolve to a bundled agent, and reasoning roles
// must never land on the mechanical agent.
func TestReplacePlatformReferencesOMP_CollapsesEveryRetiredRoleOntoBundledAgents(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"explorer":            `"agent": "scout"`,
		"reviewer":            `"agent": "reviewer"`,
		"security-auditor":    `"agent": "security-reviewer"`,
		"planner":             "",
		"spec-writer":         "",
		"architect":           "",
		"executor":            "",
		"tester":              "",
		"validator":           "",
		"debugger":            "",
		"deep-worker":         "",
		"devops":              "",
		"annotator":           "",
		"ux-validator":        "",
		"frontend-specialist": "",
		"perf-engineer":       "",
	}
	for role, want := range cases {
		t.Run(role, func(t *testing.T) {
			t.Parallel()

			got := content.ReplacePlatformReferences(
				`Agent(subagent_type = "`+role+`", task = "Do the work.")`, "omp",
			)

			if want != `"agent": "`+role+`"` {
				// `reviewer` is both a retired role and a bundled agent, so only a
				// role that has to move must lose its own name.
				assert.NotContains(t, got, `"agent": "`+role+`"`)
			}
			assert.NotContains(t, got, `"agent": "sonic"`,
				"sonic is only for an explicit mechanical request")
			if want == "" {
				assert.NotContains(t, got, `"agent":`,
					"the bundled default agent is selected by omitting the field")
				return
			}
			assert.Contains(t, got, want)
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
