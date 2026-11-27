package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runAgyStop(t *testing.T, payload map[string]any) agyStopOutput {
	t.Helper()
	isolateStop(t)
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	cmd := newVerifyStopCmd()
	cmd.SetIn(bytes.NewReader(data))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--dialect", "agy", "--budget", "2m"})
	require.NoError(t, cmd.Execute())
	var got agyStopOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	return got
}

// An Antigravity Stop on a failing tree sends the agent back with
// "continue"; it waits while work still runs and lets a stop the model did
// not choose end.
func TestVerifyStopAgy(t *testing.T) {
	session := "agy-" + strings.ReplaceAll(t.Name(), "/", "_")
	resetStopState(session)
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")
	payload := func(extra map[string]any) map[string]any {
		// The shape agy 1.5.18 sends (captured through its LLM gateway).
		p := map[string]any{"conversationId": session, "cwd": dir, "workspacePaths": []string{dir}, "executionNum": 0,
			"terminationReason": "NO_TOOL_CALL", "error": "", "fullyIdle": true, "continuationActive": false,
			"hookEventName": "Stop", "transcriptPath": "/x/agy", "lastUserInput": "fix it", "modelName": "m"}
		for k, v := range extra {
			p[k] = v
		}
		return p
	}

	assert.Equal(t, agyStopOutput{Decision: "stop"}, runAgyStop(t, payload(map[string]any{"fullyIdle": false})),
		"background work still runs")
	for _, reason := range []string{"MAX_INVOCATIONS", "ERROR", "USER_CANCELED"} {
		assert.Equal(t, agyStopOutput{Decision: "stop"}, runAgyStop(t, payload(map[string]any{"terminationReason": reason})), reason)
	}

	got := runAgyStop(t, payload(nil))
	assert.Equal(t, "continue", got.Decision)
	assert.Contains(t, got.Reason, "Verification failed")
}

func TestVerifyStop_RejectsAnUnknownDialect(t *testing.T) {
	cmd := newVerifyStopCmd()
	cmd.SetIn(strings.NewReader("{}"))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--dialect", "x"})
	assert.Error(t, cmd.Execute())
}
