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

func newQualityCmd() *cobra.Command {
	return newQualityCmdWithOMPDependencies(defaultOMPPlatformDependencies())
}

func newQualityCmdWithOMPDependencies(deps ompPlatformDependencies) *cobra.Command {
	deps = normalizeOMPPlatformDependencies(deps)
	var apply bool
	cmd := &cobra.Command{
		Use:   "quality [preset]",
		Short: "Choose quality modes and OMP agent models",
		Long: "Show or change quality.default, quality.providers, and quality.supervisor_model_policy in autopus.yaml.\n\n" +
			"Use `auto quality <preset> --apply` for the global fallback, " +
			"`auto quality provider <provider> <preset|inherit> --apply` for one provider, " +
			"or run `auto quality` without arguments for guided quality and OMP model selection.",
		Args: cobra.MaximumNArgs(1),
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
	return &cobra.Command{
		Use:     "show",
		Aliases: []string{"status"},
		Short:   "Show current quality mode and supervisor policy",
		Args:    cobra.NoArgs,
		RunE:    runQualityShow,
	}
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

func runQualityInteractive(cmd *cobra.Command, apply bool, deps ompPlatformDependencies) error {
	dir, cfg, err := loadQualityConfig(cmd)
	if err != nil {
		return err
	}
	if tools := configuredQualityModelTools(cfg); len(tools) > 0 {
		target, err := chooseQualityTarget(cmd, cfg)
		if err != nil {
			return err
		}
		for _, tool := range tools {
			if tool.platform != target {
				continue
			}
			if tool.platform == "omp" {
				return runOMPQualityInteractive(cmd, dir, cfg, deps)
			}
			return runQualityToolInteractive(cmd, dir, cfg, tool, apply)
		}
	}
	options := orderedQualityPresets(cfg)
	if len(options) == 0 {
		return fmt.Errorf("quality presets are not configured")
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Current quality: %s\n", cfg.Quality.Default)
	fmt.Fprintln(out, "Select quality mode:")
	for i, preset := range options {
		marker := " "
		if preset == cfg.Quality.Default {
			marker = "*"
		}
		desc := strings.TrimSpace(cfg.Quality.Presets[preset].Description)
		if desc != "" {
			fmt.Fprintf(out, "  %s %d) %s - %s\n", marker, i+1, preset, desc)
		} else {
			fmt.Fprintf(out, "  %s %d) %s\n", marker, i+1, preset)
		}
	}
	fmt.Fprint(out, "Choose: ")

	choice, err := readQualityChoice(cmd, options)
	if err != nil {
		return err
	}
	if err := saveQualityDefault(dir, cfg, choice); err != nil {
		return err
	}
	fmt.Fprintf(out, "quality.default = %s\n", choice)
	if apply {
		return applyQualityHarness(cmd, dir, cfg, "auto quality "+choice+" --apply")
	}
	return nil
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
