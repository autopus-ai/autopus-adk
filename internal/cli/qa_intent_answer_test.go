package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/intent"
)

func runQAIntentAnswerCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"qa", "intent", "answer"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestQAIntentAnswerCmd_NumbersOptionsFromOne(t *testing.T) {
	dir := t.TempDir()
	path := intent.QuestionsPath(dir, "INTENT-words")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`[
  {"id":"Q1","criterion":"AC-3","question":"Spelling?","options":["lowercase","first seen"],"recommended":0},
  {"id":"Q2","criterion":"AC-1","question":"Layout?","options":["word count","count word"],"recommended":0}
]`), 0o644))

	out, err := runQAIntentAnswerCLI(t, "INTENT-words", "Q2=2", "--project-dir", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "Q2 (AC-1) 2. count word (user)")
	assert.Contains(t, out, "still open: Q1")

	out, err = runQAIntentAnswerCLI(t, "INTENT-words", "--recommended", "--project-dir", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "Q1 (AC-3) 1. lowercase (recommended)")
	assert.Contains(t, out, "Q2 (AC-1) 2. count word (user)")
	assert.Contains(t, out, "next: auto qa go INTENT-words")

	for _, bad := range [][]string{{"INTENT-words", "Q1=3"}, {"INTENT-words", "Q1"}, {"INTENT-words", "Q1=x"}, {"INTENT-words"}} {
		_, err := runQAIntentAnswerCLI(t, append(bad, "--project-dir", dir)...)
		assert.Equal(t, intent.CodeAnswerInvalid, intent.ErrorCode(err), "%v", bad)
	}
}
