package adapter_test

import (
	"path/filepath"
	"strings"
	"testing"

	templatefs "github.com/insajin/autopus-adk/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextEngineering_GeneratedSurfacesMatchCanonicalContract(t *testing.T) {
	surfaces := generateContextEngineeringSurfaces(t)
	t.Run("S1 claude thin router", func(t *testing.T) {
		surface := surfaces["claude"]
		router := readContextEngineeringFile(t, surface.root, ".claude/skills/auto/SKILL.md")
		detail := readContextEngineeringFile(t, surface.root, surface.details["go"])
		assert.Equal(t, 1, strings.Count(router, surface.details["go"]))
		assert.Equal(t, 1, strings.Count(detail, "## Context Profile"))
		assert.NoFileExists(t, filepath.Join(surface.root, ".claude", "skills", "autopus", "auto-workflows.md"))
		for _, forbidden := range []string{"Always load every project context document", "unconditional all-document preload"} {
			assert.NotContains(t, router+"\n"+detail, forbidden)
		}
	})

	expectedMatrix := map[string]contextEngineeringMatrix{
		"plan": {required: []string{"architecture", "core", "relevant_spec"},
			optional: []string{"learning", "signature"}, excluded: []string{"canary", "test"}},
		"test": {required: []string{"core", "test"},
			optional: []string{"learning", "signature"}, excluded: []string{"canary"}},
		"canary": {required: []string{"canary", "core"},
			optional: []string{"learning"}, excluded: []string{"signature", "test"}},
		"go": {required: []string{"acceptance", "core", "plan", "resolved_spec"},
			workerOptional: []string{"learning", "signature", "task_declared_extra"}, excluded: []string{"canary", "test"}},
	}
	for _, surface := range surfaces {
		surface := surface
		t.Run("S2 matrix "+surface.name, func(t *testing.T) {
			for command, expected := range expectedMatrix {
				body := readContextEngineeringFile(t, surface.root, surface.details[command])
				actual, err := parseContextEngineeringMatrix(body)
				require.NoError(t, err, "%s %s", surface.name, command)
				assert.Equal(t, expected, actual, "%s %s", surface.name, command)
			}
		})
	}
	t.Run("S2 codex native go mirror", func(t *testing.T) {
		body := readContextEngineeringFile(t, surfaces["codex"].root, ".codex/skills/codex-auto-go/SKILL.md")
		actual, err := parseContextEngineeringMatrix(body)
		require.NoError(t, err)
		assert.Equal(t, expectedMatrix["go"], actual)
	})

	expectedFields := []string{"blockers", "changed_files", "next_required_step", "owned_paths", "verification"}
	for name, source := range map[string][]byte{
		"shared contract": readEmbeddedContextEngineeringFile(t, templatefs.FS, "shared/orchestration-contract.md.tmpl"),
	} {
		name, source := name, source
		t.Run("S3 canonical owner "+name, func(t *testing.T) {
			assert.Equal(t, expectedFields, extractCanonicalWorkerFields(t, string(source)))
		})
	}
	for _, surface := range surfaces {
		surface := surface
		t.Run("S3-S4 route-resolved pipeline "+surface.name, func(t *testing.T) {
			detail := readContextEngineeringFile(t, surface.root, surface.details["go"])
			_, err := resolveContextEngineeringPipeline(surface.root, detail, surface.pipeline)
			require.NoError(t, err)
			assertContextEngineeringGuidance(t, surface.root, surface.pipeline, expectedFields)
		})
	}
	t.Run("S3-S4 stale and missing pipeline refs fail closed", func(t *testing.T) {
		const expected = ".agents/skills/agent-pipeline/SKILL.md"
		_, err := resolveContextEngineeringPipeline(t.TempDir(),
			"Load `.agents/skills/stale-agent-pipeline/SKILL.md`.", expected)
		assert.ErrorContains(t, err, "exact generated pipeline target")
		_, err = resolveContextEngineeringPipeline(t.TempDir(), "Load `"+expected+"`.", expected)
		assert.Error(t, err, "referenced but missing pipeline must fail")
	})
	// The plugin mirror is a copy, not a variant. Comparing the whole
	// normalized body plus the resource a worker actually opens catches any
	// divergence, including ones no clause list would have named.
	t.Run("S4 gemini antigravity normalized parity", func(t *testing.T) {
		native := readContextEngineeringFile(t, surfaces["gemini"].root, surfaces["gemini"].pipeline)
		plugin := readContextEngineeringFile(t, surfaces["antigravity-mirror"].root, surfaces["antigravity-mirror"].pipeline)
		// The mirror rehomes managed paths into the plugin directory and
		// changes nothing else, so undoing that rewrite makes the two bodies
		// directly comparable.
		rehomed := strings.NewReplacer(
			".agents/plugins/autopus/skills/auto/", ".gemini/skills/auto/",
			".agents/plugins/autopus/skills/", ".gemini/skills/autopus/",
			".agents/plugins/autopus/rules/", ".gemini/rules/autopus/",
			".agents/plugins/autopus/agents/", ".gemini/agents/autopus/",
			".agents/plugins/autopus/commands/auto/", ".gemini/commands/auto/",
		).Replace(plugin)
		assert.Equal(t,
			normalizeContextEngineeringProse(native),
			normalizeContextEngineeringProse(rehomed),
			"the plugin mirror must carry the same pipeline body as the native surface")

		nativeCoordination := readContextEngineeringFile(t, surfaces["gemini"].root,
			strings.TrimSuffix(surfaces["gemini"].pipeline, "SKILL.md")+"references/coordination.md")
		pluginCoordination := readContextEngineeringFile(t, surfaces["antigravity-mirror"].root,
			strings.TrimSuffix(surfaces["antigravity-mirror"].pipeline, "SKILL.md")+"references/coordination.md")
		assert.Equal(t,
			normalizeContextEngineeringProse(nativeCoordination),
			normalizeContextEngineeringProse(pluginCoordination),
			"the plugin mirror must carry the same coordination resource")
	})
}
