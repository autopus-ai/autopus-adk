package codex

import (
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexRawCollaborationTemplates_HaveNoLegacyResiduals(t *testing.T) {
	t.Parallel()
	bodies := map[string]string{
		"agent-teams-native":   codexAgentTeamsSkillBody(config.CodexAgentConcurrencyDefault),
		"worktree-isolation":   codexWorktreeIsolationSkillBody(),
		"subagent-development": codexSubagentDevSkillBody(),
	}
	for _, name := range []string{"agent-teams.md.tmpl", "auto-go.md.tmpl", "auto-idea.md.tmpl"} {
		data, err := templates.FS.ReadFile("codex/skills/" + name)
		require.NoError(t, err)
		bodies[name] = string(data)
	}
	for name, body := range bodies {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, obsolete := range []string{
				"send_input", "resume_agent", "close_agent", "forked workspace",
				"auto pipeline worktree", "worktree branches", "merge worktree",
			} {
				assert.NotContains(t, strings.ToLower(body), obsolete, name)
			}
			assert.Contains(t, body, "spawn_agent", name)
			assert.Contains(t, body, "task_name", name)
			assert.Contains(t, body, "message", name)
			assert.Contains(t, body, "send_message", name)
			assert.Contains(t, body, "followup_task", name)
			assert.Contains(t, body, "wait_agent()", name)
			assert.Contains(t, body, "interrupt_agent", name)
			assert.Contains(t, body, "list_agents", name)
			lower := strings.ToLower(body)
			assert.True(t, strings.Contains(lower, "shared cwd") || strings.Contains(lower, "same cwd"), name)
			assert.Contains(t, strings.ToLower(body), "disjoint write ownership", name)
		})
	}
}
