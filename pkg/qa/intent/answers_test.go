package intent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeIntent leaves an intent with the two questions a word counter's
// request raised: how the merged word is spelled, and the line layout.
func writeIntent(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	dir := Dir(project, "INTENT-words")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, write(dir, "count words", Draft{Acceptance: "x\n", Questions: []Question{
		{ID: "Q1", Criterion: "AC-3", Question: "How is a merged word shown?", Options: []string{"lowercase", "first spelling"}, Recommended: 0},
		{ID: "Q2", Criterion: "AC-1", Question: "Line layout?", Options: []string{"word count", "count word"}, Recommended: 0},
	}}))
	return project
}

func TestAnswer_RecordsWhoChoseAndKeepsTheRestOpen(t *testing.T) {
	project := writeIntent(t)
	qs, err := Answer(project, "INTENT-words", map[string]int{"Q2": 1}, AnsweredByUser)
	require.NoError(t, err)
	assert.Equal(t, []string{"Q1"}, ids(Open(qs)))
	assert.Equal(t, []string{"AC-1: Line layout? -> count word (user)"}, Readings(qs))
	assert.Equal(t, []string{"AC-3: How is a merged word shown? (undecided between: lowercase | first spelling)"}, Undecided(qs))

	qs, err = AnswerRecommended(project, "INTENT-words")
	require.NoError(t, err)
	assert.Empty(t, Open(qs))
	assert.Equal(t, []string{
		"AC-3: How is a merged word shown? -> lowercase (recommended)",
		"AC-1: Line layout? -> count word (user)",
	}, Readings(qs), "a recommended default never overwrites the user's choice")

	reloaded, err := LoadQuestions(project, "INTENT-words")
	require.NoError(t, err)
	assert.Equal(t, qs, reloaded)
}

func TestAnswer_RejectsUnknownQuestionsAndOptionsBeforeWriting(t *testing.T) {
	project := writeIntent(t)
	before, err := os.ReadFile(QuestionsPath(project, "INTENT-words"))
	require.NoError(t, err)
	for _, picks := range []map[string]int{{"Q9": 0}, {"Q1": 2}, {"Q1": -1}, {"Q1": 0, "Q9": 0}} {
		_, err := Answer(project, "INTENT-words", picks, AnsweredByUser)
		assert.Equal(t, CodeAnswerInvalid, ErrorCode(err), "%v", picks)
	}
	after, err := os.ReadFile(QuestionsPath(project, "INTENT-words"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))

	_, err = Answer(t.TempDir(), "INTENT-none", map[string]int{"Q1": 0}, AnsweredByUser)
	assert.Equal(t, CodeAnswerInvalid, ErrorCode(err))
	_, err = AnswerRecommended(t.TempDir(), "INTENT-none")
	assert.Equal(t, CodeAnswerInvalid, ErrorCode(err), "no intent is not a success")
	qs, err := LoadQuestions(t.TempDir(), "SPEC-HAND-1")
	require.NoError(t, err, "a hand-written SPEC has no questions.json")
	assert.Nil(t, qs)
}

// The agent drafts questions; it cannot also answer them.
func TestParseOutput_DraftCannotArriveAnswered(t *testing.T) {
	out := "```acceptance\n### AC-1: x\nGiven a\nWhen b\nThen c\n```\n\n```intent-questions\n" +
		"- id: Q1\n  criterion: AC-1\n  question: which?\n  options: [a, b]\n  recommended: 0\n  answer: 1\n  answered_by: user\n```\n"
	draft, err := ParseOutput(out)
	require.NoError(t, err)
	require.Len(t, draft.Questions, 1)
	assert.Nil(t, draft.Questions[0].Answer)
	assert.Empty(t, draft.Questions[0].AnsweredBy)
}

func ids(qs []Question) []string {
	out := []string{}
	for _, q := range qs {
		out = append(out, q.ID)
	}
	return out
}
