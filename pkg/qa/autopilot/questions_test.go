package autopilot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/generate"
	"github.com/insajin/autopus-adk/pkg/qa/intent"
)

func withQuestions(t *testing.T, dir string) {
	t.Helper()
	path := intent.QuestionsPath(dir, "INTENT-x")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`[{"id":"Q1","criterion":"AC-1","question":"Layout?","options":["a","b"],"recommended":1}]`), 0o644))
}

// Attended, an open question stops the run before any agent is paid for.
func TestRun_OpenQuestionStopsAnAttendedRun(t *testing.T) {
	t.Parallel()
	dir := project(t)
	withQuestions(t, dir)
	c := &calls{}
	_, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: "INTENT-x", Agent: agentexec.TargetClaude, Lane: "fast",
		Confirm: func(generate.Report) bool { return true }}, fakeDeps(c, nil))
	var stop *Error
	require.ErrorAs(t, err, &stop)
	assert.Equal(t, CodeQuestionsOpen, stop.Code)
	assert.Contains(t, stop.Error(), "auto qa intent answer INTENT-x Q1=<n>")
	assert.Empty(t, c.order)
}

// Unattended, the recommended option is recorded as nobody's choice and the
// run says so.
func TestRun_AutoRecordsRecommendedAnswers(t *testing.T) {
	t.Parallel()
	dir := project(t)
	withQuestions(t, dir)
	c := &calls{}
	var lines []string
	_, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: "INTENT-x", Agent: agentexec.TargetClaude, Lane: "fast", Auto: true,
		Progress: func(stage, line string) { lines = append(lines, stage+": "+line) }}, fakeDeps(c, nil))
	require.NoError(t, err)
	assert.Equal(t, []string{"generate:INTENT-x", "promote", "compile", "loop"}, c.order)
	assert.Contains(t, lines[0], "questions: nobody answered Q1; using the recommended options")
	qs, err := intent.LoadQuestions(dir, "INTENT-x")
	require.NoError(t, err)
	require.NotNil(t, qs[0].Answer)
	assert.Equal(t, 1, *qs[0].Answer)
	assert.Equal(t, intent.AnsweredByRecommended, qs[0].AnsweredBy)
}

// When every generated document was rejected twice the run stops at
// generation and says so, instead of failing later at compile for the wrong
// reason.
func TestRun_AllDocumentsRejectedStopsAtGenerate(t *testing.T) {
	t.Parallel()
	c := &calls{}
	deps := fakeDeps(c, nil)
	deps.Generate = func(context.Context, generate.Options) (generate.Report, error) {
		c.order = append(c.order, "generate")
		return generate.Report{Rejected: []generate.Rejection{{Code: "qa_test_scenario_parse_invalid"}}}, nil
	}
	_, err := Run(context.Background(), Options{ProjectDir: project(t), SpecID: "SPEC-X", Agent: agentexec.TargetCodex, Lane: "fast", Auto: true}, deps)
	var stop *Error
	require.ErrorAs(t, err, &stop)
	assert.Equal(t, CodeNothingGenerated, stop.Code)
	assert.Contains(t, stop.Error(), "qa_test_scenario_parse_invalid")
	assert.Equal(t, []string{"generate", "generate"}, c.order, "one retry, then stop")
}

// A first answer with nothing usable is asked again once; a good second
// answer carries the run on.
func TestRun_RetriesGenerationOnce(t *testing.T) {
	t.Parallel()
	c := &calls{}
	deps := fakeDeps(c, nil)
	calls := 0
	deps.Generate = func(_ context.Context, o generate.Options) (generate.Report, error) {
		calls++
		c.order = append(c.order, "generate")
		if calls == 1 {
			return generate.Report{Documents: 0}, nil
		}
		return generate.Report{Written: []string{"x"}, TestScenarios: []string{o.SpecID}}, nil
	}
	var lines []string
	_, err := Run(context.Background(), Options{ProjectDir: project(t), SpecID: "SPEC-X", Agent: agentexec.TargetGemini, Lane: "fast", Auto: true,
		Progress: func(stage, line string) { lines = append(lines, stage+": "+line) }}, deps)
	require.NoError(t, err)
	assert.Equal(t, []string{"generate", "generate", "promote", "compile", "loop"}, c.order)
	assert.Contains(t, strings.Join(lines, "\n"), "asking once more")
}
