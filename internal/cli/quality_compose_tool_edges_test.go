package cli

// SPEC-MODELUX-002 label and change-line edges: an agent list without a
// tool column names the tier alone, a Codex tab edit that leaves ultra
// names the Codex default session it moves, and a file whose quality block
// lines cannot be located says the write will be refused.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestComposeLabels_AgentEntryWithoutToolColumnsNamesTheTier(t *testing.T) {
	dir := writeComposeFixture(t, "opencode", "antigravity-cli")
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)
	planner := session.Planner.(*composeSessionPlanner)

	assert.Equal(t, "planner — fable", newComposeAllToolsLabeler(planner).agentEntry(planner.Start(), "planner"))
}

// R binds ultra with supervisor_model_policy quality: a Codex tab edit
// rebinds Codex to codex-custom, which moves the managed default session off
// the ultra effort, and the preview says so.
func TestComposeToolPlanner_LeavingUltraNamesTheCodexDefaultSessionMove(t *testing.T) {
	dir, _ := composeSessionProject(t, "R")
	planner := composeToolPlanner{run: newComposeToolRun(t, dir, composeSessionFile), tool: composeToolNamed(t, "codex")}
	edited := planner.Start()
	for _, agent := range composeRoleGroups[0].Agents {
		edited[agent] = "fable"
	}

	plan, err := planner.PlanEdit(edited)

	require.NoError(t, err)
	assert.Contains(t, plan.Changes, "Codex 기본 세션: 6-astra · ultra → 6-astra · xhigh")
}

// A line break YAML reads but the splice cannot locate makes every writer
// refuse, so the preview says so instead of the rewrite notice.
func TestComposeToolPlanner_UnlocatableBlockSaysTheWriteIsRefused(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")
	path := filepath.Join(dir, "autopus.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append([]byte("# note "+string(rune(0x2028))+"#\n"), data...), 0o644))
	planner := composeToolPlanner{run: newComposeToolRun(t, dir, composeSessionFile), tool: composeToolNamed(t, "codex")}

	plan, err := planner.PlanEdit(composeS5Edit(planner.Start()))

	require.NoError(t, err)
	assert.Contains(t, plan.Changes, composeBindTextFor("ko").blockUnreadable)
	assert.NotContains(t, plan.Changes, composeToolBlockNotice)
}
