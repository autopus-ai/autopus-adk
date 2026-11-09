package generate

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
)

// An answer that yields nothing is saved where a person can read it, and a
// YAML document that does not parse says why.
func TestRun_SavesARejectedReplyWithTheParseError(t *testing.T) {
	t.Parallel()
	dir := projectX(t, acceptanceX)
	reply := "```yaml\nschema_version: qamesh.test-scenarios.v1\ncases:\n  - id: a\n    then: prints: 3 the\n```\n"
	report, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: specX, Target: agentexec.TargetGemini,
		Runner: &fakeRunner{stdout: reply}})
	require.NoError(t, err)
	require.NotEmpty(t, report.Rejected)
	assert.Equal(t, CodeYAMLInvalid, report.Rejected[0].Code)
	assert.Contains(t, report.Rejected[0].Message, "mapping values are not allowed")
	require.NotEmpty(t, report.ReplyPath)
	saved, err := os.ReadFile(report.ReplyPath)
	require.NoError(t, err)
	assert.Equal(t, reply, string(saved))
}

func TestRun_FeedbackReachesThePrompt(t *testing.T) {
	t.Parallel()
	agent := &fakeRunner{stdout: "nothing"}
	_, err := Run(context.Background(), Options{ProjectDir: projectX(t, acceptanceX), SpecID: specX, Target: agentexec.TargetGemini,
		Runner: agent, Feedback: "- qa_generate_yaml_invalid: line 5: mapping values are not allowed"})
	require.NoError(t, err)
	assert.Contains(t, agent.got.Prompt, "Your previous answer to this request was rejected:\n- qa_generate_yaml_invalid: line 5")
	assert.Contains(t, agent.got.Prompt, "double-quote any string that holds a colon")
}
