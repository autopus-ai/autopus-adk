package cli

import (
	"fmt"
	"maps"

	"github.com/insajin/autopus-adk/pkg/config"
)

// saveQualityToolPreset stores one tool's custom tier map as a quality preset
// and binds the tool to it. Writing the preset without the binding would leave
// the tool on its previous mode, so both land in the same file replacement.
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
	written := config.QualityPreset{
		Description: tool.label + " custom agent tiers",
		Agents:      maps.Clone(agents),
	}
	return saveQualityBlock(dir, cfg, func(quality *config.QualityConf) error {
		if quality.Presets == nil {
			quality.Presets = make(map[string]config.QualityPreset, 1)
		}
		quality.Presets[preset] = config.QualityPreset{
			Description: written.Description,
			Agents:      maps.Clone(written.Agents),
		}
		if quality.Providers == nil {
			quality.Providers = make(map[string]string, 1)
		}
		quality.Providers[tool.provider] = preset
		return nil
	})
}
