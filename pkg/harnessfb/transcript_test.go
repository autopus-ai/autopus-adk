package harnessfb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

// tx builds Claude Code transcript lines; at is seconds after t0.
type tx struct{ lines []string }

func (x *tx) add(at int, v map[string]any) *tx {
	v["timestamp"] = t0.Add(time.Duration(at) * time.Second).Format(time.RFC3339Nano)
	data, _ := json.Marshal(v)
	x.lines = append(x.lines, string(data))
	return x
}

func (x *tx) prompt(at int, text string) *tx {
	return x.add(at, map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}})
}

func (x *tx) use(at int, id, name string, input map[string]any) *tx {
	return x.add(at, map[string]any{"type": "assistant", "message": map[string]any{"content": []any{
		map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}}}})
}

func (x *tx) result(at int, id, text string) *tx {
	return x.add(at, map[string]any{"type": "user", "message": map[string]any{"content": []any{
		map[string]any{"type": "tool_result", "tool_use_id": id, "content": text}}}})
}

func (x *tx) notify(at int, id string) *tx {
	return x.prompt(at, fmt.Sprintf("<task-notification>\n<task-id>b1</task-id>\n<tool-use-id>%s</tool-use-id>\n<status>completed</status>\n</task-notification>", id))
}

func (x *tx) stopBlock(at int, reason string) *tx {
	return x.add(at, map[string]any{"type": "attachment", "attachment": map[string]any{
		"type": "hook_blocking_error", "hookEvent": "Stop", "toolUseID": "stop1",
		"blockingError": map[string]any{"blockingError": reason, "command": "auto verify language"}}})
}

func (x *tx) stopOK(at int, stop string) *tx {
	return x.add(at, map[string]any{"type": "attachment", "attachment": map[string]any{
		"type": "hook_success", "hookEvent": "Stop", "toolUseID": stop}})
}

func (x *tx) text() string { return strings.Join(x.lines, "\n") + "\n" }

func sessionTranscript() *tx {
	x := &tx{}
	x.prompt(0, "fix the bug")
	x.use(10, "t1", "Bash", map[string]any{"command": "go test ./pkg/..."}).result(70, "t1", "ok")
	x.use(80, "t2", "Bash", map[string]any{"command": "auto disk lease -- go test ./...", "run_in_background": true}).
		result(81, "t2", "Command running in background with ID: b1.")
	x.notify(200, "t2")
	x.use(210, "e1", "Edit", map[string]any{"file_path": "a.go"})
	x.use(220, "r1", "Agent", map[string]any{"subagent_type": "reviewer", "description": "Review diff"}).
		result(320, "r1", "LGTM\nagentId: abc123")
	// The second review of the turn resumes the same reviewer; nothing is
	// edited after it.
	x.use(330, "r2", "SendMessage", map[string]any{"to": "abc123", "message": "again"}).
		result(331, "r2", `{"success":true,"resumedAgentId":"abc123"}`)
	x.notify(400, "r2")
	x.stopBlock(401, "autopus.yaml sets language.ai_responses to ko, but a progress note")
	x.stopBlock(402, "autopus.yaml sets language.ai_responses to ko, but another note")
	x.stopOK(402, "stop1") // a sibling hook of the same stop
	x.stopOK(406, "stop2")
	x.use(410, "t3", "Bash", map[string]any{"command": "pytest -q"}).result(500, "t3", "Exit code 143\nCommand timed out after 1m 30s")
	// A sub-agent's own lines are not this session's.
	x.add(510, map[string]any{"type": "assistant", "isSidechain": true, "message": map[string]any{"content": []any{
		map[string]any{"type": "tool_use", "id": "s1", "name": "Bash", "input": map[string]any{"command": "go test ./x"}}}}})
	// An hour away, then the next turn.
	x.prompt(4110, "thanks")
	return x
}

func TestReadClaudeTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(sessionTranscript().text()), 0o600))
	s := Session{Metrics: Metrics{SessionID: "s"}}
	require.NoError(t, ReadClaudeTranscript(path, &s))

	assert.Equal(t, 2, s.Turns)
	assert.Equal(t, 3, s.TestRuns)
	assert.InDelta(t, 60+120+90, s.TestWaitSeconds, 0.01)
	assert.Equal(t, 2, s.Reviews)
	assert.InDelta(t, 100+70, s.ReviewSeconds, 0.01)
	assert.Equal(t, 1, s.RepeatReviews)
	assert.Equal(t, 1, s.RepeatReviewsNoFix, "nothing was edited after the second review")
	assert.InDelta(t, 70, s.RepeatNoFixSeconds, 0.01)
	assert.Equal(t, 2, s.StopBlocks)
	assert.Equal(t, map[string]int{"language": 2}, s.StopBlockKinds)
	assert.Equal(t, 1, s.Timeouts)
	assert.InDelta(t, 90, s.TimeoutSeconds, 0.01)
	assert.InDelta(t, 4, s.StopBlockSeconds["language"], 0.01, "from the end of the blocked stop to the next stop")
	assert.Contains(t, s.StopBlockSample["language"], "a progress note")
	assert.InDelta(t, 500, s.ActiveSeconds, 0.01, "the hour away and the sub-agent lines are not session time")
	assert.Empty(t, s.Open)
}

// A stop reads only what was appended since the last one, and a line
// still being written waits for the next read.
func TestReadClaudeTranscript_Incremental(t *testing.T) {
	full := sessionTranscript().text()
	cut := strings.Index(full, `"r2"`)
	cut = strings.LastIndex(full[:cut], "\n") + 20 // inside the r2 line
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(full[:cut]), 0o600))
	s := Session{Metrics: Metrics{SessionID: "s"}}
	require.NoError(t, ReadClaudeTranscript(path, &s))
	assert.Equal(t, 1, s.Reviews)

	data, err := json.Marshal(s)
	require.NoError(t, err)
	var again Session
	require.NoError(t, json.Unmarshal(data, &again))
	require.NoError(t, os.WriteFile(path, []byte(full), 0o600))
	require.NoError(t, ReadClaudeTranscript(path, &again))

	whole := Session{Metrics: Metrics{SessionID: "s"}}
	require.NoError(t, ReadClaudeTranscript(path, &whole))
	assert.Equal(t, whole.Metrics, again.Metrics)
}

// An edit after the second review means it found something to fix.
func TestReadClaudeTranscript_RepeatReviewThatFixed(t *testing.T) {
	x := &tx{}
	x.prompt(0, "go")
	x.use(1, "r1", "Agent", map[string]any{"subagent_type": "reviewer"}).result(50, "r1", "found a bug")
	x.use(60, "r2", "Agent", map[string]any{"description": "Re-review the fix"}).result(90, "r2", "ok")
	x.use(95, "b1", "Bash", map[string]any{"command": "sed -i '' 's/a/b/' x.go"})
	x.prompt(100, "next")
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(x.text()), 0o600))
	s := Session{}
	require.NoError(t, ReadClaudeTranscript(path, &s))
	assert.Equal(t, 1, s.RepeatReviews)
	assert.Zero(t, s.RepeatReviewsNoFix)
}

func TestClassify(t *testing.T) {
	for cmd, want := range map[string]string{
		"go test ./...":                 "test",
		"cd x && npm run test":          "test",
		"auto disk lease -- cargo test": "test",
		"echo go testing":               "bash",
		"git log --oneline":             "bash",
		"python3 -m pytest tests/ -q":   "test",
		"grep -n 'go test' Makefile":    "bash",
	} {
		assert.Equal(t, want, classify("Bash", toolInput{Command: cmd}, nil), cmd)
	}
	assert.Equal(t, "review", classify("Agent", toolInput{SubagentType: "reviewer"}, nil))
	assert.Equal(t, "agent", classify("Agent", toolInput{SubagentType: "Explore", Description: "find callers"}, nil))
	assert.Equal(t, "agent", classify("Agent", toolInput{SubagentType: "executor", Description: "Apply review and audit fixes"}, nil))
	assert.Equal(t, "agent", classify("Agent", toolInput{Description: "Fix review findings"}, nil))
	assert.Equal(t, "review", classify("Agent", toolInput{Description: "Verify-mode re-review of fixes"}, nil))
	assert.Equal(t, "review", classify("Agent", toolInput{SubagentType: "security-auditor", Description: "Security pass"}, nil))
	assert.Equal(t, "", classify("SendMessage", toolInput{To: "zzz"}, map[string]bool{"abc": true}))
}

// A commit ends a change: the review of the next change is not a repeat.
func TestReadClaudeTranscript_CommitEndsTheChange(t *testing.T) {
	x := &tx{}
	x.prompt(0, "ship both PRs")
	x.use(1, "r1", "Agent", map[string]any{"subagent_type": "reviewer"}).result(50, "r1", "ok")
	x.use(60, "c1", "Bash", map[string]any{"command": "git -C ../wt commit -q -F msg.txt"}).result(61, "c1", "")
	x.use(70, "r2", "Agent", map[string]any{"subagent_type": "reviewer"}).result(90, "r2", "ok")
	x.prompt(100, "next")
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(x.text()), 0o600))
	s := Session{}
	require.NoError(t, ReadClaudeTranscript(path, &s))
	assert.Equal(t, 2, s.Reviews)
	assert.Zero(t, s.RepeatReviews)
}

// Reviews run side by side are not repeats, and an agent sent to fix what
// a review found is not a review.
func TestReadClaudeTranscript_ParallelReviewsAndFixAgents(t *testing.T) {
	x := &tx{}
	x.prompt(0, "go")
	x.use(1, "r1", "Agent", map[string]any{"subagent_type": "security-auditor", "description": "Security review round 8"})
	x.use(2, "r2", "Agent", map[string]any{"subagent_type": "reviewer", "description": "Code review round 8"})
	x.result(40, "r1", "ok").result(50, "r2", "ok")
	x.use(60, "f1", "Agent", map[string]any{"subagent_type": "executor", "description": "Fix review findings"}).result(70, "f1", "done")
	x.prompt(80, "Another Claude session sent a message: <teammate-message>done</teammate-message>")
	x.use(90, "r3", "Agent", map[string]any{"description": "Verify-mode re-review of fixes"}).result(99, "r3", "ok")
	x.prompt(100, "next")
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(x.text()), 0o600))
	s := Session{}
	require.NoError(t, ReadClaudeTranscript(path, &s))
	assert.Equal(t, 3, s.Reviews)
	assert.Equal(t, 1, s.RepeatReviews, "only the re-review after both finished")
	assert.Equal(t, 2, s.Turns, "another agent's message is not a user turn")
}
