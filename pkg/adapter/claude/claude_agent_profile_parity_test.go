package claude_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

// TestClaudeAgentFrontmatterEqualsSharedProjection proves the generator writes
// exactly config.QualityConf.ClaudeAgentProfile for every canonical agent, fed
// the embedded source `model:` line as the fallback tier. That pair is what the
// composition table shows in the Claude Code column (SPEC-MODELUX-001
// REQ-MUX-05), so table and generated file cannot disagree.
func TestClaudeAgentFrontmatterEqualsSharedProjection(t *testing.T) {
	t.Parallel()
	shippedUltra := map[string]string{
		"planner": "opus", "executor": "opus", "validator": "opus", "tester": "gpt",
		"reviewer": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"debugger": "opus", "explorer": "opus", "devops": "opus",
	}
	shippedBalanced := map[string]string{
		"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet",
		"devops": "sonnet", "validator": "haiku", "explorer": "haiku",
	}
	configs := map[string]func(*config.HarnessConfig){
		"standard balanced": func(*config.HarnessConfig) {},
		"older ultra with an invalid tier": func(cfg *config.HarnessConfig) {
			cfg.Quality.Default = "ultra"
			cfg.Quality.Presets["ultra"] = config.QualityPreset{Agents: shippedUltra}
		},
		"older balanced": func(cfg *config.HarnessConfig) {
			cfg.Quality.Presets["balanced"] = config.QualityPreset{Agents: shippedBalanced}
		},
		"claude override": func(cfg *config.HarnessConfig) {
			cfg.Quality.Presets["custom"] = config.QualityPreset{Agents: map[string]string{"planner": "haiku", "executor": "opus"}}
			cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: "custom"}
		},
	}
	for name, mutate := range configs {
		cfg := config.DefaultFullConfig("profile-parity")
		mutate(cfg)
		agentDir := generateClaudeAgents(t, cfg)
		for _, agent := range config.CanonicalAgentNames() {
			fallback := frontmatterValue(sourceFrontmatterLines(t, agent+".md"), "model")
			wantModel, wantEffort := cfg.Quality.ClaudeAgentProfile(agent, fallback)
			lines := agentFrontmatterLines(t, agentDir, agent+".md")
			assert.Equal(t, wantModel, frontmatterValue(lines, "model"), "%s: %s model", name, agent)
			assert.Equal(t, wantEffort, frontmatterValue(lines, "effort"), "%s: %s effort", name, agent)
		}
	}
}

// frontmatterValue returns one top-level frontmatter scalar, or "" when the
// key is absent (Claude omits effort for haiku).
func frontmatterValue(lines []string, key string) string {
	for _, line := range lines {
		if value, ok := strings.CutPrefix(line, key+":"); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
