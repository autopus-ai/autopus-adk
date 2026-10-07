package harneval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// AdapterFactory builds the platform adapters for one generation root.
type AdapterFactory func(root string, pins Pins, codexCatalog []byte) []adapter.PlatformAdapter

// PinnedAdapters is the default factory: the five platforms in generation
// order with every known host probe answered from the manifest pins, and the
// codex plugin version taken from the pinned generator version rather than
// from this binary's build.
func PinnedAdapters(root string, pins Pins, codexCatalog []byte) []adapter.PlatformAdapter {
	return []adapter.PlatformAdapter{
		claude.NewWithRoot(root),
		codex.NewWithRoot(root,
			codex.WithModelCatalog(codexCatalog),
			codex.WithCLIVersion(pins.CodexCLIVersion),
			codex.WithPluginBaseVersion(pins.GeneratorVersion)),
		antigravity.NewWithRoot(root),
		opencode.NewWithRoot(root, opencode.WithCLIVersion(pins.OpencodeCLIVersion)),
		omp.NewWithRoot(root),
	}
}

// Surface is one generated variant: every platform generated into Root.
// Owned maps each platform to the slash paths its adapter reported generating.
type Surface struct {
	Root  string
	Owned map[string]map[string]bool
}

// Generation holds the generated surface of every variant in use. The default
// configuration has the key "".
type Generation struct {
	Surfaces    map[string]*Surface
	Invocations []string
	workDir     string
}

// VariantKeys returns the generated variant keys in sorted order.
func (g *Generation) VariantKeys() []string {
	keys := make([]string, 0, len(g.Surfaces))
	for key := range g.Surfaces {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Close removes every generated surface.
func (g *Generation) Close() error { return os.RemoveAll(g.workDir) }

// GenerationError reports a platform adapter that failed to generate.
type GenerationError struct {
	Variant  string
	Platform string
	Err      error
}

func (e *GenerationError) Error() string {
	return fmt.Sprintf("generate %s: %v", e.Detail(), e.Err)
}

func (e *GenerationError) Unwrap() error { return e.Err }

// Detail names the failed platform and, off the default config, its variant.
// It never carries temp paths, so a result that quotes it stays reproducible.
func (e *GenerationError) Detail() string {
	if e.Variant == "" {
		return e.Platform
	}
	return e.Platform + " variant=" + e.Variant
}

// Generate builds the default surface and one surface per distinct override
// set of the active surface tasks, each into a fresh temp root, under the
// sentinel. A recorded host invocation fails the generation with
// *UnpinnedProbeError; an adapter error fails it with *GenerationError.
func Generate(ctx context.Context, set *Set, factory AdapterFactory) (*Generation, error) {
	if factory == nil {
		factory = PinnedAdapters
	}
	workDir, err := os.MkdirTemp("", "harneval-surface-")
	if err != nil {
		return nil, fmt.Errorf("surface directory: %w", err)
	}
	generation := &Generation{Surfaces: map[string]*Surface{}, workDir: workDir}
	variants := variantOverrides(set)
	keys := make([]string, 0, len(variants))
	for key := range variants {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	invocations, err := withSentinel(func() error {
		for index, key := range keys {
			root := filepath.Join(workDir, "variant-"+strconv.Itoa(index))
			surface, err := generateSurface(ctx, root, set, variants[key], factory)
			if err != nil {
				err.Variant = key
				return err
			}
			generation.Surfaces[key] = surface
		}
		return nil
	})
	generation.Invocations = invocations
	if len(invocations) > 0 {
		_ = generation.Close()
		return nil, &UnpinnedProbeError{Binaries: invokedBinaries(invocations), Invocations: invocations}
	}
	if err != nil {
		_ = generation.Close()
		return nil, err
	}
	return generation, nil
}

func generateSurface(
	ctx context.Context, root string, set *Set, overrides map[string]bool, factory AdapterFactory,
) (*Surface, *GenerationError) {
	pins := set.Manifest.Pins
	cfg := config.DefaultFullConfig(pins.ProjectName)
	cfg.Platforms = append([]string(nil), Platforms...)
	if value, ok := overrides[OverridePreCommitArch]; ok {
		cfg.Hooks.PreCommitArch = value
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, &GenerationError{Platform: "config", Err: err}
	}
	if err := config.Save(root, cfg); err != nil {
		return nil, &GenerationError{Platform: "config", Err: err}
	}
	surface := &Surface{Root: root, Owned: map[string]map[string]bool{}}
	for _, generator := range factory(root, pins, set.CodexCatalog) {
		files, err := generator.Generate(ctx, cfg)
		if err != nil {
			return nil, &GenerationError{Platform: generator.Name(), Err: err}
		}
		owned := map[string]bool{}
		if files != nil {
			for _, file := range files.Files {
				owned[filepath.ToSlash(file.TargetPath)] = true
			}
		}
		surface.Owned[generator.Name()] = owned
	}
	return surface, nil
}

// variantOverrides maps each variant key in use by an active surface task to
// its overrides. The default config is always present.
func variantOverrides(set *Set) map[string]map[string]bool {
	variants := map[string]map[string]bool{"": {}}
	for _, task := range set.Tasks {
		if task.Kind != KindSurface || task.Status.State != StateActive {
			continue
		}
		for _, variant := range task.Variants {
			variants[variantKey(variant.Overrides)] = variant.Overrides
		}
	}
	return variants
}

// variantKey renders overrides canonically as sorted "key=value" pairs.
func variantKey(overrides map[string]bool) string {
	pairs := make([]string, 0, len(overrides))
	for key, value := range overrides {
		pairs = append(pairs, key+"="+strconv.FormatBool(value))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}
