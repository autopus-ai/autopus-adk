package harneval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// testCatalog advertises every model the default config places, so codex
// resolves without a fallback.
const testCatalog = `{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"medium"},{"effort":"high"}]},{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"max"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`

const archOff = OverridePreCommitArch + "=false"

// pinnedSet loads the standard fixture with a codex catalog pin, after mutate.
func pinnedSet(t *testing.T, mutate func(f *fixture)) (*fixture, *Set) {
	t.Helper()
	f := newFixture(t)
	f.standard()
	f.write("evals/harness/fixtures/codex-models.json", testCatalog)
	manifest := validManifest()
	manifest["pins"].(map[string]any)["codex_model_catalog"] = "evals/harness/fixtures/codex-models.json"
	f.writeJSON(ManifestPath, manifest)
	if mutate != nil {
		mutate(f)
	}
	set, err := LoadSet(f.root)
	require.NoError(t, err)
	return f, set
}

func withArchOffVariant(f *fixture) {
	task := surfaceTask("GT-FIX-B")
	task["variants"] = []any{map[string]any{"name": "arch-off", "overrides": map[string]any{OverridePreCommitArch: false}}}
	f.writeJSON(surfacePath("GT-FIX-B"), task)
}

func readSurface(t *testing.T, surface *Surface, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(surface.Root, filepath.FromSlash(rel)))
	require.NoError(t, err, rel)
	return string(data)
}

// TestGenerate_PinnedSurfaces_AreHermeticPerVariant covers the S2 in-process
// half: the pinned surface does not depend on the root, HOME, or repetition,
// the pinned generator version reaches codex, and no sentinel fires.
func TestGenerate_PinnedSurfaces_AreHermeticPerVariant(t *testing.T) {
	t.Parallel()
	_, set := pinnedSet(t, withArchOffVariant)

	first, err := Generate(context.Background(), set, nil)
	require.NoError(t, err)
	defer func() { _ = first.Close() }()
	second, err := Generate(context.Background(), set, nil)
	require.NoError(t, err)
	defer func() { _ = second.Close() }()

	assert.Empty(t, first.Invocations)
	assert.Equal(t, []string{"", archOff}, first.VariantKeys())
	for _, key := range first.VariantKeys() {
		a, err := SurfaceDigest(first.Surfaces[key].Root)
		require.NoError(t, err)
		b, err := SurfaceDigest(second.Surfaces[key].Root)
		require.NoError(t, err)
		assert.Regexp(t, `^[0-9a-f]{64}$`, a)
		assert.Equal(t, a, b, "variant %q is repeatable", key)
		assert.NotEqual(t, first.Surfaces[key].Root, second.Surfaces[key].Root)
	}
	defaultSurface := first.Surfaces[""]
	for _, platform := range Platforms {
		assert.NotEmpty(t, defaultSurface.Owned[platform], platform)
	}
	assert.True(t, defaultSurface.Owned["claude-code"][".claude/settings.json"])
	assert.False(t, defaultSurface.Owned["codex"][".claude/settings.json"])

	var plugin struct {
		Version string `json:"version"`
	}
	require.NoError(t, json.Unmarshal([]byte(readSurface(t, defaultSurface, ".autopus/plugins/auto/.codex-plugin/plugin.json")), &plugin))
	assert.Regexp(t, `^0\.50\.123\+codex\.harneval-fixture\.[0-9a-f]{12}$`, plugin.Version)

	// The variant really changes the config: the arch hook exists only by default.
	assert.Contains(t, readSurface(t, defaultSurface, ".claude/settings.json"), "check --hygiene --arch")
	assert.NotContains(t, readSurface(t, first.Surfaces[archOff], ".claude/settings.json"), "check --hygiene --arch")

	// Typed assertions hold on the real surface, and the arch variant flips one.
	arch := Assertion{Kind: AssertJSONPathPresent, Platform: "claude-code", Path: ".claude/settings.json",
		JSONPath: "hooks.PreToolUse[*].hooks[*].command", ValueContains: strPtr("check --hygiene --arch")}
	route := Assertion{Kind: AssertRouteDetail, Platform: "claude-code", Path: ".claude/skills/auto/SKILL.md",
		Route: "`plan`", Detail: ".claude/skills/auto-plan/SKILL.md"}
	task := Task{ID: "GT-REAL-SURFACE", Kind: KindSurface, Assertions: []Assertion{arch, route}}
	assert.True(t, EvaluateTask(task, first).Passed, "%v", EvaluateTask(task, first).Failures)
	task.Variants = []Variant{{Name: "arch-off", Overrides: map[string]bool{OverridePreCommitArch: false}}}
	assert.Equal(t, []AssertionFailure{{Variant: "arch-off", Index: 0, Detail: "json_path_absent"}},
		EvaluateTask(task, first).Failures)
}

// TestGenerate_UnpinnedCodex_IsHostProbeUnpinned is the S2 negative: a codex
// adapter without its pins probes the host, the sentinel records it, and the
// generation is refused with the binary name.
func TestGenerate_UnpinnedCodex_IsHostProbeUnpinned(t *testing.T) {
	t.Parallel()
	_, set := pinnedSet(t, nil)
	unpinned := func(root string, _ Pins, _ []byte) []adapter.PlatformAdapter {
		return []adapter.PlatformAdapter{codex.NewWithRoot(root)}
	}

	generation, err := Generate(context.Background(), set, unpinned)

	assert.Nil(t, generation)
	var probe *UnpinnedProbeError
	require.ErrorAs(t, err, &probe)
	assert.Equal(t, []string{"codex"}, probe.Binaries)
	assert.Contains(t, probe.Invocations, "codex debug models")
}

type refusingAdapter struct{ adapter.PlatformAdapter }

func (refusingAdapter) Name() string { return "codex" }
func (refusingAdapter) Generate(context.Context, *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	return nil, errors.New("placement refused")
}

func TestGenerate_AdapterFailure_IsGenerationError(t *testing.T) {
	t.Parallel()
	_, set := pinnedSet(t, withArchOffVariant)
	refusing := func(string, Pins, []byte) []adapter.PlatformAdapter {
		return []adapter.PlatformAdapter{refusingAdapter{}}
	}

	generation, err := Generate(context.Background(), set, refusing)

	assert.Nil(t, generation)
	var failure *GenerationError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "codex", failure.Platform)
	assert.Equal(t, "", failure.Variant, "the default surface is generated first")
	assert.Equal(t, "codex", failure.Detail())
}

func TestVariantKey_IsCanonical(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", variantKey(nil))
	assert.Equal(t, "", variantKey(map[string]bool{}))
	assert.Equal(t, archOff, variantKey(map[string]bool{OverridePreCommitArch: false}))
	assert.Equal(t, OverridePreCommitArch+"=true", variantKey(map[string]bool{OverridePreCommitArch: true}))
}
