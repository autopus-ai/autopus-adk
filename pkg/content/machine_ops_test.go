package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// machine_ops: false leaves out the disk gate unless the project sets it.
func TestGenerateProjectHookConfigs_MachineOpsOffDropsTheDiskGate(t *testing.T) {
	t.Parallel()
	hasGate := func(cfg *config.HarnessConfig) bool {
		hooks, _, err := content.GenerateProjectHookConfigs(cfg, "claude-code", true)
		require.NoError(t, err)
		for _, h := range hooks {
			if h.Event == "PreToolUse" && h.Command != "" && contains(h.Command, "auto disk gate") {
				return true
			}
		}
		return false
	}
	cfg := config.DefaultFullConfig("ops")
	assert.True(t, hasGate(cfg), "unset machine_ops keeps the gate")
	off, on := false, true
	cfg.MachineOps = &off
	assert.False(t, hasGate(cfg))
	cfg.Hooks.DiskGate = &config.DiskGateConf{Enabled: &on}
	assert.True(t, hasGate(cfg), "hooks.disk_gate set explicitly wins")
}

func TestIsMachineOpsSkill(t *testing.T) {
	t.Parallel()
	assert.True(t, content.IsMachineOpsSkill("disk-triage"))
	assert.True(t, content.IsMachineOpsSkill("ci-optimize"))
	assert.False(t, content.IsMachineOpsSkill("review"))
	assert.False(t, content.IsMachineOpsSkill("stack-freshness"), "the version gate is on by default and points at it")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// machine_ops: false keeps disk-triage and ci-optimize off the surface; the
// ops bundle or explicit_skills brings them back.
func TestMachineOpsSkillsFollowTheKey(t *testing.T) {
	t.Parallel()
	catalog, err := content.LoadSkillCatalogFromFS(contentfs.FS, "skills")
	require.NoError(t, err)
	skill, ok := catalog.Get("disk-triage")
	require.True(t, ok)
	review, ok := catalog.Get("review")
	require.True(t, ok)

	cfg := config.DefaultFullConfig("ops")
	assert.True(t, content.ResolveCatalogSkillState(skill, "claude-code", cfg).Compiled, "unset keeps it")
	off := false
	cfg.MachineOps = &off
	assert.False(t, content.ResolveCatalogSkillState(skill, "claude-code", cfg).Compiled)
	assert.True(t, content.ResolveCatalogSkillState(review, "claude-code", cfg).Compiled, "the verification core stays")
	cfg.Skills.Compiler.Bundles = []string{"ops"}
	assert.True(t, content.ResolveCatalogSkillState(skill, "claude-code", cfg).Compiled, "the ops bundle brings it back")
}
