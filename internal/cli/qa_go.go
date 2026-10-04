package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/autopilot"
	"github.com/insajin/autopus-adk/pkg/qa/generate"
	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
)

type qaGoOptions struct {
	ProjectDir    string
	Agent         string
	Lane          string
	MaxIterations int
	AgentTimeout  time.Duration
	JSONOut       bool
	Format        string
}

func newQAGoCmd() *cobra.Command {
	return newQAGoCmdWith(autopilot.Deps{})
}

// newQAGoCmdWith takes injected steps so tests can fake generation and the
// loop while the command's own wiring stays real.
func newQAGoCmdWith(deps autopilot.Deps) *cobra.Command {
	opts := qaGoOptions{}
	cmd := &cobra.Command{
		Use:   "go [SPEC-ID]",
		Short: "Generate scenarios from a SPEC, then run, triage, fix, and re-verify in one command",
		Long: `Run the whole intent-anchored QA chain:

  1. generate test and user scenarios from the SPEC's acceptance criteria
  2. show coverage and ask before going on (skip with --auto)
  3. promote the candidates that hold up, and compile them
  4. run the lane; triage each failure, let the agent fix what its class
     permits, and re-verify, on branch autopus/qa-loop/<run-id>

Without a SPEC-ID, generation and promotion are skipped: the scenarios
already in place are compiled and the loop runs over them. --agent defaults to the first installed CLI
(claude, codex, agy, opencode); --lane defaults to browser-staging when a
Journey Pack declares it.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := ""
			if len(args) == 1 {
				spec = args[0]
			}
			return runQAGo(cmd, spec, opts, deps)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	flags.StringVar(&opts.Agent, "agent", "", "Agent: claude, codex, gemini, or opencode (default: first installed)")
	flags.StringVar(&opts.Lane, "lane", "", "QA lane (default: browser-staging if a pack declares it, else the first declared lane)")
	flags.IntVar(&opts.MaxIterations, "max-iterations", qaloop.DefaultMaxIterations, "Maximum fix iterations")
	flags.DurationVar(&opts.AgentTimeout, "agent-timeout", qaloop.DefaultAgentTimeout, "Timeout for one agent run")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func runQAGo(cmd *cobra.Command, spec string, opts qaGoOptions, deps autopilot.Deps) error {
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
	target, err := resolveQAGoAgent(opts.Agent)
	if err != nil {
		return fail(err, firstNonEmptyCode(agentexec.ErrorCode(err), qaloop.CodeInvalidOptions), nil)
	}
	out := cmd.OutOrStdout()
	if jsonMode {
		out = io.Discard
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	// --auto is the root's global flag; JSON output has nobody to answer a
	// prompt, so it runs unattended too.
	unattended := globalFlagsFromContext(ctx).AutoMode || jsonMode
	report, err := autopilot.Run(ctx, autopilot.Options{
		ProjectDir: opts.ProjectDir, SpecID: spec, Agent: target, Lane: opts.Lane,
		Auto: unattended, MaxIterations: opts.MaxIterations, AgentTimeout: opts.AgentTimeout,
		OnGenerated: func(gen generate.Report) { writeQAGenerateCoverage(out, gen) },
		Confirm: func(generate.Report) bool {
			return confirmIssue(cmd, "promote these candidates and run the loop? [y/N] ")
		},
		Progress: func(stage, line string) { fmt.Fprintf(out, "[%s] %s\n", stage, line) },
	}, deps)
	if report.Loop != nil && report.Loop.RunID != "" {
		writeQALoopText(out, *report.Loop)
	}
	if err != nil {
		code := "qa_go_failed"
		var stop *autopilot.Error
		if errors.As(err, &stop) && stop.Code != "" {
			code = stop.Code
		}
		return fail(err, code, report)
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, report, nil, nil)
	}
	if report.Loop != nil && report.Loop.Branch != "" && !report.Loop.BranchDeleted {
		fmt.Fprintf(out, "next: review and merge %s\n", report.Loop.Branch)
	}
	return nil
}

// resolveQAGoAgent honours --agent, else picks the first installed CLI.
func resolveQAGoAgent(flag string) (agentexec.Target, error) {
	if strings.TrimSpace(flag) != "" {
		return agentexec.ParseTarget(flag)
	}
	return agentexec.DetectTarget(exec.LookPath, os.Getenv)
}

// writeQAGenerateCoverage prints what generation produced: one line per
// criterion, so a person can see an uncovered criterion before promoting.
func writeQAGenerateCoverage(w io.Writer, gen generate.Report) {
	fmt.Fprintf(w, "spec %s: %d criteria, %d candidate file(s), %d rejected\n",
		gen.Spec, gen.Criteria, len(gen.Written), len(gen.Rejected))
	for _, row := range gen.Coverage {
		fmt.Fprintf(w, "  %-24s %-26s %s\n", row.ID, row.Status, strings.Join(row.Refs, ", "))
	}
	for _, rej := range gen.Rejected {
		fmt.Fprintf(w, "  rejected document %d: %s %s\n", rej.Index, rej.Code, rej.Message)
	}
}
