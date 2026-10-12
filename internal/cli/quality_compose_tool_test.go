package cli

// SPEC-MODELUX-002 T4 (REQ-MUX2-07, REQ-MUX2-16; acceptance S5, S6): a
// Claude Code or Codex tab edit is planned, previewed, and written by the
// Per-Tool Binding Contract through the existing per-tool writer, and the
// same run generates exactly its after-table.

import (
	"bytes"
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// newComposeToolRun is a real file session over a fixture written to dir
// whose table was last drawn with CAT-A.
func newComposeToolRun(t *testing.T, dir string, writer composeSessionWriter) *composeSessionRun {
	t.Helper()
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", writer, nil)
	session.shared.snapshot = composeTableSnapshot(t, "CAT-A")
	return session.shared
}

// composeS5Edit is S5's Codex tab edit: 설계·판단 on opus.
func composeS5Edit(start map[string]string) map[string]string {
	edited := maps.Clone(start)
	for _, agent := range composeRoleGroups[0].Agents {
		edited[agent] = "opus"
	}
	return edited
}

const composeToolBlockNotice = "quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)"

// S5: the change lines of a Codex tab edit over D, then the write: only
// quality.presets.codex-custom and quality.providers.codex change, every
// byte outside the quality block stays, and the block decodes strictly.
func TestComposeToolPlanner_CodexEditWritesTheProviderBinding(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")
	run := newComposeToolRun(t, dir, composeSessionFile)
	planner := composeToolPlanner{run: run, tool: composeToolNamed(t, "codex")}
	before := readBlockFixture(t, dir)

	plan, err := planner.PlanEdit(composeS5Edit(planner.Start()))

	require.NoError(t, err)
	assert.Equal(t, []string{
		"quality.presets.codex-custom 새로 씀 (16개 에이전트)",
		"quality.providers.codex 없음 → codex-custom",
		"codex-custom은 balanced 표준 배치를 받지 않음",
		composeToolBlockNotice,
		"quality.providers는 v0.50.88보다 오래된 auto가 읽지 않습니다",
	}, plan.Changes)
	assert.Equal(t, "codex-custom", plan.Candidate.Quality.Providers["codex"])
	assert.Equal(t, before, readBlockFixture(t, dir), "planning writes nothing")

	require.NoError(t, plan.Apply())

	after := readBlockFixture(t, dir)
	beforeHead, beforeTail := qualityBlockOuter(t, before)
	afterHead, afterTail := qualityBlockOuter(t, after)
	assert.Equal(t, beforeHead, afterHead)
	assert.Equal(t, beforeTail, afterTail)
	written := rawQualityOf(t, after)
	assert.Equal(t, "balanced", written.Default)
	assert.Equal(t, map[string]string{"codex": "codex-custom"}, written.Providers)
	preset := written.Presets["codex-custom"]
	assert.Equal(t, "Codex custom agent tiers", preset.Description)
	want := map[string]string{"architect": "opus", "planner": "opus", "security-auditor": "opus", "spec-writer": "opus",
		"debugger": "fable", "deep-worker": "fable", "reviewer": "fable"}
	for _, agent := range []string{"annotator", "devops", "executor", "explorer", "frontend-specialist",
		"perf-engineer", "tester", "ux-validator", "validator"} {
		want[agent] = "sonnet"
	}
	assert.Equal(t, want, preset.Agents)
	assert.Equal(t, written, run.cfg.Quality, "the loaded config follows the written quality")
	assertStrictQualityBlock(t, after)
}

// The same run's --apply generates the after-table: only the four edited
// Codex agents move, and Claude Code keeps its standard placement (S5).
func TestComposeToolPlanner_AppliedCodexEditGeneratesItsTable(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")
	run := newComposeToolRun(t, dir, composeSessionFile)
	planner := composeToolPlanner{run: run, tool: composeToolNamed(t, "codex")}
	plan, err := planner.PlanEdit(composeS5Edit(planner.Start()))
	require.NoError(t, err)
	require.NoError(t, plan.Apply())

	cmd, _ := newComposeTestCommand()
	require.NoError(t, applyQualityHarnessWith(cmd, dir, run.cfg, "auto quality --apply", run.snapshot))

	assert.Equal(t, config.CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"}, readComposeCodexProfile(t, dir, "planner"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "max"}, readComposeCodexProfile(t, dir, "debugger"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, readComposeCodexProfile(t, dir, "executor"))
	assert.Equal(t, composeTableClaudeCell{model: "claude-fable-5-1", effort: "max"},
		readComposeTableClaudeProfile(t, dir, "planner"))
}

// An edit equal to the start writes nothing and asks nothing (REQ-MUX2-07).
func TestComposeToolPlanner_EditEqualToStartIsNoChange(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")
	run := newComposeToolRun(t, dir, composeSessionFile)
	planner := composeToolPlanner{run: run, tool: composeToolNamed(t, "claude-code")}

	plan, err := planner.PlanEdit(planner.Start())

	require.NoError(t, err)
	assert.True(t, plan.NoChange)
	assert.Nil(t, plan.Apply)
}

// A preset the tool is bound to is rewritten, one it is not bound to is
// overwritten, and a binding that stays gets no providers line.
func TestComposeToolPlanner_PresetKindAndProviderLines(t *testing.T) {
	bound := t.TempDir()
	cfg := composeFixtureConfig("claude-code", "codex")
	cfg.Quality.Presets["codex-custom"] = config.QualityPreset{Description: "codex", Agents: map[string]string{"planner": "haiku"}}
	cfg.Quality.Providers = map[string]string{"codex": "codex-custom"}
	require.NoError(t, config.Save(bound, cfg))
	unbound := t.TempDir()
	delete(cfg.Quality.Providers, "codex")
	require.NoError(t, config.Save(unbound, cfg))

	plan := func(dir string) []string {
		planner := composeToolPlanner{run: newComposeToolRun(t, dir, composeSessionFile), tool: composeToolNamed(t, "codex")}
		result, err := planner.PlanEdit(composeS5Edit(planner.Start()))
		require.NoError(t, err)
		return result.Changes
	}

	boundLines := plan(bound)
	assert.Equal(t, "quality.presets.codex-custom 고쳐 씀 (16개 에이전트)", boundLines[0])
	assert.NotContains(t, boundLines, "quality.providers.codex codex-custom → codex-custom")
	assert.NotContains(t, boundLines, "codex-custom은 balanced 표준 배치를 받지 않음")
	unboundLines := plan(unbound)
	assert.Equal(t, "quality.presets.codex-custom 덮어씀 (16개 에이전트)", unboundLines[0])
	assert.Contains(t, unboundLines, "quality.providers.codex 없음 → codex-custom")
}

// init's memory writer sets the same two keys on its config and writes no
// file; init's own save writes them.
func TestComposeToolPlanner_MemoryWriterSetsTheInitConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := composeFixtureConfig("claude-code", "codex")
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionMemory, nil)
	session.shared.snapshot = composeTableSnapshot(t, "CAT-A")
	planner := composeToolPlanner{run: session.shared, tool: composeToolNamed(t, "claude-code")}
	edited := planner.Start()
	edited["executor"] = "opus"

	plan, err := planner.PlanEdit(edited)
	require.NoError(t, err)
	assert.NotContains(t, plan.Changes, composeToolBlockNotice, "no file is rewritten")
	require.NoError(t, plan.Apply())

	assert.Equal(t, "claude-custom", cfg.Quality.Providers["claude"])
	assert.Equal(t, config.QualityPreset{Description: "Claude Code custom agent tiers", Agents: edited},
		cfg.Quality.Presets["claude-custom"])
	assert.NoFileExists(t, filepath.Join(dir, "autopus.yaml"))
}

func TestComposeToolPlanner_HasNoRefresh(t *testing.T) {
	planner := composeToolPlanner{run: &composeSessionRun{}, tool: composeToolNamed(t, "codex")}
	_, err := planner.PlanRefresh()
	assert.Error(t, err)
}

// assertStrictQualityBlock decodes the quality block of data into the
// current QualityConf with unknown keys refused: no new YAML key.
func assertStrictQualityBlock(t *testing.T, data string) {
	t.Helper()
	var doc struct {
		Quality yaml.Node `yaml:"quality"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(data), &doc))
	block, err := yaml.Marshal(&doc.Quality)
	require.NoError(t, err)
	decoder := yaml.NewDecoder(bytes.NewReader(block))
	decoder.KnownFields(true)
	var quality config.QualityConf
	require.NoError(t, decoder.Decode(&quality))
}
