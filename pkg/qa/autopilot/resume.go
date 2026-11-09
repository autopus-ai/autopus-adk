package autopilot

import (
	"context"
	"errors"
	"fmt"

	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
)

// CodeNothingToResume stops --resume when no loop branch carries scenarios.
const CodeNothingToResume = "qa_go_nothing_to_resume"

// resume runs the loop from the latest seeded loop branch. Its scenarios are
// committed there, so nothing is generated, restored, or compiled here:
// restoring them into this tree would only block the switch to that branch.
func resume(ctx context.Context, opts Options, deps Deps, report Report, progress func(stage, line string)) (Report, error) {
	if report.Spec != "" {
		return report, &Error{Stage: StageLoop, Code: qaloop.CodeInvalidOptions,
			Err: errors.New("--resume continues an earlier loop's scenarios; it takes no SPEC-ID")}
	}
	base := deps.LatestBranch(opts.ProjectDir)
	if base == "" {
		return report, &Error{Stage: StageLoop, Code: CodeNothingToResume,
			Err: fmt.Errorf("no loop branch with scenarios to resume under %s; run `auto qa go <SPEC-ID>` first", qaloop.BranchPrefix)}
	}
	progress(StageLoop, fmt.Sprintf("resuming from %s: its scenarios and fixes are the starting point; running lane %s", base, opts.Lane))
	lr, err := deps.Loop(ctx, qaloop.Options{
		ProjectDir: opts.ProjectDir, Lane: opts.Lane, Agent: opts.Agent,
		MaxIterations: opts.MaxIterations, AgentTimeout: opts.AgentTimeout, BaseRef: base,
	})
	report.Loop = &lr
	if err != nil {
		return report, &Error{Stage: StageLoop, Code: qaloop.ErrorCode(err), Err: err}
	}
	return report, nil
}
