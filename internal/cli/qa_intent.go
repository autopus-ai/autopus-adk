package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/intent"
)

type qaIntentOptions struct {
	ProjectDir  string
	ID          string
	Request     string
	RequestFile string
	Agent       string
	Timeout     time.Duration
	Force       bool
	JSONOut     bool
	Format      string
}

// qaIntentPayload is the JSON data of an intent run.
type qaIntentPayload struct {
	intent.Report
	NextSteps []string `json:"next_steps"`
}

func newQAIntentCmd() *cobra.Command {
	var opts qaIntentOptions
	cmd := &cobra.Command{
		Use:   "intent",
		Short: "Write acceptance criteria from the user's request alone, for auto qa go",
		Long: "Runs an agent CLI headless in an empty directory with only the request text, so the criteria " +
			"describe what the user asked for rather than what the code does. Writes " +
			".autopus/specs/<id>/acceptance.md, the verbatim request.md, and questions.json with the points " +
			"where the request allows more than one reading. Resolve those questions with the user, then run " +
			"'auto qa go <id>'. The directory has no spec.md, so it is not listed as a SPEC.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQAIntent(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Request, "request", "", "The user's request, verbatim")
	cmd.Flags().StringVar(&opts.RequestFile, "request-file", "", "File holding the user's request, verbatim (- for stdin)")
	cmd.Flags().StringVar(&opts.ID, "id", "", "Intent id such as INTENT-login (default: INTENT-<request hash>)")
	cmd.Flags().StringVar(&opts.Agent, "agent", "", "Agent CLI to run headless: claude, codex, gemini, or opencode (default: first installed)")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 10*time.Minute, "Agent run timeout")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Replace an existing intent with the same id")
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func runQAIntent(cmd *cobra.Command, opts qaIntentOptions) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	fail := func(cause error, data any) error {
		if jsonMode {
			return writeJSONResultAndExit(cmd, jsonStatusError, cause, intent.ErrorCode(cause), data, nil, nil)
		}
		return cause
	}
	request, err := readQAIntentRequest(cmd.InOrStdin(), opts)
	if err != nil {
		return fail(err, nil)
	}
	target, err := resolveQAGoAgent(opts.Agent)
	if err != nil {
		return fail(err, nil)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	report, err := intent.Run(ctx, intent.Options{
		ProjectDir: opts.ProjectDir, ID: opts.ID, Request: request, Target: target, Timeout: opts.Timeout, Force: opts.Force,
	})
	payload := qaIntentPayload{Report: report, NextSteps: []string{}}
	if err != nil {
		return fail(err, payload)
	}
	payload.NextSteps = []string{"auto qa go " + report.ID + " --project-dir " + opts.ProjectDir}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, payload, nil, nil)
	}
	writeQAIntentText(cmd.OutOrStdout(), payload)
	return nil
}

// readQAIntentRequest takes the request from exactly one of --request and
// --request-file.
func readQAIntentRequest(stdin io.Reader, opts qaIntentOptions) (string, error) {
	switch {
	case opts.Request != "" && opts.RequestFile != "":
		return "", &intent.Error{Code: intent.CodeRequestMissing, Message: "pass --request or --request-file, not both"}
	case opts.RequestFile == "-":
		data, err := io.ReadAll(io.LimitReader(stdin, intent.MaxRequestBytes+1))
		return string(data), err
	case opts.RequestFile != "":
		data, err := os.ReadFile(opts.RequestFile)
		return string(data), err
	case strings.TrimSpace(opts.Request) == "":
		return "", &intent.Error{Code: intent.CodeRequestMissing, Message: "--request or --request-file is required"}
	}
	return opts.Request, nil
}

func writeQAIntentText(out io.Writer, payload qaIntentPayload) {
	report := payload.Report
	fmt.Fprintf(out, "intent %s: %d criteria in %s\n", report.ID, len(report.Criteria), report.Acceptance)
	if len(report.Questions) == 0 {
		fmt.Fprintln(out, "no open questions")
	}
	for _, q := range report.Questions {
		fmt.Fprintf(out, "  %s (%s) %s\n", q.ID, q.Criterion, q.Question)
		for i, option := range q.Options {
			mark := " "
			if i == q.Recommended {
				mark = "*"
			}
			fmt.Fprintf(out, "    %s %d. %s\n", mark, i+1, option)
		}
	}
	for _, step := range payload.NextSteps {
		fmt.Fprintf(out, "next: %s\n", step)
	}
}
