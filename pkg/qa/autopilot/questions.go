package autopilot

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/intent"
)

// CodeQuestionsOpen stops a run whose intent still has unanswered questions.
const CodeQuestionsOpen = "qa_go_questions_open"

// StageQuestions checks the intent's questions before generation.
const StageQuestions = "questions"

// resolveQuestions makes sure generation never runs on a reading nobody
// chose silently. An attended run stops and says how to answer; an
// unattended one records the recommended options as such and says which.
func resolveQuestions(opts Options, progress func(stage, line string)) error {
	questions, err := intent.LoadQuestions(opts.ProjectDir, opts.SpecID)
	if err != nil {
		return &Error{Stage: StageQuestions, Code: CodeQuestionsOpen, Err: err}
	}
	open := intent.Open(questions)
	if len(open) == 0 {
		return nil
	}
	ids := make([]string, 0, len(open))
	for _, q := range open {
		ids = append(ids, q.ID)
	}
	if !opts.Auto {
		return &Error{Stage: StageQuestions, Code: CodeQuestionsOpen, Err: fmt.Errorf(
			"%s has %d unanswered question(s) (%s): run `auto qa intent answer %s %s=<n> ...`, or `--recommended` to accept the recommended options",
			opts.SpecID, len(open), strings.Join(ids, ", "), opts.SpecID, ids[0])}
	}
	if _, err := intent.AnswerRecommended(opts.ProjectDir, opts.SpecID); err != nil {
		return &Error{Stage: StageQuestions, Code: CodeQuestionsOpen, Err: err}
	}
	progress(StageQuestions, fmt.Sprintf("nobody answered %s; using the recommended options, recorded as answered_by=recommended in %s",
		strings.Join(ids, ", "), intent.QuestionsPath(".", opts.SpecID)))
	return nil
}
