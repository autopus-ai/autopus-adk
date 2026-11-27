package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func subagentEvent(t *testing.T, event, session, agent string) {
	t.Helper()
	var out bytes.Buffer
	in := strings.NewReader(`{"session_id":"` + session + `","agent_id":"` + agent + `","hook_event_name":"Subagent"}`)
	require.NoError(t, recordSubagent(in, &out, event))
	assert.Equal(t, "{}\n", out.String(), "Codex needs JSON on exit 0")
}

// Codex reports no background tasks: a sub-agent between SubagentStart and
// SubagentStop defers the gate the way a Claude Code background agent does.
func TestVerifyStop_DefersWhileACodexSubagentRuns(t *testing.T) {
	isolateStop(t)
	session := "cx-sub"
	resetStopState(session)
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")
	codex := map[string]any{"session_id": session, "cwd": dir, "turn_id": "t1", "model": "gpt-5", "stop_hook_active": false,
		"last_assistant_message": "done"}

	subagentEvent(t, "start", session, "agent/1")
	subagentEvent(t, "start", session, "agent/2")
	assert.Equal(t, 2, runningSubagents(session))
	out := runStopPayload(t, codex)
	assert.Empty(t, out.Decision)
	assert.Contains(t, out.SystemMessage, "background agent task")

	subagentEvent(t, "stop", session, "agent/1")
	subagentEvent(t, "stop", session, "agent/2")
	assert.Zero(t, runningSubagents(session))
	assert.Equal(t, "block", runStopPayload(t, codex).Decision, "with no sub-agent left the failing build blocks")
}

// A sub-agent whose SubagentStop never came stops counting after a while.
func TestRunningSubagents_ForgetsStaleMarkers(t *testing.T) {
	isolateStop(t)
	session := "cx-stale"
	subagentEvent(t, "start", session, "lost")
	old := time.Now().Add(-3 * time.Hour)
	require.NoError(t, os.Chtimes(stopStateFile(session, subagentKind("lost")), old, old))
	assert.Zero(t, runningSubagents(session))
	assert.NoFileExists(t, stopStateFile(session, subagentKind("lost")))

	var out bytes.Buffer
	assert.Error(t, recordSubagent(strings.NewReader(`{}`), &out, "pause"))
}

// A sub-agent whose SubagentStop never came (interrupted, or ended on an
// error) defers at most subagentMaxDefers stops, then the tree is verified.
func TestVerifyStop_ALostCodexSubagentStopsDeferring(t *testing.T) {
	isolateStop(t)
	session := "cx-lost"
	resetStopState(session)
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")
	codex := map[string]any{"session_id": session, "cwd": dir, "turn_id": "t1", "last_assistant_message": "done"}
	subagentEvent(t, "start", session, "interrupted")
	for i := 0; i < subagentMaxDefers; i++ {
		assert.Empty(t, runStopPayload(t, codex).Decision, "stop %d waits", i+1)
	}
	assert.Equal(t, "block", runStopPayload(t, codex).Decision, "then the failing build blocks")
	assert.NoFileExists(t, stopStateFile(session, subagentKind("interrupted")))
}
