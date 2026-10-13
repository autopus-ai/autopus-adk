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

var ompNativeReferences = content.PlatformReferenceOptions{OMPAgentSource: config.OMPAgentSourceNative}

// S14 (body): autopus mode dispatches each role to its own agent; native
// collapses executor onto the unnamed task default and explorer onto scout.
func TestReplacePlatformReferencesWith_OMPDispatchFollowsTheAgentSource(t *testing.T) {
	t.Parallel()
	autopus := content.ReplacePlatformReferencesWith(ompModeDispatchBody, "omp", content.PlatformReferenceOptions{})
	for _, agent := range []string{"executor", "explorer", "reviewer"} {
		assert.Contains(t, autopus, `"agent": "`+agent+`"`)
	}
	assert.NotContains(t, autopus, `"agent": "scout"`)
	assert.Equal(t, autopus, content.ReplacePlatformReferences(ompModeDispatchBody, "omp"), "autopus is the default")

	native := content.ReplacePlatformReferencesWith(ompModeDispatchBody, "omp", ompNativeReferences)
	assert.NotContains(t, native, `"agent": "executor"`)
	assert.Contains(t, native, `"agent": "scout"`)
	assert.Contains(t, native, `"agent": "reviewer"`)
}

// A bundled name written into a source never reaches an agent autopus mode
// disables: scout and security-reviewer become the roles that replace them,
// and sonic falls to the unnamed default.
func TestReplacePlatformReferencesWith_AutopusNeverDispatchesADisabledAgent(t *testing.T) {
	t.Parallel()
	body := "```\n" +
		"Agent(subagent_type=\"scout\", prompt=\"a\")\n" +
		"Agent(subagent_type=\"security-reviewer\", prompt=\"b\")\n" +
		"Agent(subagent_type=\"sonic\", prompt=\"c\")\n" +
		"```\n"
	rendered := content.ReplacePlatformReferencesWith(body, "omp", content.PlatformReferenceOptions{})
	for _, disabled := range config.OMPAutopusDisabledAgents() {
		assert.NotContains(t, rendered, `"agent": "`+disabled+`"`)
	}
	assert.Contains(t, rendered, `"agent": "explorer"`)
	assert.Contains(t, rendered, `"agent": "security-auditor"`)

	agent := content.NormalizeAgentReferencesWith("Agent(subagent_type=\"executor\", prompt=\"a\")", "omp", ompNativeReferences)
	assert.NotContains(t, agent, `"agent": "executor"`, "agent bodies follow the native option too")
}
