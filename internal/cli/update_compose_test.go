package cli

// SPEC-MODELUX-001 T10 and probe A2 (acceptance S10): the composition step
// inside the real `auto update` flow. Only the TTY and CI seams, the run's
// catalog probe, and the codex CLI are stand-ins; the update command, the
// driver, the writers, and the platform adapters run for real.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const composeUpdateStatusR = "quality.default = ultra   ultra: 이전 버전 기본값 (갱신 가능)   balanced: 이전 버전 기본값 (갱신 가능)"

// Interactive keep paths ask once, leave autopus.yaml byte-identical, probe
// the catalog once, and generate exactly the table the operator saw. Stale
// generated files on disk never reach the table and are regenerated to it
// (S20).
func TestUpdateCompose_InteractiveKeepPathsLeaveTheConfigByteIdentical(t *testing.T) {
	cases := []struct{ name, ci, input string }{
		{"enter", "", "\n"},
		{"input end", "", ""},
		{"three invalid answers", "", "x\nx\nx\n"},
		{"CI=false stays interactive", "false", "\n"},
		{"CI=0 stays interactive", "0", "\n"},
	}
	base := newComposeUpdateRun(t, true, "")
	settled := base.settled(t, "R")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := base.rerun(t, true, tc.ci)
			dir := cloneComposeProject(t, settled)
			rewriteComposeFile(t, composeGeneratedPath(dir, ".claude", "agents", "autopus", "planner.md"),
				"model: opus", "model: sonnet")
			rewriteComposeLine(t, composeGeneratedPath(dir, ".codex", "agents", "planner.toml"),
				"model = ", `model = "gpt-5.5"`)
			before := composeSessionHash(t, dir)

			out, err := runComposeUpdate(t, strings.NewReader(tc.input), "--dir", dir)

			require.NoError(t, err, out)
			assert.Equal(t, before, composeSessionHash(t, dir))
			assert.NotContains(t, out, "5.5")
			assert.Contains(t, out, "에이전트 모델 구성")
			assert.Contains(t, out, "구성을 바꿀까요? [Enter] 유지 · u) 최신 기본값으로 갱신 · g) 역할 그룹 · a) 고급\n")
			assert.Len(t, *run.probes, 1)
			assert.False(t, run.adapterProbed(t), "generation must read the run's snapshot")
			assert.NotContains(t, out, composeUpdateNotice)
			if tc.input == "x\nx\nx\n" {
				assert.Contains(t, out, "입력이 올바르지 않아 구성을 유지합니다.\n")
			}
			table := composeRenderedTable(t, out, composeUpdateStatusR)
			claudeMarks, codexMarks := assertGeneratedEqualsTable(t, dir, table)
			assert.Zero(t, claudeMarks+codexMarks)
			assert.Equal(t, "opus · max", table["planner"]["Claude Code"])
			assert.Equal(t, "6.1-sol · xhigh", table["planner"]["Codex"])
			for _, column := range []string{"OMP", "OpenCode", "Antigravity"} {
				assert.Equal(t, "플랫폼 기본값", table["planner"][column])
			}
		})
	}
}

// Non-interactive runs never read stdin, never draw the table or probe the
// catalog for it, keep autopus.yaml, and print the notice exactly once.
// --check without --self still generates the platform files.
func TestUpdateCompose_NonInteractiveRunsReadNothingAndNoticeOnce(t *testing.T) {
	cases := []struct {
		name string
		tty  bool
		ci   string
		args []string
	}{
		{"not a terminal", false, "", nil},
		{"--yes", true, "", []string{"--yes"}},
		{"-y", true, "", []string{"-y"}},
		{"--auto", true, "", []string{"--auto"}},
		{"--check", true, "", []string{"--check"}},
		{"CI=true", true, "true", nil},
		{"CI=1", true, "1", nil},
	}
	base := newComposeUpdateRun(t, true, "")
	settled := base.settled(t, "R")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := base.rerun(t, tc.tty, tc.ci)
			dir := cloneComposeProject(t, settled)
			planner := composeGeneratedPath(dir, ".claude", "agents", "autopus", "planner.md")
			rewriteComposeFile(t, planner, "model: opus", "model: sonnet")
			before := composeSessionHash(t, dir)

			out, err := runComposeUpdate(t, driverFailingReader{t}, append([]string{"--dir", dir}, tc.args...)...)

			require.NoError(t, err, out)
			assert.Equal(t, before, composeSessionHash(t, dir))
			assert.Equal(t, 1, strings.Count(out, composeUpdateNotice), out)
			assert.NotContains(t, out, "에이전트 모델 구성")
			assert.Empty(t, *run.probes)
			assert.Equal(t, "opus", readComposeTableClaudeProfile(t, dir, "planner").model,
				"the update must still regenerate the platform files")
		})
	}
}

// A preview prints nothing of the step, and `--self` ends before it.
func TestUpdateCompose_PreviewAndSelfNeverEnterTheStep(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	dir := run.settled(t, "R")
	entries := countingComposeStep(t)
	before := composeSessionHash(t, dir)

	out, err := runComposeUpdate(t, driverFailingReader{t}, "--dir", dir, "--plan")
	require.NoError(t, err, out)
	assert.NotContains(t, out, composeUpdateNotice)
	assert.NotContains(t, out, "에이전트 모델 구성")

	_, _ = runComposeUpdate(t, driverFailingReader{t}, "--dir", dir, "--self", "--check")

	assert.Zero(t, *entries)
	assert.Empty(t, *run.probes)
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// A workspace update passes --yes to every repository, so no repository is
// asked and each prints the notice at most once.
func TestUpdateCompose_WorkspaceYesNeverAsksAndNoticesOncePerRepo(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	root := run.settled(t, "R")
	child := filepath.Join(root, "service")
	data, err := os.ReadFile(filepath.Join(root, "autopus.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(child, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(child, "autopus.yaml"), data, 0o644))
	for _, repo := range []string{root, child} {
		require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	}

	out, err := runComposeUpdate(t, driverFailingReader{t}, "--dir", root, "--workspace", "--yes")

	require.NoError(t, err, out)
	assert.Contains(t, out, "Workspace update complete: 2 repo(s) updated")
	repos := strings.Split(out, "\n[")[1:]
	require.Len(t, repos, 2, out)
	for _, repo := range repos {
		assert.LessOrEqual(t, strings.Count(repo, composeUpdateNotice), 1, repo)
	}
	assert.Equal(t, 2, strings.Count(out, composeUpdateNotice))
	assert.NotContains(t, out, "에이전트 모델 구성")
	assert.Empty(t, *run.probes)
}

// S10's group edit: the confirmed custom binding is written, and the same
// run generates exactly the after-table the operator approved, from the one
// catalog snapshot the table read.
func TestUpdateCompose_GroupEditGeneratesTheApprovedTable(t *testing.T) {
	dir, _ := composeSessionProject(t, "D")
	run := newComposeUpdateRun(t, true, "")

	out, err := runComposeUpdate(t, strings.NewReader("g\n\n3\n\n4\ny\n"), "--dir", dir)

	require.NoError(t, err, out)
	assert.Contains(t, out, "구성을 적용했습니다.\n")
	assert.Len(t, *run.probes, 1)
	assert.False(t, run.adapterProbed(t), "generation must read the run's snapshot")
	written := rawQualityOf(t, readBlockFixture(t, dir))
	assert.Equal(t, "custom", written.Default)
	assert.Equal(t, composeTableK, written.Presets["custom"].Agents)

	claudeMarks, codexMarks := assertGeneratedEqualsTable(t, dir, composeRenderedTable(t, out, "미리보기 (아직 바뀌지 않음)"))
	assert.Equal(t, 16, claudeMarks)
	assert.Equal(t, 4, codexMarks)
	assert.Equal(t, composeTableClaudeCell{model: "sonnet", effort: "medium"}, readComposeTableClaudeProfile(t, dir, "debugger"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, readComposeCodexProfile(t, dir, "debugger"))
}

// A refused write prints one warning with the writer's text, keeps
// autopus.yaml, and the update goes on generating the unchanged config.
func TestUpdateCompose_RefusedWriteWarnsOnceAndUpdateContinues(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	dir := run.settled(t, "D")
	path := filepath.Join(dir, "autopus.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(data), "planner: fable"))
	rewriteComposeFile(t, path, "planner: fable", "planner: &tier fable")
	rewriteComposeFile(t, path, "planner: fable", "planner: *tier")
	before := composeSessionHash(t, dir)

	out, err := runComposeUpdate(t, strings.NewReader("g\n\n3\n\n4\ny\n"), "--dir", dir)

	require.NoError(t, err, out)
	assert.Equal(t, before, composeSessionHash(t, dir))
	assert.Equal(t, 1, strings.Count(out, errQualityWriteRefused.Error()), out)
	assert.Contains(t, out, "  경고: "+errQualityWriteRefused.Error()+"\n")
	assert.NotContains(t, out, "구성을 적용했습니다.")
	assert.Contains(t, out, "Update complete: 2 platform(s) updated")
	assert.Len(t, *run.probes, 1)
	assert.Equal(t, composeTableClaudeCell{model: "claude-fable-5-1", effort: "max"}, readComposeTableClaudeProfile(t, dir, "debugger"))
}

// The supervisor migration saves the whole config after the step, so the
// confirmed composition must already be in the loaded config by then.
func TestUpdateCompose_ConfirmedWriteSurvivesTheSupervisorMigrationSave(t *testing.T) {
	dir := writeLegacySupervisorUpdateFixture(t, true)
	run := newComposeUpdateRun(t, true, "")

	out, err := runComposeUpdate(t, strings.NewReader("g\n\n3\n\n4\ny\n"), "--dir", dir)

	require.NoError(t, err, out)
	assert.Contains(t, out, "구성을 적용했습니다.\n")
	assert.Contains(t, out, "Codex supervisor model now inherits the user default")
	saved, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, config.SupervisorModelPolicyInherit, saved.Quality.SupervisorModelPolicy)
	assert.Equal(t, "custom", saved.Quality.Default)
	assert.Equal(t, composeTableK, saved.Quality.Presets["custom"].Agents)
	assert.Len(t, *run.probes, 1)
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, readComposeCodexProfile(t, dir, "debugger"))
}

// S19 through update: under CAT-D the table marks the standard placements
// `생성 거부`, Enter keeps autopus.yaml, and the same run's Codex generation
// fails on the same snapshot while Claude Code is still updated.
func TestUpdateCompose_RefusedCodexPlacementFailsTheSameRunsGeneration(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	dir := run.settled(t, "D")
	probes := composeSessionCatalog(t, "CAT-D")
	planner := composeGeneratedPath(dir, ".claude", "agents", "autopus", "planner.md")
	rewriteComposeFile(t, planner, "model: claude-fable-5-1", "model: sonnet")
	before := composeSessionHash(t, dir)

	out, err := runComposeUpdate(t, strings.NewReader("\n"), "--dir", dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "플랫폼 업데이트 실패: codex")
	assert.Contains(t, out, "Codex 생성이 실패합니다\n")
	table := composeRenderedTable(t, out, "quality.default = balanced   ultra: 현재 기본값   balanced: 현재 기본값")
	for _, agent := range []string{"architect", "debugger", "deep-worker", "planner", "reviewer", "security-auditor", "spec-writer"} {
		assert.Equal(t, "생성 거부", table[agent]["Codex"], agent)
	}
	assert.Len(t, *probes, 1)
	assert.Empty(t, *run.probes)
	assert.False(t, run.adapterProbed(t))
	assert.Equal(t, before, composeSessionHash(t, dir))
	assert.Equal(t, "claude-fable-5-1", readComposeTableClaudeProfile(t, dir, "planner").model)
}
