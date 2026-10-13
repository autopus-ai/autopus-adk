package cli

import (
	"fmt"
	"maps"

	"github.com/insajin/autopus-adk/pkg/config"
)

// saveQualityToolPreset stores one tool's custom tier map as a quality preset
// and binds the tool to it. Writing the preset without the binding would leave
// the tool on its previous mode, so both land in the same file replacement.
// The per-tool editor edits tiers only, so the preset keeps the efforts the
// tool's preset stores, read from the raw quality the edit receives
// (SPEC-MODELUX-003 REQ-MUX3-20).
//
// The write goes through saveQualityBlock (SPEC-MODELUX-001 REQ-MUX-08): the
// edit adds only this preset and this binding to the raw quality section, so
// no environment value the loader expanded into cfg is ever written back
// (F-001), a block holding an environment value or a YAML reference is
// refused with errQualityWriteRefused, and every byte outside the quality
// block stays as it was. cfg follows the written quality only after a write.
func saveQualityToolPreset(
	dir string,
	cfg *config.HarnessConfig,
	tool qualityModelTool,
	preset string,
	agents map[string]string,
) error {
	return saveQualityToolPresetWith(dir, cfg, tool, preset, agents, func(quality config.QualityConf) map[string]string {
		return qualityToolEfforts(quality, tool)
	})
}

// saveQualityToolPresetEfforts is saveQualityToolPreset whose preset stores
// efforts, the explicit efforts of a tool tab's working composition
// (SPEC-MODELUX-003 REQ-MUX3-04), in the same one block edit.
func saveQualityToolPresetEfforts(
	dir string,
	cfg *config.HarnessConfig,
	tool qualityModelTool,
	preset string,
	agents, efforts map[string]string,
) error {
	return saveQualityToolPresetWith(dir, cfg, tool, preset, agents, func(config.QualityConf) map[string]string {
		return efforts
	})
}

// saveQualityToolPresetWith validates the write and performs it with the
// efforts effortsOf answers for the raw quality being edited.
func saveQualityToolPresetWith(
	dir string,
	cfg *config.HarnessConfig,
	tool qualityModelTool,
	preset string,
	agents map[string]string,
	effortsOf func(config.QualityConf) map[string]string,
) error {
	if !config.IsValidQualityPresetName(preset) {
		return fmt.Errorf(
			"invalid quality preset name %q (use 1-64 ASCII letters, digits, hyphens, or underscores)",
			preset,
		)
	}
	if tool.provider == "" {
		return fmt.Errorf("%s has no quality.providers key", tool.label)
	}
	if len(agents) == 0 {
		return fmt.Errorf("preset %q would assign no agent a tier", preset)
	}
	return saveQualityBlock(dir, cfg, func(quality *config.QualityConf) error {
		setQualityToolPreset(quality, tool, preset, agents, effortsOf(*quality))
		return nil
	})
}

// setQualityToolPreset sets the two keys a tool's custom tiers take: the
// preset holding them and efforts, with the tool's description, and the
// binding of the tool to it. The values come from the tier and effort
// vocabularies and the tool alone, never from a loaded config.
func setQualityToolPreset(quality *config.QualityConf, tool qualityModelTool, preset string, agents, efforts map[string]string) {
	if quality.Presets == nil {
		quality.Presets = make(map[string]config.QualityPreset, 1)
	}
	quality.Presets[preset] = config.QualityPreset{
		Description: tool.label + " custom agent tiers",
		Agents:      maps.Clone(agents),
		Efforts:     presetEfforts(efforts),
	}
	if quality.Providers == nil {
		quality.Providers = make(map[string]string, 1)
	}
	quality.Providers[tool.provider] = preset
}

// presetEfforts is the efforts value a written preset holds: a copy, or nil
// for none, which the omitempty encoding writes as no key at all.
func presetEfforts(efforts map[string]string) map[string]string {
	if len(efforts) == 0 {
		return nil
	}
	return maps.Clone(efforts)
}

// qualityToolEfforts returns the explicit efforts tool's effective preset
// stores, each a vocabulary level as AgentEffort reads it, or nil for none.
func qualityToolEfforts(quality config.QualityConf, tool qualityModelTool) map[string]string {
	return qualityViewEfforts(quality, tool.provider)
}

// qualityViewEfforts is qualityToolEfforts for any provider view; "" is the
// global binding every tool without an override follows.
func qualityViewEfforts(quality config.QualityConf, provider string) map[string]string {
	var efforts map[string]string
	for _, agent := range config.CanonicalAgentNames() {
		if effort, ok := quality.AgentEffort(provider, agent); ok {
			if efforts == nil {
				efforts = make(map[string]string)
			}
			efforts[agent] = effort
		}
	}
	return efforts
}
