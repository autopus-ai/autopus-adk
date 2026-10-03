package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
)

type qaLoopOptions struct {
	ProjectDir    string
	Lane          string
	Profile       string
	JourneyID     string
	Agent         string
	MaxIterations int
	AgentTimeout  time.Duration
	JSONOut       bool
	Format        string
}

// newQALoopCmd builds `auto qa loop` wired to the production lane runner,
// triage, and agent.
func newQALoopCmd() *cobra.Command {
	return newQALoopCmdWith(qaloop.DefaultDeps)
}

// newQALoopCmdWith builds the command over injected loop seams so tests can
// fake the lane and the agent while git stays real.
func newQALoopCmdWith(deps func() qaloop.Deps) *cobra.Command {
	opts := qaLoopOptions{}
	cmd := &cobra.Command{
		Use:   "loop",
		Short: "Run a lane, triage failures, let an agent repair them, and re-verify on a loop branch",
		Long: `Run a QA lane, re-run each failure once, triage it, and let a headless agent
repair only what the failure's class permits. Every agent diff is checked
against the class allowlist and committed on autopus/qa-loop/<run-id>, or
reverted. The loop stops on passed, passed_with_flaky, max_iterations,
no_progress, blocked_environment, unknown_failure, guard_rejected,
commit_rejected, or agent_failed, writes .autopus/qa/loop/<run-id>/report.json
and report.md, and checks out the original branch again.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runQALoop(cmd, opts, deps())
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	flags.StringVar(&opts.Lane, "lane", "fast", "QA lane to run")
	flags.StringVar(&opts.Profile, "profile", "", "QA profile (default standalone)")
	flags.StringVar(&opts.JourneyID, "journey", "", "Limit the loop to one journey id")
	flags.StringVar(&opts.Agent, "agent", "", "Repair agent: claude, codex, gemini, or opencode")
	flags.IntVar(&opts.MaxIterations, "max-iterations", qaloop.DefaultMaxIterations, "Maximum fix iterations")
	flags.DurationVar(&opts.AgentTimeout, "agent-timeout", qaloop.DefaultAgentTimeout, "Timeout for one agent run")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func runQALoop(cmd *cobra.Command, opts qaLoopOptions, deps qaloop.Deps) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	fail := func(cause error, code string, data any) error {
		if jsonMode {
			return writeJSONResultAndExit(cmd, jsonStatusError, cause, code, data, nil, nil)
		}
		return cause
	}
	target, err := agentexec.ParseTarget(opts.Agent)
	if err != nil {
		return fail(err, firstNonEmptyCode(agentexec.ErrorCode(err), qaloop.CodeInvalidOptions), nil)
	}
	if opts.MaxIterations < 1 {
		return fail(fmt.Errorf("--max-iterations must be at least 1"), qaloop.CodeInvalidOptions, nil)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	report, err := qaloop.Run(ctx, qaloop.Options{
		ProjectDir: opts.ProjectDir, Lane: opts.Lane, Profile: opts.Profile, JourneyID: opts.JourneyID,
		Agent: target, MaxIterations: opts.MaxIterations, AgentTimeout: opts.AgentTimeout,
	}, deps)
	if err != nil {
		var data any
		if report.RunID != "" {
			data = report
			if !jsonMode {
				writeQALoopText(cmd.OutOrStdout(), report)
			}
		}
		return fail(err, qaloop.ErrorCode(err), data)
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, report, nil, nil)
	}
	writeQALoopText(cmd.OutOrStdout(), report)
	return nil
}

func writeQALoopText(w io.Writer, report qaloop.Report) {
	commits := 0
	for _, it := range report.Iterations {
		if it.Commit != "" {
			commits++
		}
	}
	fmt.Fprintf(w, "qa loop %s: %s\n", report.RunID, report.StopReason)
	if report.StopDetail != "" {
		fmt.Fprintf(w, "detail: %s\n", report.StopDetail)
	}
	if report.BranchDeleted {
		fmt.Fprintf(w, "loop branch: none kept (no fix commit; %s was removed)\n", report.Branch)
	} else {
		fmt.Fprintf(w, "loop branch: %s (%d fix commit(s))\n", report.Branch, commits)
	}
	fmt.Fprintf(w, "original ref: %s (restored: %t)\n", report.OriginalRef, report.Restored)
	fmt.Fprintf(w, "iterations: %d of max %d\n", len(report.Iterations), report.MaxIterations)
	fmt.Fprintf(w, "report: %s\n", report.ReportPath)
}

func firstNonEmptyCode(codes ...string) string {
	for _, code := range codes {
		if code != "" {
			return code
		}
	}
	return ""
}
