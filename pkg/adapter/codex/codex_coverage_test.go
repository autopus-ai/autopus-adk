package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Extended Skills ---

func TestRenderExtendedSkills(t *testing.T) {
	t.Parallel()
	a := NewWithRoot(t.TempDir())
	files, err := a.renderExtendedSkills(config.DefaultFullConfig("test"))
	require.NoError(t, err)
	assert.NotEmpty(t, files)
	for _, f := range files {
		assert.Contains(t, f.TargetPath, ".codex/skills/")
		assert.Equal(t, adapter.OverwriteAlways, f.OverwritePolicy)
	}
}

func TestNormalizeCodexExtendedSkill_RewritesSpecialSkills(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("rewrite-project")

	teams := normalizeCodexExtendedSkill("agent-teams", "placeholder", cfg)
	assert.Contains(t, teams, "Codex Multi-Agent V2 Team Skill")
	assert.Contains(t, teams, "same shared cwd and filesystem")
	assert.Contains(t, teams, "Lead")
	assert.Contains(t, teams, "followup_task")
	assert.Contains(t, teams, "target-less `wait_agent()`")
	assert.NotContains(t, teams, "TeamCreate")
	assert.NotContains(t, teams, "SendMessage")
	assert.NotContains(t, teams, "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS")

	// Codex now consumes the canonical pipeline body and appends only a native
	// execution delta. The rewrite's contract is that delta: keep the shared
	// body intact, name the tools Codex actually exposes, and state the shared
	// filesystem consequence that a conversation fork does not solve.
	pipeline := normalizeCodexExtendedSkill("agent-pipeline", "canonical body marker", cfg)
	assert.Contains(t, pipeline, "canonical body marker",
		"the shared body must survive the Codex rewrite, not be replaced by it")
	for _, tool := range []string{
		"spawn_agent", "send_message", "followup_task",
		"wait_agent()", "interrupt_agent", "list_agents",
	} {
		assert.Contains(t, pipeline, tool, "the native delta must name %q", tool)
	}
	assert.NotContains(t, pipeline, "bypassPermissions")
	assert.NotContains(t, pipeline, "auto permission detect")
	for _, foreign := range []string{"TeamCreate", "SendMessage(", "Agent("} {
		assert.NotContains(t, pipeline, foreign,
			"the Codex delta must not reintroduce a Claude-only primitive")
	}

	worktree := normalizeCodexExtendedSkill("worktree-isolation", "placeholder", cfg)
	assert.Contains(t, worktree, "shared cwd and filesystem")
	assert.Contains(t, worktree, "disjoint write ownership")
	assert.Contains(t, worktree, "owned_paths")
	assert.Contains(t, worktree, "next_required_step")
	assert.NotContains(t, worktree, "forked workspace")

	prd := normalizeCodexExtendedSkill("prd", "사용자 입력이 불충분할 경우 AskUserQuestion으로 확인:", cfg)
	assert.NotContains(t, prd, "AskUserQuestion")
	assert.Contains(t, prd, "request_user_input")
	assert.Contains(t, prd, "plain-text")
}

func TestLogTransformReport_Nil(t *testing.T) {
	t.Parallel()
	logTransformReport(nil)
}

func TestLogTransformReport_WithData(t *testing.T) {
	t.Parallel()
	report := &pkgcontent.TransformReport{
		Platform:     "codex",
		Compatible:   []string{"skill-a", "skill-b"},
		Incompatible: []string{"skill-c"},
	}
	logTransformReport(report)
}

// --- Hooks ---

func TestInstallGitHooks(t *testing.T) {
	t.Parallel()
	require.NoError(t, NewWithRoot(t.TempDir()).installGitHooks(config.DefaultFullConfig("test")))
}

func TestRenderHooksTemplate(t *testing.T) {
	t.Parallel()
	rendered, err := NewWithRoot(t.TempDir()).renderHooksTemplate(config.DefaultFullConfig("test"))
	require.NoError(t, err)
	assert.Contains(t, rendered, "PreToolUse")
	assert.Contains(t, rendered, "PostToolUse")
	assert.Contains(t, rendered, "SessionStart")
	assert.Contains(t, rendered, `"hooks"`)
	assert.NotContains(t, rendered, "auto session save")
	assert.NotContains(t, rendered, "auto check --status")
	assert.NotContains(t, rendered, "auto check --lore --quiet")
}

func TestGenerateHooks_WritesToDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files, err := NewWithRoot(dir).generateHooks(config.DefaultFullConfig("test"))
	require.NoError(t, err)
	require.Len(t, files, 3)
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	require.NoError(t, err)
	assert.JSONEq(t, string(files[0].Content), string(data))
}

func TestPrepareHooksFile_MergesExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	hooksDir := filepath.Join(dir, ".codex")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "hooks.json"),
		[]byte(`{"hooks":{"CustomEvent":[{"command":"user.sh"}]}}`),
		0644,
	))

	files, err := a.prepareHooksFile(cfg)
	require.NoError(t, err)
	require.Len(t, files, 3)

	content := string(files[0].Content)
	assert.Contains(t, content, "user.sh", "user hook preserved")
	assert.Contains(t, content, "PreToolUse", "autopus hooks added")
	assert.Contains(t, content, "PostToolUse", "autopus hooks added")
}

func TestMergeHooks_InvalidRenderedJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := mergeHooks(filepath.Join(dir, "x.json"), "{bad")
	assert.Error(t, err)
}

func TestMergeHookCategories_EmptyDocs(t *testing.T) {
	t.Parallel()
	empty := hooksDoc{Hooks: map[string]hookGroups{}}
	result := mergeHookCategories(empty, empty)
	assert.NotNil(t, result.Hooks)
	assert.Empty(t, result.Hooks)
}

// --- Lifecycle ---

func TestValidate_MarkerPresentButNoSkills(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)

	content := "# Test\n" + markerBegin + "\ncontent\n" + markerEnd + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644))

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)

	found := false
	for _, e := range errs {
		if e.Level == "error" && e.File == ".codex/skills" {
			found = true
		}
	}
	assert.True(t, found, "should report missing .codex/skills")
}

func TestValidate_NoMarkerSection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "AGENTS.md"),
		[]byte("# No Marker\n"),
		0644,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "skills"), 0755))

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)

	found := false
	for _, e := range errs {
		if e.Level == "warning" && e.File == "AGENTS.md" {
			found = true
		}
	}
	assert.True(t, found, "should warn about missing marker")
}
