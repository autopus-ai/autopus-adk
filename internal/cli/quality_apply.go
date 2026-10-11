package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

var qualityPlatformUpdater = updateHarnessPlatform

func applyQualityHarness(cmd *cobra.Command, dir string, cfg *config.HarnessConfig, retryCommand string) error {
	return applyQualityHarnessPlatforms(cmd, dir, cfg, cfg.Platforms, retryCommand)
}

func applyQualityHarnessPlatforms(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	platforms []string,
	retryCommand string,
) error {
	return applyQualityHarnessPlatformsUsing(cmd, dir, cfg, platforms, retryCommand, qualityPlatformUpdater)
}

// applyQualityHarnessPlatformsUsing owns the apply report so every caller,
// whichever updater it passes, prints the same lines.
func applyQualityHarnessPlatformsUsing(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	platforms []string,
	retryCommand string,
	update func(context.Context, string, string, *config.HarnessConfig) (bool, error),
) error {
	out := cmd.OutOrStdout()
	updated := 0
	var failures []string
	codexApplied := false

	for _, platform := range platforms {
		supported, err := update(commandContext(cmd), dir, platform, cfg)
		if !supported {
			fmt.Fprintf(out, "  skipped unknown platform: %s\n", platform)
			continue
		}
		if err != nil {
			fmt.Fprintf(out, "  failed %s: %v\n", platform, err)
			failures = append(failures, fmt.Sprintf("%s: %v", platform, err))
			continue
		}
		fmt.Fprintf(out, "  applied %s\n", platform)
		updated++
		if platform == "codex" {
			codexApplied = true
		}
	}

	fmt.Fprintf(out, "quality.applied_platforms = %d\n", updated)
	if codexApplied {
		fmt.Fprintln(out, "Start a new Codex session to load the updated managed agents.")
	}
	if len(failures) > 0 {
		fmt.Fprintln(out, "quality.apply_partial = true")
		fmt.Fprintf(out, "Retry: %s\n", qualityRetryCommand(dir, retryCommand))
		return fmt.Errorf("quality saved but harness apply failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func qualityRetryCommand(dir, retryCommand string) string {
	configPath := filepath.Join(dir, "autopus.yaml")
	if absolute, err := filepath.Abs(configPath); err == nil {
		configPath = absolute
	}
	suffix := strings.TrimSpace(strings.TrimPrefix(retryCommand, "auto "))
	return "auto --config " + shellQuoteQualityArg(configPath) + " " + suffix
}

func shellQuoteQualityArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd == nil || cmd.Context() == nil {
		return context.Background()
	}
	return cmd.Context()
}
