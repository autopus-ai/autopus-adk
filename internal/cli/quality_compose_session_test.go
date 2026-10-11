package cli

// SPEC-MODELUX-001 REQ-MUX-17: the composition session one command run builds
// with the real table, group editor, and binding planner. Only the catalog
// probe is stubbed; the driver, the table, the planner, and the quality
// writers run for real against files in a temp project.

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeSessionCatalog stubs the run's catalog probe with one acceptance
// catalog fixture; CAT-C is a failed probe.
func composeSessionCatalog(t *testing.T, catalog string) *[]composeProbeCall {
	t.Helper()
	if catalog == "CAT-C" {
		return stubComposeCatalogProbe(t, "", errors.New("codex: not signed in"))
	}
	data, err := os.ReadFile(filepath.Join(composeTableTestdata, strings.ToLower(catalog)+".json"))
	require.NoError(t, err)
	return stubComposeCatalogProbe(t, string(data), nil)
}

// composeSessionProject writes acceptance fixture D (the saved code defaults
// with Claude Code and Codex, supervisor inherit) or R (this repository's
// autopus.yaml copy) into a temp project and loads it the way update does.
func composeSessionProject(t *testing.T, fixture string) (string, *config.HarnessConfig) {
	t.Helper()
	dir := t.TempDir()
	switch fixture {
	case "D":
		cfg := config.DefaultFullConfig("p")
		cfg.Platforms = []string{"claude-code", "codex"}
		cfg.Quality.SupervisorModelPolicy = config.SupervisorModelPolicyInherit
		require.NoError(t, config.Save(dir, cfg))
	case "R":
		data, err := os.ReadFile(filepath.Join(composeTableTestdata, "R.yaml"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), data, 0o644))
	default:
		t.Fatalf("unknown compose session fixture %q", fixture)
	}
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	return dir, cfg
}

func composeSessionSources(t *testing.T) map[string]composeAgentSource {
	t.Helper()
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	return sources
}

func composeSessionHash(t *testing.T, dir string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	return sha256.Sum256(data)
}

func composeSessionGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(composeTableTestdata, name))
	require.NoError(t, err)
	return string(data)
}

// runComposeSession runs one session over input the way an entry point does.
func runComposeSession(t *testing.T, session composeDriverSession, input string) (composeDriverResult, string, error) {
	t.Helper()
	cmd, out := newDriverCommand(strings.NewReader(input))
	cmd.SetContext(context.Background())
	result, err := session.run(cmd)
	return result, out.String(), err
}

// S6 through the session: the real group editor shows S2's displays, the
// preview is the pinned D→K after-table read with the run's snapshot and its
// legend appears once, and `y` binds custom through the block writer and
// syncs the loaded config.
func TestNewComposeSession_FileWriterPreviewsAndWritesTheGroupEdit(t *testing.T) {
	dir, cfg := composeSessionProject(t, "D")
	probes := composeSessionCatalog(t, "CAT-A")
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)

	result, out, err := runComposeSession(t, session, "g\n\n3\n\n4\ny\n")

	require.NoError(t, err)
	require.Equal(t, composeDriverApplied, result.Outcome)
	assert.Len(t, *probes, 1)
	assert.Contains(t, out, "quality.default = balanced   ultra: 현재 기본값   balanced: 현재 기본값\n"+
		composeSessionGolden(t, "table-D-CAT-A.golden")+"구성을 바꿀까요?")
	for _, display := range []string{
		"설계·판단  planner, architect, spec-writer, security-auditor  현재: fable\n",
		"구현  executor, debugger, deep-worker, devops, frontend-specialist, perf-engineer  현재: 혼합 (fable 2 · sonnet 4)\n",
		"검증·리뷰  reviewer, validator, tester, ux-validator  현재: 혼합 (fable 1 · sonnet 3)\n",
		"탐색·보조  explorer, annotator  현재: sonnet\n",
	} {
		assert.Contains(t, out, display)
	}
	assert.Contains(t, out, "미리보기 (아직 바뀌지 않음)\n"+composeSessionGolden(t, "preview-D-K-CAT-A.golden")+
		"바뀌는 설정:\n"+
		"  quality.presets.custom 새로 씀 (16개 에이전트)\n"+
		"  quality.default balanced → custom\n"+
		"  custom은 balanced 표준 배치를 받지 않음\n"+
		"  quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)\n"+
		"적용할까요? [y/N] · e) 계속 편집\n")
	assert.Equal(t, 1, strings.Count(out, "* = 바뀌는 칸"), "the preview legend must appear once")

	written := rawQualityOf(t, readBlockFixture(t, dir))
	assert.Equal(t, "custom", written.Default)
	assert.Equal(t, composeTableK, written.Presets["custom"].Agents)
	assert.Equal(t, "역할 그룹 구성", written.Presets["custom"].Description)
	assert.Empty(t, written.Providers)
	assert.Equal(t, written, cfg.Quality, "the loaded config must hold what was written")
	assert.Equal(t, cfg.Quality, result.Plan.Candidate.Quality)
	assert.Equal(t, composeTableK, result.Plan.Composition)
	assert.Equal(t, []byte(mustReadComposeCatalog(t, "CAT-A")), result.Snapshot.codexCatalog)
}

// The init writer keeps the confirmed plan in memory: the config init saves
// afterwards carries it, no file is touched, and no block-rewrite notice is
// shown because no block is rewritten. The locale reaches every surface.
func TestNewComposeSession_MemoryWriterAppliesToTheConfigOnly(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("p")
	cfg.Platforms = []string{"claude-code", "codex"}
	session := newComposeSession(dir, cfg, composeSessionSources(t), "en", composeSessionMemory, nil)

	result, out, err := runComposeSession(t, session, "g\n\n3\n\n4\ny\n")

	require.NoError(t, err)
	require.Equal(t, composeDriverApplied, result.Outcome)
	assert.Contains(t, out, "Change the composition? [Enter] keep · g) role groups · a) advanced\n")
	assert.Contains(t, out, "Implementation  executor, debugger, deep-worker, devops, frontend-specialist, perf-engineer"+
		"  current: mixed (fable 2 · sonnet 4)\n")
	assert.Contains(t, out, "Settings that change:\n"+
		"  quality.presets.custom written new (16 agents)\n"+
		"  quality.default balanced → custom\n"+
		"  custom does not get the balanced standard placement\n"+
		"Apply? [y/N]")
	assert.NotContains(t, out, "the quality block is rewritten")
	assert.Equal(t, "custom", cfg.Quality.Default)
	assert.Equal(t, composeTableK, cfg.Quality.Presets["custom"].Agents)
	assert.Equal(t, config.DefaultFullConfig("p").Quality.Presets["balanced"], cfg.Quality.Presets["balanced"])
	assert.NoFileExists(t, filepath.Join(dir, "autopus.yaml"))
}

// `u`, then `e` and an advanced edit, still makes one write: the refresh and
// the custom binding planned on top of the refreshed candidate (REQ-MUX-06,
// REQ-MUX-11).
func TestNewComposeSession_EditAfterRefreshKeepsTheRefreshInTheOneWrite(t *testing.T) {
	dir, cfg := composeSessionProject(t, "R")
	composeSessionCatalog(t, "CAT-A")
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)

	result, out, err := runComposeSession(t, session, "u\ne\na\nplanner=sonnet\n\ny\n")

	require.NoError(t, err)
	require.Equal(t, composeDriverApplied, result.Outcome)
	assert.Equal(t, 2, strings.Count(out, "미리보기 (아직 바뀌지 않음)\n"))
	assert.Contains(t, out, "  quality.presets.ultra 최신 기본값으로 갱신: 모델 변화 없음, YAML만 정리\n"+
		"  quality.presets.balanced 최신 기본값으로 갱신: 모델 변화 없음, YAML만 정리\n"+
		"  quality.presets.custom 새로 씀 (16개 에이전트)\n"+
		"  quality.default ultra → custom\n")
	written := rawQualityOf(t, readBlockFixture(t, dir))
	defaults := config.DefaultFullConfig("").Quality.Presets
	assert.Equal(t, defaults["ultra"], written.Presets["ultra"])
	assert.Equal(t, defaults["balanced"], written.Presets["balanced"])
	assert.Equal(t, "custom", written.Default)
	custom := written.Presets["custom"].Agents
	assert.Equal(t, "sonnet", custom["planner"])
	delete(custom, "planner")
	expected := maps16Without(defaults["ultra"].Agents, "planner")
	assert.Equal(t, expected, custom)
}

// The change lines read the snapshot the table drew with: under CAT-A the
// quality-managed Codex default session of R moves from 6-astra · ultra to
// 6-astra · xhigh, which an unread catalog would resolve differently. A
// cancel writes nothing.
func TestNewComposeSession_ChangeLinesReadTheRunSnapshotAndCancelKeeps(t *testing.T) {
	dir, cfg := composeSessionProject(t, "R")
	composeSessionCatalog(t, "CAT-A")
	before := composeSessionHash(t, dir)
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)

	result, out, err := runComposeSession(t, session, "a\nplanner=sonnet\n\nn\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.Contains(t, out, "  quality.default ultra → custom\n"+
		"  Codex 기본 세션: 6-astra · ultra → 6-astra · xhigh\n"+
		"  custom은 ultra effort 사다리를 받지 않음\n")
	assert.Contains(t, out, "취소했습니다. 설정을 바꾸지 않았습니다.\n")
	assert.Equal(t, before, composeSessionHash(t, dir))
	assert.Equal(t, "ultra", cfg.Quality.Default)
}

// An edit equal to the start plans no write: the planner reports it and the
// driver ends with 변경 없음 before any confirmation.
func TestNewComposeSession_UnchangedEditWritesNothing(t *testing.T) {
	dir, cfg := composeSessionProject(t, "R")
	composeSessionCatalog(t, "CAT-A")
	before := composeSessionHash(t, dir)
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)

	result, out, err := runComposeSession(t, session, "g\n\n\n\n\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.Contains(t, out, "변경 없음\n")
	assert.NotContains(t, out, "적용할까요?")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

func mustReadComposeCatalog(t *testing.T, catalog string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(composeTableTestdata, strings.ToLower(catalog)+".json"))
	require.NoError(t, err)
	return string(data)
}

// maps16Without returns a copy of agents without the named agent.
func maps16Without(agents map[string]string, agent string) map[string]string {
	copied := make(map[string]string, len(agents))
	for name, tier := range agents {
		if name != agent {
			copied[name] = tier
		}
	}
	return copied
}
