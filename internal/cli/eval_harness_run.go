package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// newEvalHarnessRunCmd runs the deterministic lane (REQ-HE-03): the
// harness_eval_result.v1 document alone on stdout (and in --output), human
// guidance on stderr, and the exit code of harneval.ExitCode.
func newEvalHarnessRunCmd(deps evalHarnessDeps, dir *string) *cobra.Command {
	var format, output string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Evaluate the golden set against the committed baseline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireHarnessJSON(format); err != nil {
				return err
			}
			result, err := harneval.Run(cmd.Context(), *dir, deps.run)
			if err != nil {
				return fmt.Errorf("harness eval: %w", err)
			}
			if err := harneval.Emit(result, output, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
				return err
			}
			return harnessExit(harneval.ExitCode(result))
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "output format (json)")
	cmd.Flags().StringVar(&output, "output", "", "also write the result document to this file")
	return cmd
}

// harnessDigests is the `auto eval harness digest` document: the digests a
// run compares and a live protocol freezes, for the tree at --dir.
type harnessDigests struct {
	SetVersion     string              `json:"set_version"`
	SetDigest      string              `json:"set_digest"`
	AgentSetDigest string              `json:"agent_set_digest"`
	SurfaceDigest  string              `json:"surface_digest"`
	Tasks          []harnessTaskDigest `json:"tasks"`
}

// harnessTaskDigest is one task's entry of the set digest.
type harnessTaskDigest struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	State             string `json:"state"`
	ExpectationDigest string `json:"expectation_digest"`
}

// newEvalHarnessDigestCmd prints the set, agent set, per-task expectation,
// and default surface digests. The surface is generated exactly as a run
// generates it: pinned, under the sentinel, in a fresh temp root.
func newEvalHarnessDigestCmd(deps evalHarnessDeps, dir *string) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Print the golden set and default surface digests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireHarnessJSON(format); err != nil {
				return err
			}
			set, err := harneval.LoadSet(*dir)
			if err != nil {
				return fmt.Errorf("load golden set: %w", err)
			}
			generation, err := harneval.Generate(cmd.Context(), set, deps.run.Adapters)
			if err != nil {
				return fmt.Errorf("generate surface: %w", err)
			}
			defer func() { _ = generation.Close() }()
			surface, err := harneval.SurfaceDigest(generation.Surfaces[""].Root)
			if err != nil {
				return err
			}
			doc := harnessDigests{
				SetVersion:     set.Manifest.SetVersion,
				SetDigest:      harneval.SetDigest(set),
				AgentSetDigest: harneval.AgentSetDigest(set),
				SurfaceDigest:  surface,
				Tasks:          make([]harnessTaskDigest, 0, len(set.Tasks)),
			}
			for _, task := range set.Tasks {
				doc.Tasks = append(doc.Tasks, harnessTaskDigest{
					ID: task.ID, Kind: task.Kind, State: task.Status.State,
					ExpectationDigest: harneval.ExpectationDigest(task),
				})
			}
			return writeHarnessJSON(cmd.OutOrStdout(), doc)
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "output format (json)")
	return cmd
}
