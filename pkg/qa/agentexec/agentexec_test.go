package agentexec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-QALOOP-008: every target and mode builds exactly the REQ-8 argv.
func TestBuildPlan_TargetModeTable_MatchesREQ8(t *testing.T) {
	t.Parallel()
	const prompt = "write scenarios"
	const out = "/tmp/qa/last-message.txt"
	cases := []struct {
		target Target
		mode   Mode
		argv   []string
		stdin  string
		output string
	}{
		{TargetClaude, ModeGenerate, []string{"claude", "-p", "--output-format", "text"}, prompt, ""},
		{TargetClaude, ModeEdit, []string{"claude", "-p", "--permission-mode", "acceptEdits"}, prompt, ""},
		{TargetCodex, ModeGenerate, []string{"codex", "exec", "--skip-git-repo-check", "--sandbox", "read-only", "-o", out, "-"}, prompt, out},
		{TargetCodex, ModeEdit, []string{"codex", "exec", "--skip-git-repo-check", "--sandbox", "workspace-write", "-o", out, "-"}, prompt, out},
		{TargetGemini, ModeGenerate, []string{"agy", "-p", prompt}, "", ""},
		{TargetGemini, ModeEdit, []string{"agy", "-p", "--mode", "accept-edits", prompt}, "", ""},
		{TargetOpenCode, ModeGenerate, []string{"opencode", "run", prompt}, "", ""},
		{TargetOpenCode, ModeEdit, []string{"opencode", "run", prompt}, "", ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.target)+"/"+string(tc.mode), func(t *testing.T) {
			t.Parallel()
			plan, err := BuildPlan(tc.target, tc.mode, prompt, out, "")
			require.NoError(t, err)
			assert.Equal(t, tc.argv, plan.Argv)
			assert.Equal(t, tc.stdin, plan.Stdin)
			assert.Equal(t, tc.output, plan.OutputFile)
		})
	}
}

// AC-QALOOP-008: the override replaces the table and the prompt goes on stdin.
func TestBuildPlan_Override_ReplacesArgvAndMovesPromptToStdin(t *testing.T) {
	t.Parallel()
	const override = ` ["/opt/fake-agent", "--json", ""] `
	for _, target := range []Target{TargetClaude, TargetCodex, TargetGemini, TargetOpenCode} {
		for _, mode := range []Mode{ModeGenerate, ModeEdit} {
			plan, err := BuildPlan(target, mode, "fix the login test", "/tmp/out.txt", override)
			require.NoError(t, err, "%s/%s", target, mode)
			assert.Equal(t, Plan{Argv: []string{"/opt/fake-agent", "--json", ""}, Stdin: "fix the login test"}, plan, "%s/%s", target, mode)
		}
	}
}

func TestBuildPlan_InvalidOverride_ReportsOverrideInvalid(t *testing.T) {
	t.Parallel()
	for _, override := range []string{"claude -p", "[]", `[""]`, `["  "]`, "null", `{"argv":["x"]}`, `[1, 2]`, `["x"`} {
		_, err := BuildPlan(TargetClaude, ModeGenerate, "p", "", override)
		var reqErr *RequestError
		require.ErrorAs(t, err, &reqErr, "override %q", override)
		assert.Equal(t, "qa_agent_override_invalid", reqErr.Code, "override %q", override)
		assert.Contains(t, err.Error(), OverrideEnv)
	}
}

func TestBuildPlan_InvalidRequest_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		target Target
		mode   Mode
		prompt string
		output string
		code   string
	}{
		{"empty prompt", TargetClaude, ModeGenerate, " \n\t", "", CodeRequestInvalid},
		{"unknown mode", TargetClaude, Mode("review"), "p", "", CodeRequestInvalid},
		{"empty mode", TargetClaude, Mode(""), "p", "", CodeRequestInvalid},
		{"unknown target", Target("cursor"), ModeGenerate, "p", "", CodeTargetUnknown},
		{"target is case-sensitive here", Target("Claude"), ModeGenerate, "p", "", CodeTargetUnknown},
		{"codex without output file", TargetCodex, ModeGenerate, "p", "", CodeRequestInvalid},
	}
	for _, tc := range cases {
		_, err := BuildPlan(tc.target, tc.mode, tc.prompt, tc.output, "")
		require.Error(t, err, tc.name)
		assert.Equal(t, tc.code, ErrorCode(err), tc.name)
	}
}

func TestParseTarget(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]Target{
		"claude": TargetClaude, " Codex ": TargetCodex, "GEMINI": TargetGemini, "opencode": TargetOpenCode,
	} {
		got, err := ParseTarget(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "agy", "cursor"} {
		_, err := ParseTarget(in)
		assert.Equal(t, CodeTargetUnknown, ErrorCode(err), in)
	}
}

func TestErrors_CarryCodesThroughWrapping(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	cases := []struct {
		err  error
		code string
		text string
	}{
		{&SetupGapError{Code: CodeCLIMissing, Binary: "agy"}, "qa_agent_cli_missing", `"agy"`},
		{&AgentError{Code: CodeFailed, ExitCode: 2, Err: cause}, "qa_agent_failed", "status 2: boom"},
		{&AgentError{Code: CodeTimeout, ExitCode: -1}, "qa_agent_timeout", "status -1"},
		{&RequestError{Code: CodeOverrideInvalid, Err: cause}, "qa_agent_override_invalid", "boom"},
		{&RequestError{Code: CodeRequestInvalid}, "qa_agent_request_invalid", "qa_agent_request_invalid"},
	}
	for _, tc := range cases {
		wrapped := fmt.Errorf("qa loop: %w", tc.err)
		assert.Equal(t, tc.code, ErrorCode(wrapped))
		assert.Contains(t, wrapped.Error(), tc.code)
		assert.Contains(t, wrapped.Error(), tc.text)
	}
	assert.Empty(t, ErrorCode(cause))
	assert.Empty(t, ErrorCode(nil))
	assert.ErrorIs(t, fmt.Errorf("x: %w", &AgentError{Code: CodeFailed, Err: cause}), cause)
	assert.ErrorIs(t, fmt.Errorf("x: %w", &RequestError{Code: CodeOverrideInvalid, Err: cause}), cause)
}
