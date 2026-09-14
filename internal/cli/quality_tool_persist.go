package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// saveQualityToolPreset stores one tool's custom tier map as a quality preset
// and binds the tool to it. Writing the preset without the binding would leave
// the tool on its previous mode, so both land in the same file replacement.
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
	if cfg.Quality.Presets == nil {
		cfg.Quality.Presets = make(map[string]config.QualityPreset, 1)
	}
	cfg.Quality.Presets[preset] = config.QualityPreset{
		Description: tool.label + " custom agent tiers",
		Agents:      agents,
	}
	if cfg.Quality.Providers == nil {
		cfg.Quality.Providers = make(map[string]string, 1)
	}
	cfg.Quality.Providers[tool.provider] = preset
	return saveQualitySection(dir, cfg)
}

// saveQualitySection persists the whole quality block. The scalar and provider
// writers edit single lines because they touch one value; a preset is a nested
// mapping, so this replaces the quality node and leaves every other top-level
// key byte-identical.
func saveQualitySection(dir string, cfg *config.HarnessConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	path := filepath.Join(dir, "autopus.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		encoded, marshalErr := yaml.Marshal(cfg)
		if marshalErr != nil {
			return fmt.Errorf("marshal config: %w", marshalErr)
		}
		if writeErr := atomicWriteQualityConfig(path, encoded); writeErr != nil {
			return fmt.Errorf("write config: %w", writeErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	updated, err := replaceAutopusConfigSection(data, "quality", cfg.Quality)
	if err != nil {
		return err
	}
	if err := validateQualityYAML(updated, cfg); err != nil {
		return err
	}
	if err := atomicWriteQualityConfig(path, updated); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
