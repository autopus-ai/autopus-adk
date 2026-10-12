package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func updateHarnessPlatform(
	ctx context.Context,
	dir string,
	platform string,
	cfg *config.HarnessConfig,
) (bool, error) {
	return updateHarnessPlatformWith(ctx, dir, platform, cfg, driftGenerationSnapshot{})
}

func newlyDetectedPlatforms(cfg *config.HarnessConfig) []string {
	var added []string
	for _, platform := range detectInstalledPlatforms() {
		if !containsPlatform(cfg.Platforms, platform) {
			added = append(added, platform)
		}
	}
	return added
}

func appendDetectedPlatforms(cfg *config.HarnessConfig) []string {
	added := newlyDetectedPlatforms(cfg)
	cfg.Platforms = append(cfg.Platforms, added...)
	return added
}

// updateDetectedHarnessPlatformsWith adds each detected platform to cfg,
// generates it, and saves the config one platform at a time, with adapters
// that read the run's Codex catalog snapshot: a detected Codex then writes
// the cells the composition table showed instead of probing a catalog of its
// own. The zero snapshot keeps each adapter's own probe.
func updateDetectedHarnessPlatformsWith(
	ctx context.Context,
	dir string,
	cfg *config.HarnessConfig,
	platforms []string,
	flags globalFlags,
	out io.Writer,
	snapshot driftGenerationSnapshot,
) (*config.HarnessConfig, int, bool, []string) {
	current := cfg
	updated := 0
	codexUpdated := false
	var platformErrors []string

	for _, platform := range platforms {
		descriptor, ok := lookupPlatformDescriptor(platform)
		if !ok {
			platformErrors = append(platformErrors, fmt.Sprintf("%s: unsupported platform", platform))
			continue
		}
		descriptor = descriptor.withSnapshot(snapshot)
		fmt.Fprintf(out, "  → Updating %s...\n", updatePlatformLabel(platform))
		candidate, err := cloneHarnessConfig(current)
		if err == nil {
			candidate.Platforms = append(candidate.Platforms, platform)
			_, err = config.MigrateOrchestraConfig(candidate)
		}
		if err == nil {
			effectiveCfg := applyFlagCC21Overrides(candidate, flags)
			err = updatePlatformThenSave(ctx, dir, descriptor, effectiveCfg, candidate, config.Save)
		}
		if err != nil {
			fmt.Fprintf(out, "  ✗ %s: %v\n", platform, err)
			platformErrors = append(platformErrors, fmt.Sprintf("%s: %v", platform, err))
			continue
		}

		current = candidate
		updated++
		codexUpdated = codexUpdated || platform == "codex"
		fmt.Fprintf(out, "  ✓ %s updated\n", platform)
	}
	return current, updated, codexUpdated, platformErrors
}

func withPlatformAddOwnershipHandoff(
	descriptor platformDescriptor,
	cfg *config.HarnessConfig,
	lookup platformDescriptorLookup,
) platformDescriptor {
	owner, ok := sharedSurfaceOwnerAfterTransition(descriptor.name, cfg)
	if !ok && descriptor.name == "opencode" &&
		containsPlatform(cfg.Platforms, "codex") && containsPlatform(cfg.Platforms, "opencode") {
		owner, ok = "codex", true
	}
	if !ok {
		return descriptor
	}
	ownerDescriptor, ok := lookup(owner)
	if !ok {
		return descriptor
	}

	primaryFactory := descriptor.newAdapter
	descriptor.newAdapter = func(root string, options platformAdapterOptions) adapter.PlatformAdapter {
		return &platformGenerateHandoffAdapter{
			PlatformAdapter: primaryFactory(root, options),
			root:            root,
			owner:           ownerDescriptor,
		}
	}
	descriptor.rollbackPlatforms = append(descriptor.rollbackPlatforms, owner)
	return descriptor
}

func withPlatformRemoveOwnershipHandoff(
	descriptor platformDescriptor,
	cfg *config.HarnessConfig,
	lookup platformDescriptorLookup,
) platformDescriptor {
	owner, ok := sharedSurfaceOwnerAfterTransition(descriptor.name, cfg)
	if !ok {
		return descriptor
	}
	ownerDescriptor, ok := lookup(owner)
	if !ok {
		return descriptor
	}

	primaryFactory := descriptor.newAdapter
	descriptor.newAdapter = func(root string, options platformAdapterOptions) adapter.PlatformAdapter {
		return &platformCleanHandoffAdapter{
			PlatformAdapter: primaryFactory(root, options),
			root:            root,
			owner:           ownerDescriptor,
			cfg:             cfg,
		}
	}
	descriptor.rollbackPlatforms = append(descriptor.rollbackPlatforms, owner)
	return descriptor
}

func sharedSurfaceOwnerAfterTransition(
	transitionedPlatform string,
	cfg *config.HarnessConfig,
) (string, bool) {
	if transitionedPlatform != "codex" && transitionedPlatform != "opencode" {
		return "", false
	}
	owner := ""
	if containsPlatform(cfg.Platforms, "opencode") {
		owner = "opencode"
	} else if containsPlatform(cfg.Platforms, "codex") {
		owner = "codex"
	}
	if owner == "" || owner == transitionedPlatform {
		return "", false
	}
	return owner, true
}

type platformGenerateHandoffAdapter struct {
	adapter.PlatformAdapter
	root  string
	owner platformDescriptor
}

func (a *platformGenerateHandoffAdapter) Generate(
	ctx context.Context,
	cfg *config.HarnessConfig,
) (*adapter.PlatformFiles, error) {
	files, err := a.PlatformAdapter.Generate(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := a.owner.Update(ctx, a.root, cfg); err != nil {
		return nil, fmt.Errorf("%s 공유 파일 갱신 실패: %w", a.owner.name, err)
	}
	return files, nil
}

type platformCleanHandoffAdapter struct {
	adapter.PlatformAdapter
	root  string
	owner platformDescriptor
	cfg   *config.HarnessConfig
}

func (a *platformCleanHandoffAdapter) Clean(ctx context.Context) error {
	if err := a.PlatformAdapter.Clean(ctx); err != nil {
		return err
	}
	if _, err := a.owner.Update(ctx, a.root, a.cfg); err != nil {
		return fmt.Errorf("%s 공유 파일 갱신 실패: %w", a.owner.name, err)
	}
	return nil
}
