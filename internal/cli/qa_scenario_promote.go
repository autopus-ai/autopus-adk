package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/promote"
)

type qaScenarioPromoteOptions struct {
	ProjectDir            string
	All                   bool
	AcceptAgentAssertions bool
	DryRun                bool
	JSONOut               bool
	Format                string
}

func newQAScenarioPromoteCmd() *cobra.Command {
	var opts qaScenarioPromoteOptions
	cmd := &cobra.Command{
		Use:   "promote [id...]",
		Short: "Move reviewed candidates into the active scenario directories",
		Long: "A candidate is promoted only if it validates, the acceptance criteria it cites still exist, " +
			"and every agent-authored assertion is confirmed. An active file with different content is never overwritten.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQAScenarioPromote(cmd, args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.All, "all", false, "Promote every candidate that passes the gates")
	cmd.Flags().BoolVar(&opts.AcceptAgentAssertions, "accept-agent-assertions", false,
		"Confirm the reviewed agent-authored assertions of the selected candidates")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Report what would be promoted without moving files")
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func runQAScenarioPromote(cmd *cobra.Command, ids []string, opts qaScenarioPromoteOptions) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	report, err := promote.Promote(opts.ProjectDir, promote.Options{
		IDs: ids, All: opts.All, AcceptAgentAssertions: opts.AcceptAgentAssertions, DryRun: opts.DryRun,
	})
	if err != nil {
		if jsonMode {
			return writeJSONResultAndExit(cmd, jsonStatusError, err, promote.ErrorCode(err), nil, nil, nil)
		}
		return err
	}
	// Named candidates are a request for exactly those files, so one staying
	// behind fails the command; --all skipping a few is the expected outcome.
	var incomplete error
	if !opts.All && len(report.Skipped) > 0 {
		incomplete = &promote.Error{Code: promote.CodeIncomplete,
			Message: fmt.Sprintf("%d of the named candidates were not promoted", len(report.Skipped))}
	}
	if jsonMode {
		if incomplete != nil {
			return writeJSONResultAndExit(cmd, jsonStatusError, incomplete, promote.CodeIncomplete, report, nil, nil)
		}
		status := jsonStatusOK
		if len(report.Skipped) > 0 {
			status = jsonStatusWarn
		}
		return writeJSONResult(cmd, status, report, nil, nil)
	}
	writeQAPromoteText(cmd.OutOrStdout(), report, opts.ProjectDir)
	return incomplete
}

func writeQAPromoteText(out io.Writer, report promote.Report, projectDir string) {
	verb := "promoted"
	if report.DryRun {
		verb = "would promote"
	}
	for _, item := range report.Promoted {
		note := ""
		switch {
		case item.AlreadyActive:
			note = " (identical active file; candidate removed)"
		case item.AcceptedAssertions > 0:
			note = fmt.Sprintf(" (accepted %d agent assertion(s))", item.AcceptedAssertions)
		}
		fmt.Fprintf(out, "%s %s %s: %s -> %s%s\n", verb, item.Kind, item.ID, item.From, item.To, note)
	}
	unconfirmed := false
	for _, skip := range report.Skipped {
		fmt.Fprintf(out, "skipped %s: %s: %s\n", skip.ID, skip.Code, skip.Message)
		unconfirmed = unconfirmed || skip.Code == promote.CodeUnconfirmedAgentAssertion
	}
	if len(report.Promoted) == 0 && len(report.Skipped) == 0 {
		fmt.Fprintln(out, "no candidates to promote")
	}
	if unconfirmed {
		fmt.Fprintln(out, "hint: review the agent assertions, then re-run with --accept-agent-assertions")
	}
	if len(report.Promoted) > 0 && !report.DryRun {
		fmt.Fprintf(out, "next: auto qa scenario compile --project-dir %s\n", projectDir)
	}
}
