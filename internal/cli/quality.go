package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// qualityCmdLong is the help of `auto quality`; the composition step it
// describes is SPEC-MODELUX-001 (REQ-MUX-23).
const qualityCmdLong = `Show or change quality.default, quality.providers, and
quality.supervisor_model_policy in autopus.yaml.

Use ` + "`auto quality <preset> --apply`" + ` for the global fallback,
` + "`auto quality provider <provider> <preset|inherit> --apply`" + ` for one provider,
or run ` + "`auto quality`" + ` without arguments to compose per-agent models.

Without arguments, in a terminal, the agent model composition comes first:
one row per agent with its tier and one column per installed platform.
Claude Code and Codex cells are the model · effort generation would write
(Codex resolved with one catalog read). The OMP column is the route its
profile requests (requested route · before catalog check); OpenCode and
Antigravity show platform default. Each written balanced or ultra preset is
labelled current default, previous-version default (can update), or
customized (N differ). ` + "`auto quality show --agents`" + ` prints the same
table read-only.

Change the composition? takes:
  Enter  keep; nothing is written
  u      update previous-version default presets (shown only when one exists)
  g      one tier (fable, opus, sonnet, haiku) per role group, Enter keeps:
           Design·Judgment: planner, architect, spec-writer, security-auditor
           Implementation: executor, debugger, deep-worker, devops,
             frontend-specialist, perf-engineer
           Verification·Review: reviewer, validator, tester, ux-validator
           Exploration·Support: explorer, annotator
  a      advanced: one agent=tier line per agent, an empty line finishes
  t      per-tool settings; global, claude-code, codex, and omp work too

The preview marks every changed cell with * and lists the keys it writes;
only y writes, e keeps editing. A result equal to the current balanced or
ultra default binds that preset, writing the preset too when it is missing
or a previous-version copy; any other result is saved as
quality.presets.custom with quality.default: custom. A customized balanced
or ultra preset is never modified. When quality.providers.claude or .codex
names another preset, the step first asks whether that tool follows (y
removes the override). With --apply a confirmed change regenerates the
harness from the same catalog read; otherwise run ` + "`auto update`" + `.

The composition step and the per-tool editors refuse to write, leaving
autopus.yaml unchanged, while the quality block holds a ${NAME} value of a
set environment variable or a YAML alias or merge key; edit it by hand then.`

func newQualityCmd() *cobra.Command {
	return newQualityCmdWithOMPDependencies(defaultOMPPlatformDependencies())
}

func newQualityCmdWithOMPDependencies(deps ompPlatformDependencies) *cobra.Command {
	deps = normalizeOMPPlatformDependencies(deps)
	var apply bool
	cmd := &cobra.Command{
		Use:   "quality [preset]",
		Short: "Choose quality modes and OMP agent models",
		Long:  qualityCmdLong,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return runQualitySet(cmd, args[0], apply)
			}
			return runQualityInteractive(cmd, apply, deps)
		},
	}
	cmd.PersistentFlags().BoolVar(&apply, "apply", false, "Apply the saved quality or supervisor policy to this project's harness files")
	cmd.AddCommand(newQualityShowCmd())
	cmd.AddCommand(newQualitySetCmd(&apply))
	cmd.AddCommand(newQualityProviderCmd(&apply))
	cmd.AddCommand(newQualitySupervisorCmd(&apply))
	return cmd
}

func newQualitySupervisorCmd(apply *bool) *cobra.Command {
	return &cobra.Command{
		Use:     "supervisor <inherit|quality>",
		Aliases: []string{"model"},
		Short:   "Choose whether Codex inherits the user model or follows quality mode",
		Long: "Choose ownership of the primary Codex session model. `inherit` removes a known Autopus-managed root profile; " +
			"`quality` uses the selected quality profile. Explicit user-owned model settings remain preserved.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQualitySupervisorSet(cmd, args[0], apply != nil && *apply)
		},
	}
}

func newQualityShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show",
		Aliases: []string{"status"},
		Short:   "Show current quality mode and supervisor policy",
		Args:    cobra.NoArgs,
		RunE:    runQualityShow,
	}
	cmd.Flags().Bool("agents", false, "Also print the agent × platform model table, read-only")
	return cmd
}

func newQualitySetCmd(apply *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "set <preset>",
		Short: "Set quality.default",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQualitySet(cmd, args[0], apply != nil && *apply)
		},
	}
}

func runQualityShow(cmd *cobra.Command, _ []string) error {
	dir, cfg, err := loadQualityConfig(cmd)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "quality.default = %s\n", cfg.Quality.Default)
	writeQualityProviderStatus(out, cfg.Quality, config.QualityProviderClaude)
	writeQualityProviderStatus(out, cfg.Quality, config.QualityProviderCodex)
	fmt.Fprintf(out, "quality.supervisor_model_policy = %s\n", cfg.Quality.EffectiveSupervisorModelPolicy())
	writeOMPRoleModelPolicyStatus(out, cfg.RoleModelPolicy)
	fmt.Fprintf(out, "config = %s\n", filepath.Join(dir, "autopus.yaml"))
	fmt.Fprintf(out, "available = %s\n", strings.Join(orderedQualityPresets(cfg), ", "))
	if agents, _ := cmd.Flags().GetBool("agents"); agents {
		return writeQualityAgentTable(cmd, cfg)
	}
	return nil
}

func runQualitySet(cmd *cobra.Command, preset string, apply bool) error {
	dir, cfg, err := loadQualityConfig(cmd)
	if err != nil {
		return err
	}
	preset = strings.TrimSpace(preset)
	if err := validateQualityChoice(cfg, preset); err != nil {
		return err
	}
	if err := saveQualityDefault(dir, cfg, preset); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "quality.default = %s\n", preset)
	if apply {
		return applyQualityHarness(cmd, dir, cfg, "auto quality "+preset+" --apply")
	}
	return nil
}

func runQualitySupervisorSet(cmd *cobra.Command, policy string, apply bool) error {
	dir, cfg, err := loadQualityConfig(cmd)
	if err != nil {
		return err
	}
	policy = strings.TrimSpace(policy)
	cfg.Quality.SupervisorModelPolicy = policy
	if !cfg.Quality.IsValidSupervisorModelPolicy() || policy == "" {
		return fmt.Errorf("unknown supervisor model policy %q (available: inherit, quality)", policy)
	}
	if err := saveQualitySupervisorPolicy(dir, cfg, policy); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "quality.supervisor_model_policy = %s\n", policy)
	if apply {
		return applyQualityHarness(cmd, dir, cfg, "auto quality supervisor "+policy+" --apply")
	}
	return nil
}

// runQualityInteractive is `auto quality` without arguments: the composition
// step comes first and still reaches every existing editor
// (SPEC-MODELUX-001 REQ-MUX-16).
func runQualityInteractive(cmd *cobra.Command, apply bool, deps ompPlatformDependencies) error {
	dir, cfg, err := loadQualityConfig(cmd)
	if err != nil {
		return err
	}
	return runQualityComposeEntry(cmd, dir, cfg, apply, deps)
}

func loadQualityConfig(cmd *cobra.Command) (string, *config.HarnessConfig, error) {
	flags := globalFlagsFromContext(cmd.Context())
	if err := validateQualityConfigPath(flags.ConfigPath); err != nil {
		return "", nil, err
	}
	dir, err := resolveConfigDir(cmd, flags.ConfigPath)
	if err != nil {
		return "", nil, fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.LoadPreview(dir)
	if err != nil {
		return "", nil, fmt.Errorf("load config: %w", err)
	}
	return dir, cfg, nil
}

func validateQualityConfigPath(configPath string) error {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return nil
	}
	info, err := os.Stat(configPath)
	if err == nil && info.IsDir() {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect --config path: %w", err)
	}
	if filepath.Base(filepath.Clean(configPath)) != "autopus.yaml" {
		return fmt.Errorf("quality commands require --config to be a project directory or a file named autopus.yaml")
	}
	return nil
}

func validateQualityChoice(cfg *config.HarnessConfig, preset string) error {
	if !config.IsValidQualityPresetName(preset) {
		return fmt.Errorf(
			"invalid quality preset name %q (use 1-64 ASCII letters, digits, hyphens, or underscores)",
			preset,
		)
	}
	if _, ok := cfg.Quality.Presets[preset]; ok {
		return nil
	}
	return fmt.Errorf("unknown quality preset %q (available: %s)", preset, strings.Join(orderedQualityPresets(cfg), ", "))
}

func orderedQualityPresets(cfg *config.HarnessConfig) []string {
	seen := make(map[string]bool, len(cfg.Quality.Presets))
	var ordered []string
	for _, name := range []string{"ultra", "balanced"} {
		if _, ok := cfg.Quality.Presets[name]; ok {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}
	var rest []string
	for name := range cfg.Quality.Presets {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(ordered, rest...)
}

func readQualityChoice(cmd *cobra.Command, options []string) (string, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && f == os.Stdin && !isStdinTTY() {
		return "", fmt.Errorf("interactive quality selection requires a TTY; use `auto quality <preset>`")
	}
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		return "", fmt.Errorf("read quality choice: %w", err)
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", fmt.Errorf("quality choice is required")
	}
	for i, preset := range options {
		if answer == fmt.Sprintf("%d", i+1) || strings.EqualFold(answer, preset) {
			return preset, nil
		}
	}
	return "", fmt.Errorf("invalid quality choice %q", answer)
}
