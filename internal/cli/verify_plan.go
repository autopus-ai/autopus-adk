package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// verifyPlanStep is one step of the Stop hook's plan. Scope says how much it
// runs: "build", "typecheck", "changed-tests" (only the tests the change edited, at most
// half the budget), "scenarios" (the project's exec QA cases) or "whole"
// (the changed packages or the project's whole suite). Argv is exactly what
// the hook runs; Run is the same command for a person or agent to read (Go
// steps without -json).
type verifyPlanStep struct {
	Name  string   `json:"name"`
	Scope string   `json:"scope"`
	Dir   string   `json:"dir"`
	Argv  []string `json:"argv"`
	Run   string   `json:"run"`
}

type verifyPlanOutput struct {
	Root    string           `json:"root"`
	Changed []string         `json:"changed"`
	Budget  string           `json:"budget"`
	HookOff bool             `json:"hook_off,omitempty"`
	Steps   []verifyPlanStep `json:"steps"`
	Note    string           `json:"note"`
}

// verifyPlanNote tells agents which steps are theirs: Pilot C measured
// 48-76% of a session's wall time waiting on whole-package test runs the
// agent and its reviewers started themselves, the same runs the hook does.
const verifyPlanNote = "Run the build, typecheck and changed-tests steps yourself (or a narrower -run / test file) while you work. " +
	"The Stop hook runs every step, the whole changed packages included, under its %s budget when the session stops; " +
	"do not start a whole package or suite run yourself with a long timeout and poll it: read the hook's result with " +
	"`auto verify stop --last`."

func newVerifyPlanCmd() *cobra.Command {
	var (
		asJSON bool
		budget time.Duration
	)
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Print the build and test steps the Stop hook would run for the current changes (runs nothing)",
		Long: "Lists the steps `auto verify stop` would run on the work tree's changes, in order: the build, the tests the " +
			"change edited (Go: -run over the changed test functions), the project's exec QA cases, and the changed packages " +
			"or suites whole. Nothing is run. Use the narrow steps while working and leave the whole ones to the hook.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			root, changed, ok := verifygate.ChangedFiles(cmd.Context(), dir)
			if !ok {
				return fmt.Errorf("auto verify plan: %s is not inside a git work tree", dir)
			}
			out := buildVerifyPlan(root, changed, planBudget(cmd, root, budget))
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			printVerifyPlan(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the plan as JSON")
	cmd.Flags().DurationVar(&budget, "budget", 8*time.Minute, "The Stop hook's budget (hooks.verify_stop_budget may lower it)")
	return cmd
}

// planBudget is the budget the generated Stop hook runs with: the flag when
// given, else a valid hooks.verify_stop_budget (the generator passes it as
// --budget), else the default.
func planBudget(cmd *cobra.Command, root string, flag time.Duration) time.Duration {
	if cmd.Flags().Changed("budget") {
		return flag
	}
	if cfg := nearestConfig(root); cfg != nil && cfg.Hooks.VerifyStopBudget != "" {
		if d, err := cfg.Hooks.VerifyStopBudgetDuration(); err == nil {
			return d
		}
	}
	return flag
}

func buildVerifyPlan(root string, changed []string, budget time.Duration) verifyPlanOutput {
	out := verifyPlanOutput{Root: root, Changed: changed, Budget: budget.String(), Steps: []verifyPlanStep{},
		HookOff: strings.EqualFold(os.Getenv("AUTOPUS_VERIFY_STOP"), "off"), Note: fmt.Sprintf(verifyPlanNote, budget)}
	if out.Changed == nil {
		out.Changed = []string{}
	}
	if len(changed) == 0 {
		out.Note = "Nothing differs from HEAD, so the Stop hook runs nothing."
		return out
	}
	for _, s := range verifygate.Plan(root, changed) {
		out.Steps = append(out.Steps, verifyPlanStep{Name: s.Name, Scope: planScope(s), Dir: s.Dir,
			Argv: s.Argv, Run: readableCommand(s)})
	}
	if len(out.Steps) == 0 {
		out.Note = "No build or test toolchain the Stop hook knows covers these files, so it runs nothing; " +
			"test them with the project's own commands."
	}
	return out
}

func planScope(s verifygate.Step) string {
	switch {
	case s.CoveredLater:
		return "changed-tests"
	case len(s.Argv) > 1 && s.Argv[0] == "go" && s.Argv[1] == "build":
		return "build"
	case strings.HasSuffix(s.Name, " run typecheck"):
		return "typecheck"
	case slices.Contains(s.Argv, "qa") && slices.Contains(s.Argv, "exec"):
		return "scenarios"
	}
	return "whole"
}

// readableCommand is the step's command as a shell line, without the -json
// a Go test step uses for the hook's own parsing.
func readableCommand(s verifygate.Step) string {
	words := make([]string, 0, len(s.Argv))
	for _, a := range s.Argv {
		if s.GoTestJSON && a == "-json" {
			continue
		}
		words = append(words, shellQuote(a))
	}
	return strings.Join(words, " ")
}

func printVerifyPlan(w io.Writer, out verifyPlanOutput) {
	if out.HookOff {
		fmt.Fprintln(w, "AUTOPUS_VERIFY_STOP=off: the Stop hook is off in this shell.")
	}
	fmt.Fprintf(w, "Changed files: %d (root %s)\n", len(out.Changed), out.Root)
	for i, s := range out.Steps {
		fmt.Fprintf(w, "%d. [%s] %s\n", i+1, s.Scope, s.Run)
	}
	fmt.Fprintln(w, out.Note)
}
