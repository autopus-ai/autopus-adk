package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/insajin/autopus-adk/pkg/taskroute"
	"github.com/spf13/cobra"
)

const workflowTriageMaxBytes = 1 << 20

// singleTriageFlag rejects ambiguous repeated options rather than silently
// selecting the last facts document or output format.
type singleTriageFlag struct {
	value string
	set   bool
}

func (f *singleTriageFlag) String() string { return f.value }
func (f *singleTriageFlag) Type() string   { return "string" }
func (f *singleTriageFlag) Set(value string) error {
	if f.set {
		return fmt.Errorf("option may only be supplied once")
	}
	f.value = value
	f.set = true
	return nil
}

func newWorkflowTriageCmd() *cobra.Command {
	facts := &singleTriageFlag{}
	format := &singleTriageFlag{value: "json"}
	cmd := &cobra.Command{Use: "triage", Short: "Inspect an advisory workflow route from caller-declared facts", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if facts.value == "" {
				return fmt.Errorf("--facts-json is required")
			}
			if format.value != "json" && format.value != "human" {
				return fmt.Errorf("--format must be json or human")
			}
			input, err := readWorkflowTriageFacts(facts.value)
			if err != nil {
				return err
			}
			decision, err := taskroute.Decide(input)
			if err != nil {
				return fmt.Errorf("invalid workflow triage facts")
			}
			report := struct {
				EvidenceSource     string             `json:"evidence_source"`
				FactVerification   string             `json:"fact_verification"`
				PathRiskBasis      string             `json:"path_risk_basis"`
				ExecutionPerformed bool               `json:"execution_performed"`
				Decision           taskroute.Decision `json:"decision"`
			}{"caller_declared", "not_independently_verified", "inferred_from_declared_paths", false, decision}
			if format.value == "json" {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(report)
			}
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintln(out, "Workflow triage (advisory; caller_declared)"); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(out, "Facts are not independently verified. Path risk is inferred from declared paths. No execution, spawning, or gate changes performed."); err != nil {
				return err
			}
			// The canonical decision retains its required steps, skill suggestions, and
			// execution policy without duplicating the routing package's schema.
			encoded, err := json.MarshalIndent(decision, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, string(encoded))
			return err
		}}
	cmd.Flags().Var(facts, "facts-json", "Path to a caller-declared workflow facts JSON file")
	cmd.Flags().Var(format, "format", "Output format: json or human (default json)")
	return cmd
}

func readWorkflowTriageFacts(path string) (taskroute.Facts, error) {
	var empty taskroute.Facts
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > workflowTriageMaxBytes {
		return empty, fmt.Errorf("facts file must be a readable regular file at most 1 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, fmt.Errorf("facts file unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || opened.Size() > workflowTriageMaxBytes {
		return empty, fmt.Errorf("facts file changed or exceeds limit")
	}
	facts, err := taskroute.Decode(io.LimitReader(file, workflowTriageMaxBytes+1))
	if err != nil {
		return empty, fmt.Errorf("invalid workflow triage facts JSON")
	}
	return facts, nil
}
