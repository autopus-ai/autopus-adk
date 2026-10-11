package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// saveQualityBlock persists one confirmed quality change to dir's
// autopus.yaml (SPEC-MODELUX-001 REQ-MUX-08). edit receives the raw quality
// section, decoded without environment expansion, and applies only the
// change: it must not copy values from a loaded config, which may hold
// expanded environment values. The writer refuses (errQualityWriteRefused)
// a quality block whose raw decode differs from what config.Load reads or
// that holds an alias or merge key, then splices the re-encoded block into
// the original bytes, so every byte outside the block stays identical. The
// result must pass verifyQualityLineEdit, validateQualityYAML, and
// verifyQualityPlaceholders, which refuses a rewrite that would spell out a
// placeholder the loader then expands, before the atomic replace; on any
// failure the file and cfg are left as they were. An
// edit that changes no value writes nothing and leaves cfg alone. After a
// write, cfg.Quality, when cfg is not nil, holds the quality written.
func saveQualityBlock(dir string, cfg *config.HarnessConfig, edit func(*config.QualityConf) error) error {
	path := filepath.Join(dir, "autopus.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return createQualityBlockConfig(dir, path, cfg, edit)
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	// The retired orchestra keys never ride along (SPEC-PANERM-001 REQ-10).
	if data, _, err = pruneRetiredConfig(data); err != nil {
		return err
	}
	source, err := readQualityWriteSource(data)
	if err != nil {
		return err
	}
	unchanged, err := source.rawQuality()
	if err != nil {
		return errQualityWriteRefused
	}
	candidate, err := source.rawQuality()
	if err != nil {
		return errQualityWriteRefused
	}
	if err := edit(&candidate); err != nil {
		return err
	}
	normalizeQualityConf(&unchanged)
	normalizeQualityConf(&candidate)
	if reflect.DeepEqual(candidate, unchanged) {
		return nil
	}

	expected := *source.loaded
	expected.Quality = candidate
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	updated, err := spliceQualityBlock(data, source, candidate)
	if err != nil {
		return err
	}
	if err := verifyQualityLineEdit(data, updated, "quality"); err != nil {
		return err
	}
	if err := validateQualityYAML(updated, &expected); err != nil {
		return err
	}
	if err := verifyQualityPlaceholders(data, updated, candidate); err != nil {
		return err
	}
	if err := atomicWriteQualityConfig(path, updated); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if cfg != nil {
		cfg.Quality = candidate
	}
	return nil
}

// createQualityBlockConfig writes a new autopus.yaml for saveQualityBlock
// when there is none. The loader then reads the code defaults, so they are
// the write source: no loaded value is written.
func createQualityBlockConfig(
	dir, path string,
	cfg *config.HarnessConfig,
	edit func(*config.QualityConf) error,
) error {
	created := config.DefaultFullConfig(filepath.Base(dir))
	if err := edit(&created.Quality); err != nil {
		return err
	}
	normalizeQualityConf(&created.Quality)
	if err := created.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	data, err := yaml.Marshal(created)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := validateQualityYAML(data, created); err != nil {
		return err
	}
	if err := atomicWriteQualityConfig(path, data); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if cfg != nil {
		cfg.Quality = created.Quality
	}
	return nil
}

// normalizeQualityConf drops the empty maps that the omitempty encoding of
// QualityConf drops, so a written quality decodes equal to its source.
func normalizeQualityConf(quality *config.QualityConf) {
	if len(quality.Providers) == 0 {
		quality.Providers = nil
	}
	if len(quality.Presets) == 0 {
		quality.Presets = nil
		return
	}
	for name, preset := range quality.Presets {
		if preset.Agents != nil && len(preset.Agents) == 0 {
			preset.Agents = nil
			quality.Presets[name] = preset
		}
	}
}
