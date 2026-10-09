package adapter_test

import (
	"context"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// claudeOnlyWorktreeMechanism is the Claude Code isolation syntax: only the
// Claude Agent tool accepts an isolation parameter that places a worker in a
// runtime-created worktree.
var claudeOnlyWorktreeMechanism = []string{
	"Agent(", `isolation: "worktree"`, `isolation = "worktree"`, "Claude Code",
}

// worktreeIsolationSkills returns the generated worktree-isolation skill body
// of each platform.
func worktreeIsolationSkills(t *testing.T) map[string]string {
	t.Helper()
	cfg := config.DefaultFullConfig("worktree-isolation-surface")
	cfg.Platforms = allPlatforms
	skills := map[string]string{}
	for _, platform := range allPlatforms {
		pf, err := platformGenerators[platform](t.TempDir(), context.Background(), cfg)
		require.NoError(t, err)
		for _, file := range pf.Files {
			target := strings.ReplaceAll(file.TargetPath, "\\", "/")
			if path.Base(target) == "SKILL.md" && strings.HasSuffix(path.Dir(target), "worktree-isolation") &&
				!strings.Contains(target, "/plugins/") {
				skills[platform] = string(file.Content)
			}
		}
		require.Contains(t, skills, platform, "%s installs a worktree-isolation skill", platform)
	}
	return skills
}

// Only Claude Code has an Agent tool that creates a worktree for a worker.
// Every other surface's skill must describe a mechanism that surface has, not
// tell its agent to pass Claude syntax to a tool it does not expose.
func TestWorktreeIsolationSkill_NonClaudeSurfacesDescribeTheirOwnMechanism(t *testing.T) {
	t.Parallel()
	skills := worktreeIsolationSkills(t)

	for _, token := range []string{`isolation: "worktree"`, "Agent("} {
		assert.Contains(t, skills["claude-code"], token, "Claude Code keeps its native isolation syntax")
	}
	for platform, body := range skills {
		if platform == "claude-code" {
			continue
		}
		for _, token := range claudeOnlyWorktreeMechanism {
			assert.NotContains(t, body, token, "%s worktree-isolation skill", platform)
		}
	}
	// Gemini/Antigravity and OpenCode workers share the supervisor's checkout,
	// so the supervisor runs the worktree lifecycle itself and keeps the slot,
	// fail-closed, and terminal-state contract.
	for _, platform := range []string{"antigravity-cli", "opencode"} {
		for _, token := range []string{
			"git -c gc.auto=0 worktree add -b worktree/", "worktree_path", "fifo_task_id", "worktree_slot_cap",
			"Slot reclaim", "worktree_isolation_unavailable", "preserved_for_manual_review",
			"Migration numbering lane", "same migration numbering lane",
		} {
			assert.Contains(t, skills[platform], token, "%s worktree-isolation skill", platform)
		}
	}
}

// `auto pipeline` has no worktree subcommand, so no generated surface may tell
// an agent to run or pass one.
func TestGeneratedSurfaces_NeverNameAPipelineWorktreeCommand(t *testing.T) {
	t.Parallel()
	surface := generateSurface(t, allPlatforms)
	for file, body := range surface.files {
		assert.NotContains(t, body, "auto pipeline worktree", file)
	}
}
