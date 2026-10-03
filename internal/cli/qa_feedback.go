package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	qaevidence "github.com/insajin/autopus-adk/pkg/qa/evidence"
	qatriage "github.com/insajin/autopus-adk/pkg/qa/triage"
)

type qaFeedbackPayload struct {
	Target           string `json:"target"`
	QAResultID       string `json:"qa_result_id"`
	PromptBundlePath string `json:"prompt_bundle_path"`
	PromptPath       string `json:"prompt_path"`
}

func newQAFeedbackCmd() *cobra.Command {
	var (
		target       string
		evidencePath string
		output       string
		jsonOut      bool
		format       string
	)

	cmd := &cobra.Command{
		Use:   "feedback",
		Short: "Generate a bounded repair prompt bundle from failed QA evidence",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQAFeedback(cmd, qaFeedbackOptions{
				Target:       target,
				EvidencePath: evidencePath,
				Output:       output,
				JSONOut:      jsonOut,
				Format:       format,
			})
		},
	}
	cmd.Flags().StringVar(&target, "to", "", "Repair prompt target (codex|claude|gemini|opencode)")
	cmd.Flags().StringVar(&evidencePath, "evidence", "", "QAMESH evidence manifest path")
	cmd.Flags().StringVar(&output, "output", "", "Prompt bundle output directory")
	addJSONFlags(cmd, &jsonOut, &format)
	return cmd
}

type qaFeedbackOptions struct {
	Target       string
	EvidencePath string
	Output       string
	JSONOut      bool
	Format       string
}

// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-QAMESH-001: CLI feedback command bridges failed QA evidence into bounded repair prompt bundles.
// @AX:REASON: Provider targeting, failed-only evidence validation, and JSON output envelope behavior are coupled at this boundary.
func runQAFeedback(cmd *cobra.Command, opts qaFeedbackOptions) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	if err := validateQAFeedbackOptions(opts); err != nil {
		return qaCommandError(cmd, jsonMode, err, "qa_feedback_invalid_flags", map[string]any{"evidence": opts.EvidencePath, "output": opts.Output})
	}
	manifest, err := qaevidence.LoadManifest(opts.EvidencePath)
	if err != nil {
		return qaCommandError(cmd, jsonMode, err, "qa_feedback_read_failed", map[string]any{"evidence": opts.EvidencePath})
	}
	// The project root is the working directory: generated specs and their
	// step maps are located relative to it for the replay section.
	replay := qatriage.ReplayForManifest(".", manifest, filepath.Dir(opts.EvidencePath))
	result, err := qaevidence.WriteFeedbackBundleWithReplay(manifest, opts.Target, opts.Output, replay)
	if err != nil {
		return qaCommandError(cmd, jsonMode, err, "qa_feedback_write_failed", map[string]any{"qa_result_id": manifest.QAResultID, "target": opts.Target})
	}
	payload := qaFeedbackPayload{
		Target:           result.Target,
		QAResultID:       manifest.QAResultID,
		PromptBundlePath: result.BundlePath,
		PromptPath:       result.PromptPath,
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, payload, nil, nil)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", manifest.QAResultID, result.BundlePath)
	return nil
}

func validateQAFeedbackOptions(opts qaFeedbackOptions) error {
	for name, value := range map[string]string{
		"to":       opts.Target,
		"evidence": opts.EvidencePath,
		"output":   opts.Output,
	} {
		if err := requireFlag(name, value); err != nil {
			return err
		}
	}
	if err := rejectGeneratedQAOutput("output", opts.Output); err != nil {
		return err
	}
	return nil
}
