package cli

// SPEC-MODELUX-001 REQ-MUX-08/09/12, acceptance S1, S6, S7, S13: what a
// binding write leaves in autopus.yaml — custom and other presets, the keys
// it may touch, the detached candidate, and refusals that keep the file.

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestPlanComposeBinding_OtherComposition_WritesCustomAndKeepsOtherPresets(t *testing.T) {
	t.Parallel()
	fast := config.QualityPreset{Description: "fast", Agents: map[string]string{"planner": "sonnet", "executor": "haiku"}}
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["fast"] = fast
		q.Presets["custom"] = config.QualityPreset{Description: "old custom", Agents: map[string]string{"planner": "haiku"}}
		q.Default = "fast"
	})
	start := composeBindStart(t, cfg)
	edited := composeBindEdited(t, start, map[string]string{"planner": "opus"})

	plan := planComposeBinding(cfg, start, edited)

	assert.Equal(t, composeWriterBlock, plan.Writer)
	assert.Equal(t, "custom", plan.Default)
	require.Equal(t, []string{"custom"}, composeBindPresetNames(plan))
	assert.Equal(t, composePresetOverwrite, plan.Presets[0].Kind)
	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, fast, written.Presets["fast"])
	assert.Equal(t, config.QualityPreset{Description: "역할 그룹 구성", Agents: edited.Agents()}, written.Presets["custom"])
	assert.Len(t, written.Presets["custom"].Agents, len(config.CanonicalAgentNames()))
	assert.Equal(t, "custom", written.Default)
}

func TestPlanComposeBinding_BoundCustom_RewritesItWithoutTheDefaultLine(t *testing.T) {
	t.Parallel()
	_, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["custom"] = config.QualityPreset{Agents: map[string]string{"planner": "haiku"}}
		q.Default = "custom"
	})
	start := composeBindStart(t, cfg)

	plan := planComposeBinding(cfg, start, composeBindEdited(t, start, map[string]string{"planner": "opus"}))

	assert.Equal(t, composeWriterBlock, plan.Writer)
	assert.Empty(t, plan.Default)
	require.Equal(t, []string{"custom"}, composeBindPresetNames(plan))
	assert.Equal(t, composePresetRewrite, plan.Presets[0].Kind)
}

func TestPlanComposeBinding_S6_WritesOnlyAllowedKeysAndNoGroupNames(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindSaved(t, nil)
	start := composeBindStart(t, cfg)

	plan := planComposeBinding(cfg, start, composeBindS6(t, start))
	after := composeBindPersist(t, dir, cfg, plan)

	written := rawQualityOf(t, after)
	assert.Equal(t, s6CustomAgents(), written.Presets["custom"].Agents)
	assert.Equal(t, "custom", written.Default)
	assert.Nil(t, written.Providers)

	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(after), &doc))
	_, quality := yamlMappingPair(doc.Content[0], "quality")
	allowed := map[string]bool{"default": true, "supervisor_model_policy": true}
	for _, path := range composeBindKeyPaths(quality, "") {
		parts := strings.Split(path, ".")
		ok := allowed[path] || len(parts) >= 3 && parts[0] == "presets" &&
			(parts[2] == "description" && len(parts) == 3 || parts[2] == "agents" && len(parts) == 4)
		assert.True(t, ok, "unexpected quality key %s", path)
		if len(parts) == 4 {
			assert.Contains(t, config.CanonicalAgentNames(), parts[3])
		}
	}
	// No group label is written as a key or a value anywhere in the file.
	for _, group := range composeRoleGroups {
		for _, label := range []string{group.Label("ko"), group.Label("en")} {
			assert.NotContains(t, composeBindScalars(doc.Content[0]), label)
		}
	}
}

func composeBindKeyPaths(node *yaml.Node, prefix string) []string {
	if node == nil || node.Kind != yaml.MappingNode {
		return []string{prefix}
	}
	var paths []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if prefix != "" {
			key = prefix + "." + key
		}
		paths = append(paths, composeBindKeyPaths(node.Content[i+1], key)...)
	}
	return paths
}

func composeBindScalars(node *yaml.Node) []string {
	if node.Kind == yaml.ScalarNode {
		return []string{node.Value}
	}
	var scalars []string
	for _, child := range node.Content {
		scalars = append(scalars, composeBindScalars(child)...)
	}
	return scalars
}

func TestPlanComposeBinding_ProviderOverride_StaysUntouched(t *testing.T) {
	t.Parallel()
	override := config.QualityPreset{Agents: map[string]string{}}
	for _, agent := range config.CanonicalAgentNames() {
		override.Agents[agent] = "haiku"
	}
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["codex-custom"] = override
		q.Providers = map[string]string{config.QualityProviderCodex: "codex-custom"}
	})
	start := composeBindStart(t, cfg)

	plan := planComposeBinding(cfg, start, composeBindS6(t, start))

	assert.Equal(t, map[string]string{"codex": "codex-custom"}, plan.Candidate.Quality.Providers)
	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, map[string]string{"codex": "codex-custom"}, written.Providers)
	assert.Equal(t, override, written.Presets["codex-custom"])
}

func TestPlanComposeBinding_Candidate_IsDetachedAndCarriesTheWrite(t *testing.T) {
	t.Parallel()
	_, cfg := composeBindSaved(t, nil)
	start := composeBindStart(t, cfg)
	edited := composeBindS6(t, start)

	plan := planComposeBinding(cfg, start, edited)

	require.NotNil(t, plan.Candidate)
	assert.Equal(t, "custom", plan.Candidate.Quality.Default)
	assert.Equal(t, s6CustomAgents(), plan.Candidate.Quality.Presets["custom"].Agents)
	assert.Equal(t, "balanced", cfg.Quality.Default, "planning never touches the loaded config")
	assert.NotContains(t, cfg.Quality.Presets, "custom")
	plan.Candidate.Quality.Presets["balanced"].Agents["planner"] = "haiku"
	assert.Equal(t, "fable", cfg.Quality.Presets["balanced"].Agents["planner"])
	// The candidate resolves the edited composition, which is what the preview shows.
	assert.True(t, newQualityComposition(plan.Candidate.Quality, composeGroupsSources(t)).Equal(edited))
}

func TestPlanComposeBinding_FileWithoutQuality_AppendsTheBlock(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindWritten(t, blockAbsentPrefix)
	start := composeBindStart(t, cfg)
	plan := planComposeBinding(cfg, start, composeBindS6(t, start))

	lines := composeBindDescribe(t, dir, cfg, plan, "ko")
	after := composeBindPersist(t, dir, cfg, plan)

	assert.Contains(t, lines, "quality.default (없음) → custom")
	assert.True(t, strings.HasPrefix(after, blockAbsentPrefix))
	written := rawQualityOf(t, after)
	assert.Equal(t, "custom", written.Default)
	assert.Equal(t, s6CustomAgents(), written.Presets["custom"].Agents)
	_, err := config.Load(dir)
	assert.NoError(t, err)
}

func TestPersistComposeBinding_PresetChangedSincePlanning_RefusesAndKeepsTheFile(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindWritten(t, composeBindRepo)
	plan, ok := planComposeRefresh(cfg)
	require.True(t, ok)
	// The operator hand-edits ultra after the preview: it is customized now.
	edited := strings.Replace(composeBindRepo, "                devops: opus\n", "                devops: sonnet\n", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(edited), 0o644))

	ultraBefore := maps.Clone(cfg.Quality.Presets["ultra"].Agents)

	err := persistComposeBinding(dir, cfg, plan)

	require.ErrorContains(t, err, "quality.presets.ultra")
	assert.Equal(t, edited, readBlockFixture(t, dir))
	assert.Equal(t, ultraBefore, cfg.Quality.Presets["ultra"].Agents, "a refused write leaves cfg alone")
	assert.Equal(t, "ultra", cfg.Quality.Default)
}

func TestPersistComposeBinding_RefusedSource_SurfacesTheRefusalAndKeepsTheFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		writer composeWriter
		edits  map[string]string
		// stage makes the loaded config classify ultra as current, so the
		// plan takes the line writer; the file itself still holds F3.
		stage bool
	}{
		{name: "block", writer: composeWriterBlock, edits: map[string]string{"planner": "haiku"}},
		{name: "line", writer: composeWriterLine, edits: composeBindCurrent("ultra").Agents, stage: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, cfg := composeBindWritten(t, blockInnerAlias)
			start := composeBindStart(t, cfg)
			if tt.stage {
				cfg.Quality.Presets["ultra"] = composeBindCurrent("ultra")
			}
			plan := planComposeBinding(cfg, start, composeBindEdited(t, start, tt.edits))
			require.Equal(t, tt.writer, plan.Writer)

			err := persistComposeBinding(dir, cfg, plan)

			assert.True(t, errors.Is(err, errQualityWriteRefused), "%v", err)
			assert.Equal(t, blockInnerAlias, readBlockFixture(t, dir))
			assert.Equal(t, "balanced", cfg.Quality.Default)
		})
	}
}
