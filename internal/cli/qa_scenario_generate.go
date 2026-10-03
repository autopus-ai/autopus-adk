package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/generate"
)

type qaScenarioGenerateOptions struct {
	ProjectDir string
	Spec       string
	Agent      string
	Timeout    time.Duration
	JSONOut    bool
	Format     string
}

// qaScenarioGeneratePayload is the JSON data of a generation run.
type qaScenarioGeneratePayload struct {
	generate.Report
	NextSteps []string `json:"next_steps"`
}

func newQAScenarioGenerateCmd() *cobra.Command {
	var opts qaScenarioGenerateOptions
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Draft scenario candidates from a SPEC's acceptance criteria with an agent",
		Long: "Runs an agent CLI headless in generate mode, keeps only the YAML documents that validate " +
			"and cite the SPEC's own acceptance criteria, and writes them to the candidate directories. " +
			"Nothing compiles or runs a candidate until 'auto qa scenario promote' moves it into place.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQAScenarioGenerate(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Spec, "spec", "", "SPEC id whose acceptance.md drives generation (required)")
	cmd.Flags().StringVar(&opts.Agent, "agent", "", "Agent CLI to run headless: claude, codex, gemini, or opencode (required)")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 10*time.Minute, "Agent run timeout")
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func runQAScenarioGenerate(cmd *cobra.Command, opts qaScenarioGenerateOptions) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	fail := func(cause error, data any) error {
		if jsonMode {
			return writeJSONResultAndExit(cmd, jsonStatusError, cause, generate.ErrorCode(cause), data, nil, nil)
		}
		return cause
	}
	if strings.TrimSpace(opts.Spec) == "" {
		return fail(&generate.Error{Code: generate.CodeSpecMissing, Message: "--spec is required"}, nil)
	}
	target, err := agentexec.ParseTarget(opts.Agent)
	if err != nil {
		return fail(err, nil)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	report, err := generate.Run(ctx, generate.Options{
		ProjectDir: opts.ProjectDir, SpecID: opts.Spec, Target: target, Timeout: opts.Timeout,
	})
	payload := qaScenarioGeneratePayload{Report: report, NextSteps: generateNextSteps(report, opts.ProjectDir)}
	if err != nil {
		// A run that reached the agent still has coverage and rejections worth
		// showing, for example when only one candidate hit a conflict.
		if !jsonMode && report.Documents > 0 {
			writeQAGenerateText(cmd.OutOrStdout(), payload)
		}
		return fail(err, payload)
	}
	if jsonMode {
		warnings := generateWarnings(report)
		status := jsonStatusOK
		if len(warnings) > 0 {
			status = jsonStatusWarn
		}
		return writeJSONResult(cmd, status, payload, warnings, nil)
	}
	writeQAGenerateText(cmd.OutOrStdout(), payload)
	return nil
}

func generateNextSteps(report generate.Report, projectDir string) []string {
	if len(report.Written) == 0 {
		return []string{}
	}
	return []string{"auto qa scenario promote --all --project-dir " + projectDir}
}

func generateWarnings(report generate.Report) []jsonMessage {
	var warnings []jsonMessage
	if report.Documents == 0 {
		warnings = append(warnings, jsonMessage{Code: "qa_generate_no_documents",
			Message: "the agent printed no fenced YAML documents"})
	}
	if n := len(report.Rejected); n > 0 {
		warnings = append(warnings, jsonMessage{Code: "qa_generate_documents_rejected",
			Message: fmt.Sprintf("%d agent document(s) were rejected and not written", n)})
	}
	for _, problem := range report.Problems {
		warnings = append(warnings, jsonMessage{Code: "qa_generate_acceptance_" + problem.Code,
			Message: fmt.Sprintf("%s (line %d): %s", problem.ID, problem.Line, problem.Code)})
	}
	return warnings
}

func writeQAGenerateText(out io.Writer, payload qaScenarioGeneratePayload) {
	report := payload.Report
	fmt.Fprintf(out, "spec: %s  agent: %s  criteria: %d  documents: %d\n",
		report.Spec, report.Agent, report.Criteria, report.Documents)
	if report.PromptCriteria < report.Criteria {
		fmt.Fprintf(out, "prompt: only the first %d criteria fit; the rest stay uncovered\n", report.PromptCriteria)
	}
	for _, problem := range report.Problems {
		fmt.Fprintf(out, "acceptance problem: %s %s (line %d)\n", problem.ID, problem.Code, problem.Line)
	}
	for _, path := range report.Written {
		fmt.Fprintf(out, "candidate: %s\n", path)
	}
	for _, rejected := range report.Rejected {
		label := ""
		if rejected.ID != "" {
			label = fmt.Sprintf(" (%s %s)", rejected.Kind, rejected.ID)
		}
		fmt.Fprintf(out, "rejected: document %d%s: %s: %s\n", rejected.Index, label, rejected.Code, rejected.Message)
	}
	width := 0
	for _, row := range report.Coverage {
		if len(row.ID) > width {
			width = len(row.ID)
		}
	}
	fmt.Fprintln(out, "coverage:")
	for _, row := range report.Coverage {
		line := fmt.Sprintf("  %-*s  %-24s  %s", width, row.ID, row.Status, strings.Join(row.Refs, ", "))
		fmt.Fprintln(out, strings.TrimRight(line, " "))
	}
	for _, step := range payload.NextSteps {
		fmt.Fprintf(out, "next: review the candidates, then %s\n", step)
	}
}
