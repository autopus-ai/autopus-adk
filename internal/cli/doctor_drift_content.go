package cli

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/codexruntime"
	"github.com/insajin/autopus-adk/pkg/config"
)

// maxDriftReprPaths caps the representative drift paths surfaced per platform so
// the advisory output stays compact even when many files are stale (REQ-001).
const maxDriftReprPaths = 3

// contentDriftResult is one platform's installed-surface content-drift verdict.
// Compared counts the deterministic files that were checked; DriftCount is the
// number that differ from the current binary's generation; DriftPaths holds up
// to maxDriftReprPaths representative target paths for the report.
type contentDriftResult struct {
	Platform   string
	Compared   int
	DriftCount int
	DriftPaths []string
}

const driftCodexCatalogTimeout = 5 * time.Second

// driftGenerationSnapshot freezes external generation inputs shared by the two
// seeded roots. In particular, Codex must use one catalog response for both
// roots or an external catalog change could be mistaken for root-state drift.
type driftGenerationSnapshot struct {
	codexCatalog      []byte
	codexCatalogFixed bool
}

type driftCatalogProbe func(context.Context, string, time.Duration) ([]byte, error)

type driftBaselineGenerator func(
	context.Context,
	string,
	*config.HarnessConfig,
	driftGenerationSnapshot,
	func(string) error,
) (*adapter.PlatformFiles, bool)

type driftContentDeps struct {
	probeCatalog     driftCatalogProbe
	generateBaseline driftBaselineGenerator
}

func defaultDriftContentDeps() driftContentDeps {
	return driftContentDeps{
		probeCatalog:     codexruntime.ProbeModelCatalog,
		generateBaseline: generateDriftBaseline,
	}
}

func captureDriftGenerationSnapshot(
	ctx context.Context,
	platform string,
	probe driftCatalogProbe,
) driftGenerationSnapshot {
	descriptor, ok := lookupPlatformDescriptor(platform)
	if !ok || !descriptor.probeDriftCatalog {
		return driftGenerationSnapshot{}
	}
	catalog, err := probe(ctx, descriptor.name, driftCodexCatalogTimeout)
	if err != nil {
		catalog = nil
	}
	return driftGenerationSnapshot{codexCatalog: catalog, codexCatalogFixed: true}
}

// driftAdapterFor returns the catalog adapter rooted at root, or nil for an
// unknown platform token. External inputs are frozen for deterministic drift.
func driftAdapterFor(platform, root string, snapshot driftGenerationSnapshot) adapter.PlatformAdapter {
	descriptor, ok := lookupPlatformDescriptor(platform)
	if !ok {
		return nil
	}
	return descriptor.driftAdapter(root, snapshot.codexCatalog, snapshot.codexCatalogFixed)
}

// collectContentDrift compares each configured platform's installed deterministic
// surface against what the current binary would generate. Only platforms with an
// installed manifest are checked; others are silently skipped (REQ-008). The
// installed surface and real manifests are never mutated — generation happens in
// isolated temp roots.
func collectContentDrift(dir string, cfg *config.HarnessConfig) []contentDriftResult {
	return collectContentDriftContext(context.Background(), dir, cfg)
}

// collectContentDriftContext is the context-aware implementation. The wrapper
// preserves existing doctor call sites while generation and catalog probes share
// one cancellation boundary all the way down to platform adapters.
func collectContentDriftContext(ctx context.Context, dir string, cfg *config.HarnessConfig) []contentDriftResult {
	return collectContentDriftWithDeps(ctx, dir, cfg, defaultDriftContentDeps())
}

func collectContentDriftWithDeps(
	ctx context.Context,
	dir string,
	cfg *config.HarnessConfig,
	deps driftContentDeps,
) []contentDriftResult {
	if cfg == nil {
		return nil
	}
	var results []contentDriftResult
	snapshots := make(map[string]driftGenerationSnapshot)
	for _, platform := range cfg.Platforms {
		// Manifest presence is the "installed" gate. A read error or absent
		// manifest means this platform is not installed here — skip quietly.
		m, err := adapter.LoadManifest(dir, platform)
		if err != nil || m == nil {
			continue
		}
		snapshot, exists := snapshots[platform]
		if !exists {
			snapshot = captureDriftGenerationSnapshot(ctx, platform, deps.probeCatalog)
			snapshots[platform] = snapshot
		}
		res, ok := computeContentDrift(ctx, dir, platform, cfg, snapshot, deps.generateBaseline)
		if !ok {
			continue
		}
		results = append(results, res)
	}
	return results
}

// computeContentDrift derives the deterministic template+cfg subset for a
// platform and hashes each installed file against the freshly generated content.
// It returns ok=false when generation into a temp root fails, so a broken
// platform degrades to a silent skip rather than a false drift signal.
func computeContentDrift(
	ctx context.Context,
	dir, platform string,
	cfg *config.HarnessConfig,
	snapshot driftGenerationSnapshot,
	generateBaseline driftBaselineGenerator,
) (contentDriftResult, bool) {
	// Two isolated seeded temp roots isolate the pure template+cfg surface from
	// root-path and pre-existing-state dependent files (F-002 determinism gate):
	// A is empty, B carries representative user state. Only files byte-identical
	// across A and B are pure functions of (template, cfg) and safe to compare.
	filesA, ok := generateBaseline(ctx, platform, cfg, snapshot, nil)
	if !ok {
		return contentDriftResult{}, false
	}
	filesB, ok := generateBaseline(ctx, platform, cfg, snapshot, seedUserState)
	if !ok {
		return contentDriftResult{}, false
	}

	deterministic := deterministicOverwriteSubset(filesA, filesB)

	res := contentDriftResult{Platform: platform}
	var drifted []string
	for target, wantChecksum := range deterministic {
		installed, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(target)))
		if err != nil {
			// A missing installed file is a tracking concern owned by the git
			// hygiene check, not content staleness. Skip it so the advisory
			// content gate never false-positives on a partial install.
			continue
		}
		res.Compared++
		if adapter.Checksum(string(installed)) != wantChecksum {
			drifted = append(drifted, target)
		}
	}

	sort.Strings(drifted)
	res.DriftCount = len(drifted)
	res.DriftPaths = drifted
	return res, true
}

// generateDriftBaseline runs the platform adapter's Generate against a fresh
// isolated temp root, optionally seeded with representative user state. The
// temp root is removed before returning; the in-memory FileMapping content and
// checksums are all the drift comparison needs.
func generateDriftBaseline(
	ctx context.Context,
	platform string,
	cfg *config.HarnessConfig,
	snapshot driftGenerationSnapshot,
	seed func(string) error,
) (*adapter.PlatformFiles, bool) {
	tmp, err := os.MkdirTemp("", "autopus-drift-")
	if err != nil {
		return nil, false
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if seed != nil {
		if err := seed(tmp); err != nil {
			return nil, false
		}
	}

	ad := driftAdapterFor(platform, tmp, snapshot)
	if ad == nil {
		return nil, false
	}
	pf, err := ad.Generate(ctx, cfg)
	if err != nil || pf == nil {
		return nil, false
	}
	return pf, true
}

// deterministicOverwriteSubset returns the OverwriteAlways target paths whose
// generated content is identical across the two seeded roots, mapped to their
// checksum. Marker and merge files are excluded by policy; state- and path-
// dependent OverwriteAlways files (e.g. statusline-user-command.txt) are
// excluded because their bytes differ between the empty and seeded roots.
func deterministicOverwriteSubset(a, b *adapter.PlatformFiles) map[string]string {
	bChecksum := make(map[string]string, len(b.Files))
	for _, f := range b.Files {
		if f.OverwritePolicy == adapter.OverwriteAlways {
			bChecksum[f.TargetPath] = f.Checksum
		}
	}

	out := make(map[string]string)
	for _, fa := range a.Files {
		if fa.OverwritePolicy != adapter.OverwriteAlways {
			continue
		}
		cb, ok := bChecksum[fa.TargetPath]
		if !ok {
			continue
		}
		if fa.Checksum != cb {
			continue
		}
		out[fa.TargetPath] = fa.Checksum
	}
	return out
}

// seedUserState writes representative pre-existing user state into a temp root
// so the determinism gate can detect state-dependent OverwriteAlways files. The
// merge-mode statusline plus a user command file makes the generated
// statusline-user-command.txt a function of prior state, and a user-authored
// CLAUDE.md body exercises marker-merge state.
func seedUserState(root string) error {
	claudeDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return err
	}
	settings := `{"statusLine":{"command":".claude/statusline-combined.sh"}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "statusline-user-command.txt"), []byte("echo drift-seed\n"), 0o644); err != nil {
		return err
	}
	// The OMP project config holds the user's keys next to the claims ADK
	// merges in, and the ownership ledger commits to them: neither is a pure
	// function of templates and config.
	ompDir := filepath.Join(root, ".omp")
	if err := os.MkdirAll(ompDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(ompDir, "config.yml"), []byte("theme: drift-seed\n"), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# user preface\n"), 0o644)
}
