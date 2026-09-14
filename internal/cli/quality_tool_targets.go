package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// qualityModelTool is a coding tool whose generated agent surface carries a
// per-agent model. Only these can be configured here: the tool either writes a
// model into each agent file (claude-code, codex) or binds one through its own
// registry (omp).
type qualityModelTool struct {
	platform string
	provider string
	label    string
}

// qualityModelTools lists the configurable tools in menu order. `provider` is
// the quality.providers key a tier preset binds to; omp has none because it
// routes through role_model_policy and concrete model selectors instead.
var qualityModelTools = []qualityModelTool{
	{platform: "claude-code", provider: config.QualityProviderClaude, label: "Claude Code"},
	{platform: "codex", provider: config.QualityProviderCodex, label: "Codex"},
	{platform: "omp", label: "OMP (Oh My Pi)"},
}

// qualityInheritOnlyTools name the installed platforms whose agent surfaces
// carry no model at all, so their subagents run on the session model. Offering
// a model menu for them would write a setting nothing reads.
var qualityInheritOnlyTools = map[string]string{
	"antigravity-cli": "Antigravity CLI",
	"opencode":        "OpenCode",
}

// qualityToolAgentRow is one operator-visible row: the relative tier the
// preset assigns and the concrete model that tier becomes on this tool.
type qualityToolAgentRow struct {
	Agent string
	Tier  string
	Model string
}

// configuredQualityModelTools returns the configurable tools this project
// actually installs, in menu order.
func configuredQualityModelTools(cfg *config.HarnessConfig) []qualityModelTool {
	tools := make([]qualityModelTool, 0, len(qualityModelTools))
	for _, tool := range qualityModelTools {
		if containsOMPString(cfg.Platforms, tool.platform) {
			tools = append(tools, tool)
		}
	}
	return tools
}

// inheritOnlyQualityToolLabels names installed platforms that have no model to
// set, so the menu can say why they are absent instead of omitting them.
func inheritOnlyQualityToolLabels(cfg *config.HarnessConfig) []string {
	labels := make([]string, 0, len(qualityInheritOnlyTools))
	for _, platform := range cfg.Platforms {
		if label, ok := qualityInheritOnlyTools[platform]; ok {
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)
	return labels
}

// qualityToolAgentRows projects the effective preset onto one tool's model
// vocabulary. Both values come from the config package, so the table shows the
// same decision the generator will write.
func qualityToolAgentRows(cfg *config.HarnessConfig, provider string) ([]qualityToolAgentRow, error) {
	canonical, ok := config.NormalizeQualityProvider(provider)
	if !ok {
		return nil, fmt.Errorf("quality.providers has no key for %q", provider)
	}
	agents := config.CanonicalAgentNames()
	rows := make([]qualityToolAgentRow, 0, len(agents))
	for _, agent := range agents {
		tier := cfg.Quality.AgentTier(canonical, agent, "")
		rows = append(rows, qualityToolAgentRow{
			Agent: agent, Tier: tier, Model: qualityToolModel(cfg, canonical, agent),
		})
	}
	return rows, nil
}

func qualityToolModel(cfg *config.HarnessConfig, provider, agent string) string {
	if provider == config.QualityProviderCodex {
		return cfg.Quality.CodexAgentModel(agent, "")
	}
	return cfg.Quality.ClaudeAgentModel(agent, "")
}

// parseQualityToolTierAssignment reads one `agent=tier` edit. Both halves are
// validated against the canonical vocabularies: an unknown agent name would
// persist a preset entry no generator reads, and an unknown tier would resolve
// silently to the mid tier.
func parseQualityToolTierAssignment(raw string) (agent, tier string, err error) {
	name, value, found := strings.Cut(strings.TrimSpace(raw), "=")
	if !found {
		return "", "", fmt.Errorf("expected <agent>=<tier>, got %q", strings.TrimSpace(raw))
	}
	agent = config.NormalizeAgentName(name)
	if !containsOMPString(config.CanonicalAgentNames(), agent) {
		return "", "", fmt.Errorf(
			"unknown agent %q (choose one of: %s)",
			strings.TrimSpace(name), strings.Join(config.CanonicalAgentNames(), ", "),
		)
	}
	tier, valid := config.NormalizeQualityTier(value)
	if !valid {
		return "", "", fmt.Errorf(
			"unknown tier %q (choose one of: %s)",
			strings.TrimSpace(value), strings.Join(config.QualityTiers(), ", "),
		)
	}
	return agent, tier, nil
}

// qualityToolPresetName is the default preset name a tool's custom tiers are
// stored under. Keeping the tool in the name makes a two-tool project readable
// in autopus.yaml.
func qualityToolPresetName(tool qualityModelTool) string {
	return tool.provider + "-custom"
}
