package adapter_test

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// danglingReferenceBaseline pins the local path references that generated
// surfaces still make to something no install manifest writes. It is a
// ratchet, not an allowance: a reference outside this set fails the test, and
// a reference that has been fixed must be deleted from the set, so the list
// can only shrink.
//
// Every entry names an ADK source file. A consumer repo installs the harness
// without the ADK checkout, so these references resolve only in this dogfood
// workspace. Each entry carries its disposition so the remainder is a worklist
// rather than a permanent debt figure. The three dispositions are:
//
//   - INSTALL — the agent reads the file at runtime, so the manifest must
//     install it;
//   - INLINE — the content is needed but should live in the generated surface,
//     the way the branding formats were folded into the branding rule;
//   - MENTION — the path only explains ADK internals, so the wording is what
//     needs cleaning up.
var danglingReferenceBaseline = map[string]bool{
	// INSTALL — builtin Tier 1 executor stack profiles. content/profiles/executor/
	// really holds frontend/go/python/rust/typescript.md, the pipeline selects one
	// per task, and no adapter installs any of them, so Tier 1 selection is
	// unreachable in a consumer repo.
	"content/profiles/executor/{name}.md":  true,
	"content/profiles/executor/{stack}.md": true,

	// INSTALL or INLINE — PRD and scenario skeletons the plan flow copies to
	// author prd.md and scenarios.md. The agent needs the body, not the path;
	// installing them under a rules/skills surface or inlining the section
	// outlines both close it. Choosing between them is an adapter/manifest
	// decision, not a wording change.
	"templates/shared/prd-minimal.md.tmpl":        true,
	"templates/shared/prd-standard.md.tmpl":       true,
	"templates/shared/prd-{PRD_MODE}.md.tmpl":     true,
	"templates/shared/scenarios-api.md.tmpl":      true,
	"templates/shared/scenarios-cli.md.tmpl":      true,
	"templates/shared/scenarios-frontend.md.tmpl": true,
	"templates/shared/scenarios-library.md.tmpl":  true,

	// MENTION — provenance comment in the generated workflow JS naming its
	// manifest. Nothing reads it at runtime; the header string in
	// pkg/content/workflow_generate.go should name the ADK repo instead of a
	// bare path.
	"content/workflows/route_a.{md":    true,
	"content/workflows/route_team.{md": true,
}

// danglingReferenceOccurrenceCeiling caps total occurrences of baselined
// references so an existing violation cannot silently multiply across files.
// It started at 347 occurrences over 32 distinct references before the
// branding, rule-path, and skill-path sources were fixed.
const danglingReferenceOccurrenceCeiling = 43

// platformGenerators maps an autopus.yaml platform id to its adapter, so a
// caller can build the installed surface for exactly the platforms a scenario
// declares. Comparing a marker section against platforms it was not rendered
// for would report gaps the install never had.
var platformGenerators = map[string]func(root string, ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error){
	"claude-code": func(root string, ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
		return claude.NewWithRoot(root).Generate(ctx, cfg)
	},
	"codex": func(root string, ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
		return codex.NewWithRoot(root).Generate(ctx, cfg)
	},
	"antigravity-cli": func(root string, ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
		return antigravity.NewWithRoot(root).Generate(ctx, cfg)
	},
	"opencode": func(root string, ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
		return opencode.NewWithRoot(root, opencode.WithCLIVersion("1.18.7")).Generate(ctx, cfg)
	},
	"omp": func(root string, ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
		return omp.NewWithRoot(root).Generate(ctx, cfg)
	},
}

var allPlatforms = []string{"claude-code", "codex", "antigravity-cli", "opencode", "omp"}

// generateSurface runs the production Generate path for each named platform and
// unions the resulting target paths.
func generateSurface(t *testing.T, platforms []string) *installedSurface {
	t.Helper()
	ctx := context.Background()
	cfg := config.DefaultFullConfig("reference-integrity")
	cfg.Platforms = platforms

	surface := newInstalledSurface()
	for _, name := range platforms {
		generate, ok := platformGenerators[name]
		require.True(t, ok, "no adapter registered for platform %q", name)
		pf, err := generate(t.TempDir(), ctx, cfg)
		require.NoError(t, err)
		surface.add(pf.Files)
	}
	return surface
}

// A generated surface that names a path the installer never writes sends the
// agent to a file that does not exist in a consumer repo. This repo hides the
// failure because the ADK checkout sits next to the installed surface, so the
// assertion runs against the generated bodies and the installed path union
// rather than against the working tree.
//
// The earlier codex path-canonicalisation test only asserted that the string
// ".codex/rules/autopus/" had disappeared, which a prefix rewrite satisfies
// while leaving "AGENTS.mdbranding.md" behind. Asserting on the resolved
// target instead of the removed substring is what closes that class.
func TestGeneratedSurfaces_ReferenceOnlyInstalledPaths(t *testing.T) {
	t.Parallel()
	surface := generateSurface(t, allPlatforms)

	observed := map[string]map[string]int{}
	for file, body := range surface.files {
		surface.collectDangling(file, body, observed)
	}

	total := 0
	for _, ref := range sortedRefs(observed) {
		count, examples := refSummary(observed[ref])
		total += count
		if !danglingReferenceBaseline[ref] {
			t.Errorf("generated surface references %q, which no install manifest writes"+
				" (%d occurrences, e.g. %s)", ref, count, examples)
		}
	}

	stale := make([]string, 0, len(danglingReferenceBaseline))
	for ref := range danglingReferenceBaseline {
		if observed[ref] == nil {
			stale = append(stale, ref)
		}
	}
	t.Logf("baselined dangling references: %d distinct, %d occurrences (ceiling %d)",
		len(observed), total, danglingReferenceOccurrenceCeiling)
	sort.Strings(stale)
	for _, ref := range stale {
		t.Errorf("baseline entry %q no longer occurs; delete it from danglingReferenceBaseline"+
			" so the ratchet cannot loosen again", ref)
	}

	if total > danglingReferenceOccurrenceCeiling {
		t.Errorf("dangling reference occurrences %d exceed ceiling %d;"+
			" lower the ceiling when you fix references, never raise it",
			total, danglingReferenceOccurrenceCeiling)
	}
}

// A detector whose negative answer is indistinguishable from "nothing to
// report" is not a gate: a regex or classifier edit could silently neuter the
// scan while every assertion above keeps passing. These cases pin that the
// scanner still reports the two shapes the reported defect actually had, and
// that it still accepts a real installed path.
func TestDanglingScanner_ReportsKnownBadShapes(t *testing.T) {
	t.Parallel()
	surface := newInstalledSurface()
	surface.add([]adapter.FileMapping{
		{TargetPath: "AGENTS.md", Content: []byte("# Root\n\n## Autopus Branding\n")},
		{TargetPath: ".claude/rules/autopus/branding.md", Content: []byte("# Branding\n")},
	})

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"prefix rewrite leftover", "see `AGENTS.mdbranding.md` now", "AGENTS.mdbranding.md"},
		{"adk source path", "see `templates/shared/branding-formats.md.tmpl`", "templates/shared/branding-formats.md.tmpl"},
		{"uninstalled surface path", "see `.codex/rules/autopus/branding.md`", ".codex/rules/autopus/branding.md"},
		{"anchor with no heading", "see `AGENTS.md#document-storage`", "AGENTS.md#document-storage"},
	} {
		observed := map[string]map[string]int{}
		surface.collectDangling("probe.md", tc.body, observed)
		assert.Contains(t, observed, tc.want, tc.name)
	}

	clean := map[string]map[string]int{}
	surface.collectDangling("probe.md",
		"see `.claude/rules/autopus/branding.md` and `AGENTS.md#autopus-branding` and ~/.claude/projects/",
		clean)
	assert.Empty(t, clean, "installed paths, resolvable anchors, and home state must not be reported")
}
