package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// runQualityToolInteractive edits one coding tool's per-agent model selection.
// The tool keeps the relative tier vocabulary the generators already read, and
// the table shows what each tier becomes on that tool so the operator picks a
// model rather than an abstraction.
func runQualityToolInteractive(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	tool qualityModelTool,
	apply bool,
) error {
	out := cmd.OutOrStdout()
	rows, err := qualityToolAgentRows(cfg, tool.provider)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%s — agent models\n", tool.label)
	fmt.Fprintf(out, "Preset: %s (%s)\n",
		cfg.Quality.EffectiveMode(tool.provider), qualityToolPresetSource(cfg, tool))
	renderQualityToolRows(out, rows)

	agents := qualityToolTierMap(rows)
	edited, err := readQualityToolEdits(cmd, agents)
	if err != nil {
		return err
	}
	if edited == 0 {
		fmt.Fprintln(out, "No changes; nothing written.")
		return nil
	}

	preset, err := readQualityToolPresetName(cmd, qualityToolPresetName(tool))
	if err != nil {
		return err
	}
	candidate := withQualityToolPreset(cfg, tool, preset, agents)
	previewRows, err := qualityToolAgentRows(candidate, tool.provider)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\nPreview — %s preset %q (no changes yet)\n", tool.label, preset)
	renderQualityToolRows(out, previewRows)

	fmt.Fprint(out, "Apply these models? [y/N]: ")
	confirmed, err := readQualityToolConfirmation(cmd)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(out, "Cancelled; no settings changed.")
		return nil
	}
	if err := saveQualityToolPreset(dir, cfg, tool, preset, agents); err != nil {
		return err
	}
	fmt.Fprintf(out, "quality.presets.%s written; quality.providers.%s = %s\n",
		preset, tool.provider, preset)
	if apply {
		return applyQualityHarness(cmd, dir, cfg, "auto quality --apply")
	}
	fmt.Fprintln(out, "Run `auto update` to regenerate the agent files with these models.")
	return nil
}

// qualityToolPresetSource names where the tool's current mode comes from, so an
// operator can tell a provider override from the global default before editing.
func qualityToolPresetSource(cfg *config.HarnessConfig, tool qualityModelTool) string {
	if preset := strings.TrimSpace(cfg.Quality.Providers[tool.provider]); preset != "" {
		return "quality.providers." + tool.provider
	}
	return "global default"
}

func renderQualityToolRows(out io.Writer, rows []qualityToolAgentRow) {
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "Agent\tTier\tModel")
	for _, row := range rows {
		fmt.Fprintf(table, "%s\t%s\t%s\n", row.Agent, row.Tier, row.Model)
	}
	_ = table.Flush()
}

func qualityToolTierMap(rows []qualityToolAgentRow) map[string]string {
	agents := make(map[string]string, len(rows))
	for _, row := range rows {
		agents[row.Agent] = row.Tier
	}
	return agents
}

// readQualityToolEdits applies `agent=tier` lines to agents until a blank line
// ends the loop. A rejected line is reported and the loop continues, because
// aborting the whole session over one typo would discard earlier edits.
func readQualityToolEdits(cmd *cobra.Command, agents map[string]string) (int, error) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\nTiers: %s\n", strings.Join(config.QualityTiers(), ", "))
	fmt.Fprintln(out, "Edit with `agent=tier`; empty line finishes.")
	reader := bufio.NewReader(cmd.InOrStdin())
	edits := 0
	for {
		fmt.Fprint(out, "> ")
		line, err := reader.ReadString('\n')
		if strings.TrimSpace(line) == "" {
			if err != nil && err != io.EOF {
				return edits, fmt.Errorf("read tier edit: %w", err)
			}
			return edits, nil
		}
		agent, tier, parseErr := parseQualityToolTierAssignment(line)
		if parseErr != nil {
			fmt.Fprintf(out, "  %v\n", parseErr)
			if err != nil {
				return edits, nil
			}
			continue
		}
		agents[agent] = tier
		edits++
		fmt.Fprintf(out, "  %s = %s\n", agent, tier)
		if err != nil {
			return edits, nil
		}
	}
}

func readQualityToolPresetName(cmd *cobra.Command, fallback string) (string, error) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Preset name [%s]: ", fallback)
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read preset name: %w", err)
	}
	name := strings.TrimSpace(line)
	if name == "" {
		return fallback, nil
	}
	if !config.IsValidQualityPresetName(name) {
		return "", fmt.Errorf(
			"invalid quality preset name %q (use 1-64 ASCII letters, digits, hyphens, or underscores)",
			name,
		)
	}
	return name, nil
}

func readQualityToolConfirmation(cmd *cobra.Command) (bool, error) {
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("read apply confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// withQualityToolPreset returns a detached config carrying the candidate
// preset, so the preview resolves through the same config API as the generator
// without mutating the config a cancelled session leaves behind.
func withQualityToolPreset(
	cfg *config.HarnessConfig,
	tool qualityModelTool,
	preset string,
	agents map[string]string,
) *config.HarnessConfig {
	candidate := *cfg
	presets := make(map[string]config.QualityPreset, len(cfg.Quality.Presets)+1)
	for name, value := range cfg.Quality.Presets {
		presets[name] = value
	}
	tiers := make(map[string]string, len(agents))
	for agent, tier := range agents {
		tiers[agent] = tier
	}
	presets[preset] = config.QualityPreset{Agents: tiers}
	providers := make(map[string]string, len(cfg.Quality.Providers)+1)
	for provider, value := range cfg.Quality.Providers {
		providers[provider] = value
	}
	providers[tool.provider] = preset
	candidate.Quality.Presets = presets
	candidate.Quality.Providers = providers
	return &candidate
}
