package cli

import (
	"context"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

type platformAdapterOptions struct {
	codexModelCatalog      []byte
	codexModelCatalogFixed bool
}

type platformAdapterFactory func(string, platformAdapterOptions) adapter.PlatformAdapter
type platformDescriptorLookup func(string) (platformDescriptor, bool)

type platformCleanReceipt struct {
	changedPaths []string
}

type platformCleaner func(context.Context, string) (platformCleanReceipt, error)
type platformPruneRoots func(*config.HarnessConfig) []string
type platformDriftOptions func([]byte, bool) platformAdapterOptions

type platformDescriptor struct {
	name              string
	newAdapter        platformAdapterFactory
	clean             platformCleaner
	pruneRoots        platformPruneRoots
	driftOptions      platformDriftOptions
	probeDriftCatalog bool
	repairDetail      bool
	rollbackPlatforms []string
}

var lifecyclePlatformCatalog = [...]platformDescriptor{
	{
		name: "claude-code",
		newAdapter: func(root string, _ platformAdapterOptions) adapter.PlatformAdapter {
			return claude.NewWithRoot(root)
		},
		pruneRoots: func(*config.HarnessConfig) []string { return claude.PruneRoots() },
	},
	{
		name: "codex",
		newAdapter: func(root string, options platformAdapterOptions) adapter.PlatformAdapter {
			if options.codexModelCatalogFixed {
				return codex.NewWithRoot(root, codex.WithModelCatalog(options.codexModelCatalog))
			}
			return codex.NewWithRoot(root)
		},
		driftOptions: func(catalog []byte, fixed bool) platformAdapterOptions {
			return platformAdapterOptions{
				codexModelCatalog:      catalog,
				codexModelCatalogFixed: fixed,
			}
		},
		probeDriftCatalog: true,
		pruneRoots: func(*config.HarnessConfig) []string {
			return codex.PruneRoots()
		},
	},
	{
		name: "antigravity-cli",
		newAdapter: func(root string, _ platformAdapterOptions) adapter.PlatformAdapter {
			return antigravity.NewWithRoot(root)
		},
		pruneRoots: func(*config.HarnessConfig) []string { return antigravity.PruneRoots() },
	},
	{
		name: "opencode",
		newAdapter: func(root string, _ platformAdapterOptions) adapter.PlatformAdapter {
			return opencode.NewWithRoot(root)
		},
		pruneRoots: func(*config.HarnessConfig) []string {
			return []string{".agents/skills", ".opencode/skills"}
		},
	},
	{
		name: "omp",
		newAdapter: func(root string, _ platformAdapterOptions) adapter.PlatformAdapter {
			return omp.NewWithRoot(root)
		},
		clean: func(ctx context.Context, root string) (platformCleanReceipt, error) {
			receipt, err := omp.NewWithRoot(root).CleanWithReceipt(ctx)
			return platformCleanReceipt{changedPaths: receipt.ChangedPaths}, err
		},
		pruneRoots:   omp.PruneRoots,
		repairDetail: true,
	},
}

// @AX:ANCHOR [AUTO]: keep platform lifecycle dispatch centralized in this catalog lookup.
// @AX:REASON [AUTO]: doctor, init, add/remove, update, preview, drift, and workspace flows share this adapter and cleanup boundary.
func lookupPlatformDescriptor(name string) (platformDescriptor, bool) {
	for _, descriptor := range lifecyclePlatformCatalog {
		if descriptor.name == name {
			return descriptor, true
		}
	}
	return platformDescriptor{}, false
}

func platformCatalogNames() []string {
	names := make([]string, 0, len(lifecyclePlatformCatalog))
	for _, descriptor := range lifecyclePlatformCatalog {
		names = append(names, descriptor.name)
	}
	return names
}

func (d platformDescriptor) driftAdapter(root string, catalog []byte, fixed bool) adapter.PlatformAdapter {
	options := platformAdapterOptions{}
	if d.driftOptions != nil {
		options = d.driftOptions(catalog, fixed)
	}
	return d.newAdapter(root, options)
}
func (d platformDescriptor) adapter(root string) adapter.PlatformAdapter {
	return d.newAdapter(root, platformAdapterOptions{})
}

func (d platformDescriptor) Generate(ctx context.Context, root string, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	return d.adapter(root).Generate(ctx, cfg)
}

func (d platformDescriptor) Update(ctx context.Context, root string, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	return d.adapter(root).Update(ctx, cfg)
}

func (d platformDescriptor) Validate(ctx context.Context, root string) ([]adapter.ValidationError, error) {
	return d.adapter(root).Validate(ctx)
}

func (d platformDescriptor) Clean(ctx context.Context, root string) (platformCleanReceipt, error) {
	if d.clean != nil {
		return d.clean(ctx, root)
	}
	return platformCleanReceipt{}, d.adapter(root).Clean(ctx)
}

func (d platformDescriptor) PreviewPruneRoots(cfg *config.HarnessConfig) []string {
	if d.pruneRoots == nil {
		return nil
	}
	return d.pruneRoots(cfg)
}
