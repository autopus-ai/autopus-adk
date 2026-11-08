package autopilot

import (
	"context"
	"fmt"

	"github.com/insajin/autopus-adk/pkg/qa/generate"
)

// generateOnce runs generation and, when the agent's answer yielded nothing
// usable, runs it one more time. Agents occasionally answer in prose or with
// one malformed document; a second call usually succeeds, and a run that
// went on with nothing would only fail later at compile for the wrong reason.
func generateOnce(ctx context.Context, opts Options, deps Deps, spec string, progress func(stage, line string)) (generate.Report, error) {
	var gen generate.Report
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt == 1 {
			progress(StageGenerate, fmt.Sprintf("generating scenarios from %s with %s", spec, opts.Agent))
		} else {
			progress(StageGenerate, fmt.Sprintf("the agent's answer held nothing usable (%s); asking once more", nothingReason(gen)))
		}
		var err error
		gen, err = deps.Generate(ctx, generate.Options{ProjectDir: opts.ProjectDir, SpecID: spec, Target: opts.Agent})
		if err != nil {
			return gen, &Error{Stage: StageGenerate, Code: generate.ErrorCode(err), Err: err}
		}
		if accepted(gen) {
			return gen, nil
		}
	}
	return gen, &Error{Stage: StageGenerate, Code: CodeNothingGenerated, Err: fmt.Errorf(
		"two generation attempts produced nothing usable (%s); try another --agent", nothingReason(gen))}
}

func accepted(gen generate.Report) bool {
	return len(gen.Scenarios) > 0 || len(gen.TestScenarios) > 0
}

func nothingReason(gen generate.Report) string {
	if len(gen.Rejected) > 0 {
		return fmt.Sprintf("%d document(s) rejected, first: %s", len(gen.Rejected), gen.Rejected[0].Code)
	}
	return fmt.Sprintf("%d YAML document(s) in the answer", gen.Documents)
}
