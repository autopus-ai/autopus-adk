package generate

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
)

// An intent's answered questions reach the generator as readings it may
// assert; open ones as points it must assert neither way.
func TestRun_IntentAnswersReachThePrompt(t *testing.T) {
	t.Parallel()
	dir := projectX(t, acceptanceX)
	writeFile(t, filepath.Join(dir, ".autopus", "specs", specX, "questions.json"), `[
  {"id": "Q1", "criterion": "AC-1", "question": "Line layout?", "options": ["word count", "count word"], "recommended": 0, "answer": 1, "answered_by": "user"},
  {"id": "Q2", "criterion": "AC-1", "question": "Merged spelling?", "options": ["lowercase", "first seen"], "recommended": 0}
]`)
	agent := &fakeRunner{stdout: "nothing"}
	_, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: specX, Target: agentexec.TargetClaude, Runner: agent})
	require.NoError(t, err)
	assert.Contains(t, agent.got.Prompt, "Resolved readings (assert these where a criterion leaves the point open):\n- AC-1: Line layout? -> count word (user)\n")
	assert.Contains(t, agent.got.Prompt, "Undecided points (assert neither option; keep expectations that hold under every option):\n- AC-1: Merged spelling? (undecided between: lowercase | first seen)\n")
}

// A hand-written SPEC has no questions.json and gets neither section.
func TestRun_NoQuestionsNoReadingSections(t *testing.T) {
	t.Parallel()
	agent := &fakeRunner{stdout: "nothing"}
	_, err := Run(context.Background(), Options{ProjectDir: projectX(t, acceptanceX), SpecID: specX, Target: agentexec.TargetClaude, Runner: agent})
	require.NoError(t, err)
	assert.NotContains(t, agent.got.Prompt, "Resolved readings")
	assert.NotContains(t, agent.got.Prompt, "Undecided points")
}
