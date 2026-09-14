package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// newSpecGatesCmd computes gate applicability for a SPEC change set and
// records gate evidence for exact-input reuse.
func newSpecGatesCmd() *cobra.Command {
	var (
		changed             string
		base                string
		changeClass         string
		newContract         bool
		jsonOutput          bool
		maxAge              time.Duration
		annotationRequested bool
		referenceMissing    bool
	)

	cmd := &cobra.Command{
		Use:   "gates <SPEC-ID|SPEC_DIR>",
		Short: "Decide gate applicability and evidence reuse for a SPEC change set",
		Long: `Classifies the change set deterministically, decides which gates are
required, reusable, not_applicable, or blocked, and writes
{SPEC_DIR}/gate-applicability.json. Mandatory safety gates are never
not_applicable. Previously recorded evidence (see "gates record") is reused
only when its exact input closure still matches the current tree.

The declared change class decides the risk tier. Without --change-class the
class is derived from the change set, so it can never be understated. Low-risk
classes leave spec_authoring and risk_first_probe not_applicable; high-risk
classes require both.

The @AX annotation gate is opt-in: without --annotation it stays
not_applicable, so ordinary code work is never held behind an annotation pass
nobody asked for. With --annotation it is required for a code change set, and
blocked when --annotation-reference-missing reports the reference source
absent.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := resolveGatesTarget(args[0])
			if err != nil {
				return err
			}
			paths, err := resolveGatesChangeSet(target.Root, changed, base)
			if err != nil {
				return err
			}
			cfg, err := config.Load(target.Root)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			prior, err := gates.LoadPriorEvidence(target.Root, target.SpecDir)
			if err != nil {
				return fmt.Errorf("load gate evidence: %w", err)
			}
			classification := gates.Classify(paths, cfg.Design.UIFileGlobs)
			declared, err := resolveGatesChangeClass(changeClass)
			if err != nil {
				return err
			}
			receipt := gates.Decide(gates.DecisionInput{
				SpecID:                     target.SpecID,
				Classification:             classification,
				Change:                     gates.AssessChange(declared, classification, newContract),
				AnnotationRequested:        annotationRequested,
				AnnotationReferenceMissing: referenceMissing,
				Prior:                      prior,
				Now:                        time.Now(),
				MaxAge:                     maxAge,
			})
			receiptPath, err := gates.WriteApplicability(target.SpecDir, receipt)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeGatesJSON(cmd.OutOrStdout(), receipt)
			}
			printGateDecisions(cmd.OutOrStdout(), receipt, receiptPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&changed, "changed", "", "comma-separated changed paths relative to the project root (default: git working tree changes)")
	cmd.Flags().StringVar(&base, "base", "", "git ref to diff against when --changed is not given (default HEAD)")
	cmd.Flags().StringVar(&changeClass, "change-class", "", "declared change class: "+strings.Join(changeKindNames(), ", ")+" (default: derived from the change set)")
	cmd.Flags().BoolVar(&newContract, "new-contract", false, "the change introduces a new exported API or contract (always escalates)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print the applicability receipt as JSON")
	cmd.Flags().DurationVar(&maxAge, "max-age", gates.DefaultMaxAge, "maximum age of evidence eligible for reuse")
	cmd.Flags().BoolVar(&annotationRequested, "annotation", false, "evaluate the @AX annotation gate for this change set (default: not_applicable)")
	cmd.Flags().BoolVar(&referenceMissing, "annotation-reference-missing", false, "with --annotation, mark the annotation gate blocked because the @AX reference source is absent")

	cmd.AddCommand(newSpecGatesRecordCmd())
	return cmd
}

func newSpecGatesRecordCmd() *cobra.Command {
	var (
		gate        string
		status      string
		inputs      string
		dynamicDeps string
		command     string
		partial     bool
	)

	cmd := &cobra.Command{
		Use:   "record <SPEC-ID|SPEC_DIR>",
		Short: "Record gate evidence with its exact input closure",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(gate) == "" {
				return fmt.Errorf("--gate is required")
			}
			if strings.TrimSpace(status) == "" {
				return fmt.Errorf("--status is required")
			}
			if strings.TrimSpace(inputs) == "" {
				return fmt.Errorf("--inputs is required")
			}
			target, err := resolveGatesTarget(args[0])
			if err != nil {
				return err
			}
			receipt, err := gates.BuildEvidence(target.Root, gates.RecordOptions{
				SpecID:      target.SpecID,
				Gate:        gates.GateID(strings.TrimSpace(gate)),
				Status:      gates.EvidenceStatus(strings.ToLower(strings.TrimSpace(status))),
				Partial:     partial,
				InputGlobs:  splitCommaList(inputs),
				DynamicDeps: splitCommaList(dynamicDeps),
				Command:     command,
				Now:         time.Now(),
			})
			if err != nil {
				return fmt.Errorf("record gate evidence: %w", err)
			}
			path, err := gates.WriteEvidence(target.SpecDir, receipt)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "evidence recorded: %s (%s %s, complete=%t, %d input(s), %d dynamic dep(s), closure %s)\n",
				path, receipt.Gate, receipt.Status, receipt.Complete, len(receipt.Inputs), len(receipt.DynamicDeps), receipt.InputClosureSHA256[:12])
			return nil
		},
	}

	cmd.Flags().StringVar(&gate, "gate", "", "gate id (e.g. build, unit_tests, security)")
	cmd.Flags().StringVar(&status, "status", "", "run outcome: pass, fail, or partial")
	cmd.Flags().StringVar(&inputs, "inputs", "", "comma-separated input globs relative to the project root (** supported)")
	cmd.Flags().StringVar(&dynamicDeps, "dynamic-deps", "", "comma-separated dynamic dependency files (e.g. go.sum)")
	cmd.Flags().StringVar(&command, "command", "", "command that produced the evidence")
	cmd.Flags().BoolVar(&partial, "partial", false, "the run did not cover its full scope; evidence is never reusable")
	return cmd
}

// resolveGatesChangeClass returns the declared change class, or the empty
// class when none was given so Decide derives it from the change set.
func resolveGatesChangeClass(value string) (gates.ChangeKind, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return gates.ParseChangeKind(value)
}

func writeGatesJSON(w io.Writer, receipt gates.ApplicabilityReceipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode receipt: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func printGateDecisions(w io.Writer, receipt gates.ApplicabilityReceipt, receiptPath string) {
	fmt.Fprintf(w, "%s (%s): %d changed path(s)\n", receipt.SpecID, receipt.ChangeClass, len(receipt.ChangedPaths))
	fmt.Fprintf(w, "change risk: %s %s (declared %s, effective %s)\n",
		receipt.ChangeRisk.Tier, receipt.ChangeRisk.Decision,
		receipt.ChangeRisk.DeclaredClass, receipt.ChangeRisk.EffectiveClass)
	for _, decision := range receipt.Decisions {
		fmt.Fprintf(w, "%s: %s — %s\n", decision.Gate, decision.Applicability, decision.Reason)
	}
	fmt.Fprintf(w, "receipt: %s\n", receiptPath)
}
