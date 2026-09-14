package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// chooseQualityTarget asks which surface to configure: the shared quality mode
// or one coding tool's agent models. Only tools whose generated agents carry a
// model are listed; the rest are named in a note so their absence is a stated
// fact rather than an omission.
func chooseQualityTarget(cmd *cobra.Command, cfg *config.HarnessConfig) (string, error) {
	input := cmd.InOrStdin()
	if file, ok := input.(*os.File); ok && file == os.Stdin && !isStdinTTY() {
		return "", fmt.Errorf("interactive quality selection requires a TTY; use an explicit quality or profile command")
	}
	// Reuse one reader across every menu; creating independent buffered
	// readers would discard later answers when input arrives in one batch.
	cmd.SetIn(bufio.NewReader(input))
	out := cmd.OutOrStdout()
	tools := configuredQualityModelTools(cfg)
	options := make([]string, 0, len(tools)+1)
	options = append(options, "global")
	fmt.Fprintln(out, "What would you like to configure?")
	fmt.Fprintln(out, "  1) All platforms (quality mode)")
	for index, tool := range tools {
		fmt.Fprintf(out, "  %d) %s (agent models)\n", index+2, tool.label)
		options = append(options, tool.platform)
	}
	if inherited := inheritOnlyQualityToolLabels(cfg); len(inherited) > 0 {
		fmt.Fprintf(out, "%s inherit the session model and have no per-agent model to set.\n",
			strings.Join(inherited, " and "))
	}
	fmt.Fprint(out, "Choose: ")
	return readQualityChoice(cmd, options)
}

func runOMPQualityInteractive(cmd *cobra.Command, root string, cfg *config.HarnessConfig, deps ompPlatformDependencies) error {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Choose OMP quality:\n  1) balanced\n  2) ultra\n  3) custom (pick a model per agent)")
	fmt.Fprint(out, "Choose: ")
	mode, err := readQualityChoice(cmd, []string{"balanced", "ultra", "custom"})
	if err != nil {
		return err
	}
	runner := deps.newRunner()
	family, pins := "", []string(nil)
	switch {
	case mode == "custom":
		// Pins govern the agents the operator names; every other bundled agent
		// still resolves through the base profile, and that derivation needs an
		// anchor family.
		mode = ompCustomBaseProfile
		if family, err = readOMPQualityFamily(cmd); err != nil {
			return err
		}
		if pins, err = readOMPCustomAgentPins(cmd.Context(), cmd, runner); err != nil {
			return err
		}
		if len(pins) == 0 {
			fmt.Fprintln(out, "No agent was pinned; nothing changed.")
			return nil
		}
	default:
		if _, custom := cfg.RoleModelPolicy.Profiles[mode]; custom {
			fmt.Fprintln(out, "Keeping the models in your custom profile.")
			break
		}
		if family, err = readOMPQualityFamily(cmd); err != nil {
			return err
		}
	}
	opts, err := newOMPProfileApplyOptions(mode, family, pins, false, false)
	if err != nil {
		return err
	}
	preview, err := planOMPProfile(cmd.Context(), root, opts, runner)
	if err != nil {
		return err
	}
	renderOMPQualitySummary(out, preview)
	if len(preview.Blockers) > 0 {
		return ompProfileUnavailableError{blockers: preview.Blockers}
	}
	fmt.Fprint(out, "Apply these models? [y/N]: ")
	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read apply confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(out, "Cancelled; no settings changed.")
		return nil
	}
	// The existing transaction revalidates the catalog and configuration
	// after confirmation and retains its normal rollback guarantees.
	if _, err := applyOMPProfile(cmd.Context(), root, opts, runner, deps.activate); err != nil {
		return err
	}
	fmt.Fprintln(out, "OMP models applied. Start a new OMP session to use them.")
	return nil
}

// readOMPQualityFamily asks which family anchors a built-in derivation. The
// stored value is canonical; the menu shows the CLI spellings operators use.
func readOMPQualityFamily(cmd *cobra.Command) (string, error) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Choose model family:\n  1) GPT\n  2) Claude")
	fmt.Fprint(out, "Choose: ")
	return readQualityChoice(cmd, []string{"gpt", "claude"})
}

func renderOMPQualitySummary(out io.Writer, preview ompProfileApplyPreviewPayload) {
	fmt.Fprintf(out, "\nOMP %s — model preview (no changes yet)\n", preview.Name)
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "Agent\tModel\tThinking")
	for _, row := range preview.Agents {
		name := row.Agent
		if row.Source == ompProfileSourceAgent {
			name += "*"
		}
		model, thinking := row.EffectiveSelector, row.EffectiveThinking
		if row.Availability != ompProfileAvailabilityAvailable {
			model, thinking = row.RequestedSelector+" (unavailable)", row.RequestedThinking
		}
		fmt.Fprintf(table, "%s\t%s\t%s\n", name, model, thinking)
	}
	_ = table.Flush()
	if len(preview.Persisted.Agents) > 0 {
		fmt.Fprintln(out, "* Your explicit agent overrides are kept.")
	}
	fmt.Fprintln(out, "Global quality and multi-provider review settings stay unchanged.")
}
