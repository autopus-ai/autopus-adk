package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

const ompModeDispatchBody = "```\n" +
	"Agent(subagent_type=\"executor\", prompt=\"a\")\n" +
	"Agent(subagent_type=\"explorer\", prompt=\"b\")\n" +
	"Agent(subagent_type=\"reviewer\", prompt=\"c\")\n" +
	"```\n"

// S14 (body): every dispatch names the Autopus agent of its own role; none
// collapses onto a bundled agent the way agent_source: native did before
// v0.50.131.
func TestReplacePlatformReferences_OMPDispatchNamesEachRolesOwnAgent(t *testing.T) {
	t.Parallel()
	rendered := content.ReplacePlatformReferences(ompModeDispatchBody, "omp")
	for _, agent := range []string{"executor", "explorer", "reviewer"} {
		assert.Contains(t, rendered, `"agent": "`+agent+`"`)
	}
	assert.NotContains(t, rendered, `"agent": "scout"`)
}

// A bundled name written into a source never reaches an agent autopus mode
// disables: scout and security-reviewer become the roles that replace them,
// and sonic falls to the unnamed default. Agent bodies name the Autopus
// agents too.
func TestReplacePlatformReferences_NeverDispatchesADisabledAgent(t *testing.T) {
	t.Parallel()
	body := "```\n" +
		"Agent(subagent_type=\"scout\", prompt=\"a\")\n" +
		"Agent(subagent_type=\"security-reviewer\", prompt=\"b\")\n" +
		"Agent(subagent_type=\"sonic\", prompt=\"c\")\n" +
		"```\n"
	rendered := content.ReplacePlatformReferences(body, "omp")
	for _, disabled := range config.OMPAutopusDisabledAgents() {
		assert.NotContains(t, rendered, `"agent": "`+disabled+`"`)
	}
	assert.Contains(t, rendered, `"agent": "explorer"`)
	assert.Contains(t, rendered, `"agent": "security-auditor"`)

	agent := content.NormalizeAgentReferences("Agent(subagent_type=\"executor\", prompt=\"a\")", "omp")
	assert.Contains(t, agent, `"agent": "executor"`, "agent bodies dispatch the Autopus agent")
}
