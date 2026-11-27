package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

	// A sub-agent's stop (no user input) is left to the parent's.
	assert.Equal(t, agyStopOutput{Decision: "stop"}, runAgyStop(t, payload(map[string]any{"lastUserInput": ""})))

	// Without cwd, the first workspace path is the tree.
	got := runAgyStop(t, payload(map[string]any{"cwd": ""}))
	assert.Equal(t, "continue", got.Decision)
	assert.Contains(t, got.Reason, "Verification failed")

	// A payload without the field (a later agy) still runs the gate.
	other := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a * b }\n")
	noField := payload(map[string]any{"cwd": other, "conversationId": session + "-nf"})
	delete(noField, "lastUserInput")
	t.Cleanup(func() { resetStopState(session + "-nf") })
	assert.Equal(t, "continue", runAgyStop(t, noField).Decision)
}

// The final reply comes from agy's transcript: the last model reply after
// the last user input.
func TestAgyLastReply(t *testing.T) {
	lines := []map[string]any{
		{"step_index": 0, "source": "USER_EXPLICIT", "type": "USER_INPUT", "content": "<USER_REQUEST>old</USER_REQUEST>"},
		{"step_index": 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "content": "old reply"},
		{"step_index": 2, "source": "USER_EXPLICIT", "type": "USER_INPUT", "content": "<USER_REQUEST>refactor</USER_REQUEST>"},
		{"step_index": 3, "source": "MODEL", "type": "PLANNER_RESPONSE", "content": "working"},
		{"step_index": 4, "source": "SYSTEM", "type": "SYSTEM_MESSAGE", "content": "notice"},
		{"step_index": 5, "source": "MODEL", "type": "PLANNER_RESPONSE", "content": "Done.\nTDD-exempt: pure refactor"},
	}
	var b strings.Builder
	for _, l := range lines {
		data, _ := json.Marshal(l)
		b.Write(data)
		b.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "transcript_full.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))
	assert.Equal(t, "Done.\nTDD-exempt: pure refactor", agyLastReply(path))

	require.NoError(t, os.WriteFile(path, []byte(`{"type":"USER_INPUT","source":"USER_EXPLICIT","content":"x"}`+"\n"), 0o600))
	assert.Empty(t, agyLastReply(path), "no reply yet in this turn")
	assert.Empty(t, agyLastReply(filepath.Join(t.TempDir(), "missing")))
	assert.Empty(t, agyLastReply(t.TempDir()), "a directory is not the transcript")
	assert.Empty(t, agyLastReply(""))
}

func TestVerifyStop_RejectsAnUnknownDialect(t *testing.T) {
	cmd := newVerifyStopCmd()
	cmd.SetIn(strings.NewReader("{}"))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--dialect", "x"})
	assert.Error(t, cmd.Execute())
}
