package agentexec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review finding 8: a prompt that starts with a dash stays a prompt. agy's -p
// takes the prompt as its value, so it is attached with "=" after --mode;
// opencode takes it positionally after "--".
func TestBuildPlan_PositionalPromptCannotBeReadAsAFlag(t *testing.T) {
	t.Parallel()
	const prompt = "--mode plan\nfix the login test"
	cases := []struct {
		target Target
		mode   Mode
		argv   []string
	}{
		{TargetGemini, ModeGenerate, []string{"agy", "-p=" + prompt}},
		{TargetGemini, ModeEdit, []string{"agy", "--mode", "accept-edits", "-p=" + prompt}},
		{TargetOpenCode, ModeGenerate, []string{"opencode", "run", "--", prompt}},
		{TargetOpenCode, ModeEdit, []string{"opencode", "run", "--", prompt}},
	}
	for _, tc := range cases {
		plan, err := BuildPlan(tc.target, tc.mode, prompt, "", "")
		require.NoError(t, err, "%s/%s", tc.target, tc.mode)
		assert.Equal(t, tc.argv, plan.Argv, "%s/%s", tc.target, tc.mode)
		assert.Empty(t, plan.Stdin)
	}
}

// Review finding 8: Linux refuses one argument over 128 KiB, so a prompt that
// would travel as argv is refused above 120 KiB; stdin targets are unbounded.
func TestBuildPlan_ArgvPromptOverTheCap_IsRefused(t *testing.T) {
	t.Parallel()
	atCap := strings.Repeat("x", 120*1024)
	overCap := atCap + "x"
	for _, target := range []Target{TargetGemini, TargetOpenCode} {
		for _, mode := range []Mode{ModeGenerate, ModeEdit} {
			_, err := BuildPlan(target, mode, overCap, "", "")
			assert.Equal(t, "qa_agent_prompt_too_large", ErrorCode(err), "%s/%s", target, mode)
			_, err = BuildPlan(target, mode, atCap, "", "")
			assert.NoError(t, err, "%s/%s", target, mode)
			_, err = BuildPlan(target, mode, overCap, "", `["/opt/fake-agent"]`)
			assert.NoError(t, err, "the override sends the prompt on stdin")
		}
	}
	for _, target := range []Target{TargetClaude, TargetCodex} {
		_, err := BuildPlan(target, ModeEdit, overCap, "/tmp/out.txt", "")
		assert.NoError(t, err, target)
	}
}
