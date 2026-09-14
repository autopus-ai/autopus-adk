package adapter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertClaudeLatestCLIContract(t *testing.T, fixture latestCLIFixture) {
	t.Helper()
	manifest := fixture.manifests["claude-code"]
	skillCount := 0
	for _, path := range manifestPaths(manifest) {
		if !strings.HasPrefix(path, ".claude/skills/") {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) == 5 && parts[3] == "references" {
			// Resources are installed beside their skill; the entrypoint layout
			// contract belongs to the SKILL.md, not to its reference bodies.
			assert.True(t, strings.HasSuffix(parts[4], ".md"), path)
			require.True(t, manifestHas(manifest, strings.Join(parts[:3], "/")+"/SKILL.md"), path)
			continue
		}
		skillCount++
		require.Len(t, parts, 4, path)
		assert.Equal(t, "SKILL.md", parts[3], path)
		assert.NotEqual(t, "autopus", parts[2], path)
	}
	assert.Greater(t, skillCount, 20)
	assert.True(t, manifestHas(manifest, ".claude/skills/auto-go/SKILL.md"))
	assert.NoDirExists(t, filepath.Join(fixture.root, ".claude", "skills", "autopus"))
	assert.NoFileExists(t, filepath.Join(fixture.root, ".claude", "skills", "auto.md"))

	autoGo := readLatestCLISurface(t, fixture, ".claude/skills/auto-go/SKILL.md")
	active := autoGo +
		readLatestCLISurface(t, fixture, ".claude/workflows/route_a.workflow.js") +
		readLatestCLISurface(t, fixture, ".claude/workflows/route_team.workflow.js")
	for _, forbidden := range []string{"TeamCreate", "TeamDelete", "team_name", "team_default"} {
		assert.NotContains(t, active, forbidden)
	}
	for _, required := range []string{
		"TOPOLOGY_FLAG_COUNT > 1",
		"[TOPOLOGY ERROR] --team, --workflow, and --solo are mutually exclusive; choose only one.",
		"Workflow({",
		`scriptPath: ".claude/workflows/route_a.workflow.js"`,
		"args: {",
	} {
		assert.Contains(t, autoGo, required)
	}

	deferred := readLatestCLISurface(t, fixture, ".claude/rules/autopus/deferred-tools.md")
	for _, required := range []string{"SendMessage", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "Workflow"} {
		assert.Contains(t, deferred, required)
	}
	for _, removed := range []string{"TeamCreate", "TeamDelete", "TaskOutput", "TaskStop"} {
		assert.NotContains(t, deferred, removed)
	}
}

func assertCodexLatestCLIContract(t *testing.T, fixture latestCLIFixture) {
	t.Helper()
	manifest := fixture.manifests["codex"]
	skillCount := 0
	for _, path := range manifestPaths(manifest) {
		assert.False(t, strings.HasPrefix(path, ".codex/prompts/"), path)
		assert.False(t, strings.HasPrefix(path, ".agents/skills/"), path)
		assert.False(t, strings.HasPrefix(path, ".codex/rules/") && strings.HasSuffix(path, ".md"), path)
		if !strings.HasPrefix(path, ".codex/skills/") {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) == 5 && parts[3] == "references" {
			// Resources are installed beside their skill; the entrypoint layout
			// contract belongs to the SKILL.md, not to its reference bodies.
			assert.True(t, strings.HasSuffix(parts[4], ".md"), path)
			require.True(t, manifestHas(manifest, strings.Join(parts[:3], "/")+"/SKILL.md"), path)
			continue
		}
		skillCount++
		require.Len(t, parts, 4, path)
		assert.True(t, strings.HasPrefix(parts[2], "codex-"), path)
		assert.Equal(t, "SKILL.md", parts[3], path)
	}
	assert.Greater(t, skillCount, 20)
	assert.NoDirExists(t, filepath.Join(fixture.root, ".codex", "prompts"))
	assert.NoFileExists(t, filepath.Join(fixture.root, ".codex", "skills", "auto.md"))

	configBody := readLatestCLISurface(t, fixture, ".codex/config.toml")
	for _, required := range []string{
		"[features.multi_agent_v2]", "enabled = true",
		"max_concurrent_threads_per_session = 4",
	} {
		assert.Contains(t, configBody, required)
	}
	for _, legacy := range []string{"multi_agent =", "max_threads", "max_depth", "job_max_runtime_seconds"} {
		assert.NotContains(t, configBody, legacy)
	}

	contracts := readLatestCLISurface(t, fixture, ".codex/skills/codex-agent-teams/SKILL.md") +
		readLatestCLISurface(t, fixture, ".codex/skills/codex-agent-pipeline/SKILL.md") +
		readLatestCLISurface(t, fixture, ".codex/skills/codex-worktree-isolation/SKILL.md")
	for _, tool := range []string{
		"spawn_agent", "send_message", "followup_task",
		"wait_agent", "interrupt_agent", "list_agents",
	} {
		assert.Contains(t, contracts, tool)
	}
	for _, legacy := range []string{
		"send_input", "resume_agent", "close_agent", "forked workspace",
		"auto-merges worktree", "merge worktree branch",
	} {
		assert.NotContains(t, contracts, legacy)
	}
	assert.Contains(t, contracts, "shared cwd")
	assert.Contains(t, contracts, "disjoint write ownership")

	assert.False(t, manifestHas(manifest, "AGENTS.md"))
	assert.True(t, manifestHas(fixture.manifests["opencode"], "AGENTS.md"))
	assert.False(t, manifestHas(manifest, ".agents/skills/auto/SKILL.md"))
	assert.True(t, manifestHas(fixture.manifests["opencode"], ".agents/skills/auto/SKILL.md"))
	assert.True(t, manifestHas(manifest, ".agents/plugins/marketplace.json"))
}
