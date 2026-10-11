package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/codexruntime"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeCodexCatalogProbe reads the Codex model catalog for a composition
// run. Tests replace it with a fixture probe and restore it with t.Cleanup.
var composeCodexCatalogProbe driftCatalogProbe = codexruntime.ProbeModelCatalog

// captureComposeSnapshot probes the Codex catalog once for a composition run.
// The table and that run's generation both read the returned snapshot, so they
// cannot see two different catalogs; doctor drift shares a snapshot the same
// way. A failed probe still freezes an empty catalog, which every reader
// resolves exactly as an adapter whose own probe failed. A project without
// Codex probes nothing and gets the zero snapshot.
func captureComposeSnapshot(ctx context.Context, cfg *config.HarnessConfig) driftGenerationSnapshot {
	if cfg == nil || !containsPlatform(cfg.Platforms, "codex") {
		return driftGenerationSnapshot{}
	}
	return captureDriftGenerationSnapshot(ctx, "codex", composeCodexCatalogProbe)
}

// updateHarnessPlatformWith updates one platform through an adapter that reads
// the run's snapshot. The zero snapshot builds the same adapter as
// updateHarnessPlatform, which probes for itself.
func updateHarnessPlatformWith(
	ctx context.Context,
	dir string,
	platform string,
	cfg *config.HarnessConfig,
	snapshot driftGenerationSnapshot,
) (bool, error) {
	platformAdapter := driftAdapterFor(platform, dir, snapshot)
	if platformAdapter == nil {
		return false, nil
	}
	_, err := platformAdapter.Update(ctx, cfg)
	return true, err
}

// generatePlatformFilesWith generates every configured platform for init with
// adapters that read the run's snapshot.
func generatePlatformFilesWith(
	ctx context.Context,
	dir string,
	cfg *config.HarnessConfig,
	cmd *cobra.Command,
	snapshot driftGenerationSnapshot,
) error {
	effectiveCfg := applyFlagCC21Overrides(cfg, globalFlagsFromContext(cmd.Context()))

	for _, p := range cfg.Platforms {
		platformAdapter := driftAdapterFor(p, dir, snapshot)
		if platformAdapter == nil {
			tui.Warnf(cmd.OutOrStdout(), "알 수 없는 플랫폼 %q, 건너뜀", p)
			continue
		}
		if _, err := platformAdapter.Generate(ctx, effectiveCfg); err != nil {
			return fmt.Errorf("플랫폼 %q 파일 생성 실패: %w", p, err)
		}
		tui.Success(cmd.OutOrStdout(), p)
	}
	return nil
}

// applyQualityHarnessWith regenerates the harness after a confirmed quality
// change with real adapters built from the run's snapshot. It prints exactly
// what applyQualityHarness prints, which keeps qualityPlatformUpdater for the
// commands that never draw a composition table.
func applyQualityHarnessWith(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	retryCommand string,
	snapshot driftGenerationSnapshot,
) error {
	update := func(ctx context.Context, dir, platform string, cfg *config.HarnessConfig) (bool, error) {
		return updateHarnessPlatformWith(ctx, dir, platform, cfg, snapshot)
	}
	return applyQualityHarnessPlatformsUsing(cmd, dir, cfg, cfg.Platforms, retryCommand, update)
}
