package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const englishNote = "Waiting on two things: the local tests and the CI run for the pull request."

func assistantText(text string) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}}}}
}

func userEntry(content any, meta bool) map[string]any {
	return map[string]any{"type": "user", "isMeta": meta, "message": map[string]any{"content": content}}
}

func toolResult(text string) map[string]any {
	return userEntry([]map[string]any{{"type": "tool_result", "content": text}}, false)
}

func writeTranscript(t *testing.T, entries ...map[string]any) string {
	t.Helper()
	var sb strings.Builder
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		require.NoError(t, err)
		sb.Write(line)
		sb.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(sb.String()), 0o600))
	return path
}

func turnInput(t *testing.T, cwd, transcript, last string) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(languageStopInput{Cwd: cwd, TranscriptPath: transcript, LastAssistantMessage: last})
	require.NoError(t, err)
	return strings.NewReader(string(data))
}

// The turn starts at the last prompt: English in an earlier turn, in tool
// output, in an injected meta message, or in a subagent's sidechain is not a
// reply to the user in this turn.
func TestTurnReplies_CollectsOnlyThisTurnsAssistantText(t *testing.T) {
	sidechain := assistantText("A subagent reports in English about its own work here.")
	sidechain["isSidechain"] = true
	path := writeTranscript(t,
		assistantText("An English reply from the turn before this one."),
		userEntry("다음 이슈를 고쳐줘", false),
		assistantText("첫 진행 메모입니다."),
		toolResult("go test output written in English by the tool"),
		userEntry([]map[string]any{{"type": "text", "text": "<system-reminder>injected</system-reminder>"}}, true),
		sidechain,
		assistantText(englishNote),
		assistantText("마지막 답변입니다."),
	)
	assert.Equal(t, []string{"첫 진행 메모입니다.", englishNote, "마지막 답변입니다."}, turnReplies(path))

	notified := writeTranscript(t,
		assistantText(englishNote),
		userEntry("<task-notification>done</task-notification>", false),
		assistantText("알림을 받고 이어서 진행합니다."),
	)
	assert.Equal(t, []string{"알림을 받고 이어서 진행합니다."}, turnReplies(notified), "a notification starts a new turn")
	assert.Nil(t, turnReplies(filepath.Join(t.TempDir(), "missing.jsonl")))
}

func TestRunVerifyLanguage_BlocksAnEnglishProgressNoteBeforeAKoreanReply(t *testing.T) {
	cwd := languageProject(t, "ko", nil)
	last := "두 PR을 모두 병합했습니다."
	path := writeTranscript(t, userEntry("이어서 해줘", false), assistantText(englishNote), assistantText(last))

	out := runVerifyLanguage(turnInput(t, cwd, path, last))
	assert.Equal(t, "block", out.Decision)
	assert.Contains(t, out.Reason, "progress note earlier in this turn")
	assert.Contains(t, out.Reason, "Waiting on two things")

	korean := writeTranscript(t, userEntry("이어서 해줘", false), assistantText("로컬 테스트를 기다립니다."), assistantText(last))
	assert.Empty(t, runVerifyLanguage(turnInput(t, cwd, korean, last)).Decision)
}

// The final reply is judged even when the transcript has not caught up with
// it, and a missing transcript falls back to the final reply alone.
func TestRunVerifyLanguage_FinalReplyIsCheckedWithoutTheTranscript(t *testing.T) {
	cwd := languageProject(t, "ko", nil)
	stale := writeTranscript(t, userEntry("이어서 해줘", false), assistantText("로컬 테스트를 기다립니다."))

	out := runVerifyLanguage(turnInput(t, cwd, stale, englishNote))
	assert.Equal(t, "block", out.Decision)
	assert.Contains(t, out.Reason, "your last reply")
	assert.Equal(t, "block", runVerifyLanguage(turnInput(t, cwd, "", englishNote)).Decision)
}

// The omp stop gate has no Claude Code transcript and sends the run's
// replies directly; they are checked the same way.
func TestRunVerifyLanguage_TurnRepliesFromThePayload(t *testing.T) {
	cwd := languageProject(t, "ko", nil)
	last := "두 PR을 모두 병합했습니다."
	data, err := json.Marshal(languageStopInput{Cwd: cwd, LastAssistantMessage: last, TurnReplies: []string{englishNote, last}})
	require.NoError(t, err)

	out := runVerifyLanguage(strings.NewReader(string(data)))
	assert.Equal(t, "block", out.Decision)
	assert.Contains(t, out.Reason, "Waiting on two things")
}
