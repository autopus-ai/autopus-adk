package config

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
)

// qualityEfforts is the vocabulary of quality.presets.<name>.efforts
// (SPEC-MODELUX-003 REQ-MUX3-01), weakest first: the reasoning levels both
// Claude Code and Codex agents accept. Codex's ultra is not one of them,
// because a Codex agent lowers it to max (normalizeManagedCodexEffort), and
// an OMP thinking level is stored on its role model pin instead.
var qualityEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// QualityEfforts returns a detached copy of the effort vocabulary, weakest
// first.
func QualityEfforts() []string {
	return slices.Clone(qualityEfforts)
}

// IsQualityEffort reports whether effort is a vocabulary level, spelled
// exactly: the loader accepts nothing else, as it accepts tiers.
func IsQualityEffort(effort string) bool {
	return slices.Contains(qualityEfforts, effort)
}

// AgentEffort resolves the explicit effort one agent's preset stores under
// provider's effective quality view, reading the very preset AgentTier
// reads. The second result is false when the preset stores none, which is
// when every projection keeps its automatic effort.
func (q QualityConf) AgentEffort(provider, agentName string) (string, bool) {
	preset, ok := q.Presets[q.ForProvider(provider).Default]
	if !ok {
		preset, ok = q.Presets["balanced"]
	}
	if !ok {
		return "", false
	}
	effort, explicit := preset.Efforts[NormalizeAgentName(agentName)]
	if !explicit || !IsQualityEffort(effort) {
		return "", false
	}
	return effort, true
}

// validateQualityPresetEfforts fails closed on an efforts key that is not a
// canonical agent name and on a value outside the vocabulary, naming the key
// path and the value. Presets and agents are visited in sorted order, so a
// file with several mistakes reports the same one on every load. Preset
// names were validated before; an agent key is free text from autopus.yaml
// and is quoted unless it is a plain identifier.
func (c *HarnessConfig) validateQualityPresetEfforts() error {
	for _, presetName := range slices.Sorted(maps.Keys(c.Quality.Presets)) {
		efforts := c.Quality.Presets[presetName].Efforts
		for _, agent := range slices.Sorted(maps.Keys(efforts)) {
			value := efforts[agent]
			if !slices.Contains(canonicalAgentNames, agent) {
				return fmt.Errorf("quality.presets.%s.efforts.%s: agent_invalid %q", presetName, effortKeyText(agent), value)
			}
			if !IsQualityEffort(value) {
				return fmt.Errorf("quality.presets.%s.efforts.%s: effort_invalid %q", presetName, agent, value)
			}
		}
	}
	return nil
}

// effortKeyText prints an efforts key as written when it is a plain
// identifier and quoted otherwise, so no key can move the cursor.
func effortKeyText(agent string) string {
	if IsValidQualityPresetName(agent) {
		return agent
	}
	return strconv.Quote(agent)
}
