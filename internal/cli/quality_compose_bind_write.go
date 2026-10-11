package cli

import (
	"fmt"
	"maps"

	"github.com/insajin/autopus-adk/pkg/config"
)

// apply is the saveQualityBlock edit of the plan. It first checks every
// planned built-in write against the raw quality the writer decoded, then
// sets the plan's values, which come only from code defaults and the tier
// vocabulary — never from a loaded config. Init applies a confirmed plan to
// its in-memory config the same way before saving.
func (p composeBindingPlan) apply(quality *config.QualityConf) error {
	for _, write := range p.Presets {
		if write.expect == "" {
			continue
		}
		if status, _ := config.ClassifyBuiltinQualityPreset(*quality, write.Name); status.Class != write.expect {
			return fmt.Errorf("quality.presets.%s changed since the preview (now %s); nothing written",
				write.Name, status.Class)
		}
	}
	p.set(quality)
	return nil
}

func (p composeBindingPlan) set(quality *config.QualityConf) {
	if len(p.Presets) > 0 && quality.Presets == nil {
		quality.Presets = make(map[string]config.QualityPreset, len(p.Presets))
	}
	for _, write := range p.Presets {
		quality.Presets[write.Name] = config.QualityPreset{
			Description: write.Preset.Description,
			Agents:      maps.Clone(write.Preset.Agents),
		}
	}
	if p.Default != "" {
		quality.Default = p.Default
	}
}

// composeCandidate returns a copy of cfg whose quality maps are its own,
// carrying the plan's values, like withQualityToolPreset does for the
// per-tool editor's preview.
func composeCandidate(cfg *config.HarnessConfig, plan composeBindingPlan) *config.HarnessConfig {
	candidate := *cfg
	candidate.Quality.Providers = maps.Clone(cfg.Quality.Providers)
	candidate.Quality.Presets = make(map[string]config.QualityPreset, len(cfg.Quality.Presets)+len(plan.Presets))
	for name, preset := range cfg.Quality.Presets {
		candidate.Quality.Presets[name] = config.QualityPreset{
			Description: preset.Description,
			Agents:      maps.Clone(preset.Agents),
		}
	}
	plan.set(&candidate.Quality)
	return &candidate
}

// persistComposeBinding writes plan to dir's autopus.yaml through the writer
// it names (REQ-MUX-08). Both writers run the raw-source precheck and leave
// the file and cfg as they were on refusal (errQualityWriteRefused) or
// failure; after a write cfg.Quality holds the values written, so the same
// run generates from them.
func persistComposeBinding(dir string, cfg *config.HarnessConfig, plan composeBindingPlan) error {
	switch plan.Writer {
	case composeWriterLine:
		// saveQualityDefault sets the default on the config it receives
		// before it writes, so it gets a copy that a refusal can discard.
		staged := *cfg
		if err := saveQualityDefault(dir, &staged, plan.Default); err != nil {
			return err
		}
		cfg.Quality.Default = plan.Default
		return nil
	case composeWriterBlock:
		return saveQualityBlock(dir, cfg, plan.apply)
	default:
		return nil
	}
}
