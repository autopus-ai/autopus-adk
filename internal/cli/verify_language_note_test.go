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

// noteTranscript writes a turn whose last progress note is note.
func noteTranscript(t *testing.T, note string) string {
	t.Helper()
	lines := []map[string]any{
		{"type": "user", "message": map[string]any{"role": "user", "content": "작업해 줘"}},
		{"type": "assistant", "message": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "먼저 코드를 살펴봅니다."},
			map[string]any{"type": "tool_use", "id": "t1", "name": "Bash", "input": map[string]any{"command": "ls"}}}}},
		{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": "a.go"}}}},
		{"type": "assistant", "message": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": note},
			map[string]any{"type": "tool_use", "id": "t2", "name": "Bash", "input": map[string]any{"command": "go test ./..."}}}}},
	}
	var b strings.Builder
	for _, l := range lines {
		data, _ := json.Marshal(l)
		b.Write(data)
		b.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))
	return path
}

func noteInput(t *testing.T, session, cwd, transcript string) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(languageNoteInput{SessionID: session, Cwd: cwd, TranscriptPath: transcript})
	require.NoError(t, err)
	return strings.NewReader(string(data))
}

// The first note in another language gets one reminder at once; the same
// note does not get a second, and a note in the configured language none.
func TestRunLanguageNote(t *testing.T) {
	isolateStop(t)
	cwd := languageProject(t, "ko", nil)
	english := noteTranscript(t, "Running the package tests now to check the parser change.")
	session := "note-" + strings.ReplaceAll(t.Name(), "/", "_")
	t.Cleanup(func() { resetStopState(session) })

	got := runLanguageNote(noteInput(t, session, cwd, english))
	assert.Contains(t, got, "Korean")
	assert.Contains(t, got, "Running the package tests now")
	assert.Empty(t, runLanguageNote(noteInput(t, session, cwd, english)), "one reminder per note")

	korean := noteTranscript(t, "파서 변경을 확인하려고 패키지 테스트를 돌립니다.")
	assert.Empty(t, runLanguageNote(noteInput(t, session+"-2", cwd, korean)))
	assert.Empty(t, runLanguageNote(noteInput(t, session+"-3", languageProject(t, "en", nil), english)), "English is not judged")
	off := false
	assert.Empty(t, runLanguageNote(noteInput(t, session+"-4", languageProject(t, "ko", &off), english)), "hooks.language_stop: false")
	t.Setenv("AUTOPUS_LANGUAGE_STOP", "off")
	assert.Empty(t, runLanguageNote(noteInput(t, session+"-5", cwd, english)))
}

func TestVerifyLanguageNote_Command(t *testing.T) {
	isolateStop(t)
	cwd := languageProject(t, "ko", nil)
	session := "note-cmd-" + strings.ReplaceAll(t.Name(), "/", "_")
	t.Cleanup(func() { resetStopState(session) })
	cmd := newVerifyLanguageCmd()
	cmd.SetIn(noteInput(t, session, cwd, noteTranscript(t, "Now checking the build output for errors.")))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--note"})
	require.NoError(t, cmd.Execute())
	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	assert.Equal(t, "PostToolUse", got.HookSpecificOutput.HookEventName)
	assert.Contains(t, got.HookSpecificOutput.AdditionalContext, "Korean")
}
