package cli

import (
	"maps"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeWriter names the quality writer a binding plan persists through.
type composeWriter int

const (
	// composeWriterNone writes nothing: the composition did not change.
	composeWriterNone composeWriter = iota
	// composeWriterLine rewrites the quality.default line alone (saveQualityDefault).
	composeWriterLine
	// composeWriterBlock splices a re-encoded quality block (saveQualityBlock).
	composeWriterBlock
)

const (
	// composeCustomPreset holds a composition no current built-in default
	// matches. It is an ordinary preset name, so every shipped binary reads
	// it the same way.
	composeCustomPreset = "custom"
	// composeCustomDescription stays the same in every locale (REQ-MUX-22).
	composeCustomDescription = "역할 그룹 구성"
)

// composeBuiltinPresets lists the built-in presets in the order the
// composition step names them.
var composeBuiltinPresets = []string{"ultra", "balanced"}

// composePresetWriteKind says how one preset write relates to the file.
type composePresetWriteKind int

const (
	// composePresetNew writes a preset the file does not have.
	composePresetNew composePresetWriteKind = iota
	// composePresetRewrite rewrites custom while it is the current binding.
	composePresetRewrite
	// composePresetOverwrite replaces a custom that is not the binding.
	composePresetOverwrite
	// composePresetRefresh replaces a historical built-in this write binds.
	composePresetRefresh
	// composePresetUpdate replaces a historical built-in through `u`.
	composePresetUpdate
)

// composePresetWrite is one quality.presets.<name> write of a plan.
type composePresetWrite struct {
	Name   string
	Kind   composePresetWriteKind
	Preset config.QualityPreset
	// From is the agents map the file holds now, for the refresh diff.
	From map[string]string
	// ModelChange is false for a `u` refresh that moves no model: the preset
	// feeds no installed column, or it is a historical balanced layout the
	// runtime already keeps on the standard placement.
	ModelChange bool
	// expect is the GAP 1 class a built-in had when planned. The write
	// refuses when the file no longer agrees, so a preset the operator
	// customized after the preview is never replaced.
	expect config.BuiltinQualityPresetClass
	// runtimeStandard marks a historical balanced layout that
	// NativeBalancedAgentCandidate keeps on the standard placement.
	runtimeStandard bool
	// Carried counts the efforts this write moves from the binding preset
	// to another name (SPEC-MODELUX-003 REQ-MUX3-20), for the preview line.
	Carried int
}

// composeBindingPlan is the write one confirmed composition makes.
type composeBindingPlan struct {
	Writer composeWriter
	// Default is the quality.default the write sets, or "" when the line stays.
	Default string
	Presets []composePresetWrite
	// GivesUp names the built-in mode whose runtime extras a custom binding
	// leaves behind (C6): "balanced" (standard placement) or "ultra" (effort
	// ladder), or "".
	GivesUp string
	// Candidate is a detached copy of the loaded config with the write
	// applied: the after-table resolves it, and the loaded config stays as it
	// was until the write succeeds.
	Candidate *config.HarnessConfig
}

// planComposeBinding chooses the write for a confirmed composition by the
// Binding Contract (REQ-MUX-07, REQ-MUX-12). cfg is the loaded config: it
// decides the write and seeds the preview, but no value of it is written.
// A composition equal to its start writes nothing.
func planComposeBinding(cfg *config.HarnessConfig, start, edited qualityComposition) composeBindingPlan {
	var plan composeBindingPlan
	if !edited.Equal(start) {
		name, write := composeBindTarget(cfg.Quality, edited)
		if write != nil {
			plan.Presets = []composePresetWrite{*write}
		}
		if name == composeCustomPreset {
			plan.GivesUp = composeGivenUpMode(cfg.Quality)
		}
		if cfg.Quality.Default != name {
			plan.Default = name
		}
	}
	plan.Writer = composePlanWriter(plan)
	plan.Candidate = composeCandidate(cfg, plan)
	return plan
}

// planComposeBindingAfter plans group or advanced edits made on top of an
// unconfirmed `u` plan — `e` from its preview returns to the edit menu with
// the refreshed candidate (REQ-MUX-06). The edits bind against that
// candidate, and the refresh stays part of the one write instead of being
// dropped.
func planComposeBindingAfter(
	cfg *config.HarnessConfig,
	base composeBindingPlan,
	start, edited qualityComposition,
) composeBindingPlan {
	next := planComposeBinding(base.Candidate, start, edited)
	plan := composeBindingPlan{Default: next.Default, GivesUp: next.GivesUp}
	plan.Presets = append(append(plan.Presets, base.Presets...), next.Presets...)
	plan.Writer = composePlanWriter(plan)
	plan.Candidate = composeCandidate(cfg, plan)
	plan.markModelChanges()
	return plan
}

func composePlanWriter(plan composeBindingPlan) composeWriter {
	switch {
	case len(plan.Presets) > 0:
		return composeWriterBlock
	case plan.Default != "":
		return composeWriterLine
	default:
		return composeWriterNone
	}
}

// composeBindTarget returns the preset name the composition binds and the
// preset write that binding needs, if any. A composition equal to a current
// built-in default binds that name — keeping the standard balanced placement
// and the ultra effort ladder custom would lose — unless the operator
// customized that preset, whose values are never replaced. When the binding
// preset P stores efforts (SPEC-MODELUX-003 REQ-MUX3-20), only P's own
// built-in branch stays open, a historical refresh of which keeps them; any
// other result binds custom carrying P's efforts, so a built-in never takes
// efforts the operator did not put there.
func composeBindTarget(quality config.QualityConf, edited qualityComposition) (string, *composePresetWrite) {
	bound := quality.EffectiveMode("")
	carried := qualityViewEfforts(quality, "")
	for _, name := range composeBuiltinPresets {
		current := composeCurrentPreset(name)
		if !maps.Equal(edited.tiers, current.Agents) {
			continue
		}
		if len(carried) > 0 && name != bound {
			break
		}
		// The built-in defaults differ from each other, so at most one matches.
		status, _ := config.ClassifyBuiltinQualityPreset(quality, name)
		switch status.Class {
		case config.BuiltinQualityPresetCurrent:
			return name, nil
		case config.BuiltinQualityPresetHistorical:
			return name, &composePresetWrite{
				Name: name, Kind: composePresetRefresh, Preset: current,
				From: maps.Clone(quality.Presets[name].Agents), expect: status.Class,
			}
		case config.BuiltinQualityPresetAbsent:
			return name, &composePresetWrite{Name: name, Kind: composePresetNew, Preset: current, expect: status.Class}
		}
	}
	kind := composePresetNew
	if _, exists := quality.Presets[composeCustomPreset]; exists {
		kind = composePresetOverwrite
		if bound == composeCustomPreset {
			kind = composePresetRewrite
		}
	}
	write := &composePresetWrite{
		Name: composeCustomPreset, Kind: kind,
		Preset: config.QualityPreset{
			Description: composeCustomDescription, Agents: edited.Agents(), Efforts: presetEfforts(carried),
		},
	}
	if bound != composeCustomPreset {
		write.Carried = len(carried)
	}
	return composeCustomPreset, write
}

// composeGivenUpMode names the built-in mode a custom binding replaces.
func composeGivenUpMode(quality config.QualityConf) string {
	switch mode := quality.EffectiveMode(""); mode {
	case "balanced", "ultra":
		return mode
	}
	return ""
}

// planComposeRefresh plans `u` (REQ-MUX-11): every historical built-in preset
// takes the agents and description of the current code default, while
// quality.default stays and a customized or absent preset is left alone. ok
// is false when no preset is historical, which is when `u` is not offered.
func planComposeRefresh(cfg *config.HarnessConfig) (composeBindingPlan, bool) {
	var plan composeBindingPlan
	for _, name := range composeBuiltinPresets {
		status, _ := config.ClassifyBuiltinQualityPreset(cfg.Quality, name)
		if status.Class != config.BuiltinQualityPresetHistorical {
			continue
		}
		plan.Presets = append(plan.Presets, composePresetWrite{
			Name: name, Kind: composePresetUpdate, Preset: composeCurrentPreset(name),
			From:   maps.Clone(cfg.Quality.Presets[name].Agents),
			expect: status.Class, runtimeStandard: status.RuntimeStandard,
		})
	}
	if len(plan.Presets) == 0 {
		return composeBindingPlan{}, false
	}
	plan.Writer = composeWriterBlock
	plan.Candidate = composeCandidate(cfg, plan)
	plan.markModelChanges()
	return plan, true
}

// markModelChanges decides for each `u` refresh whether it moves a model once
// the whole plan is written: only a preset that then feeds an installed
// column does, and a historical balanced layout the runtime already keeps on
// the standard placement does not.
func (p *composeBindingPlan) markModelChanges() {
	for i, write := range p.Presets {
		if write.Kind == composePresetUpdate {
			p.Presets[i].ModelChange = composePresetFeedsTable(p.Candidate, write.Name) &&
				!(write.Name == "balanced" && write.runtimeStandard)
		}
	}
}

// composePresetFeedsTable reports whether a preset feeds an installed column:
// a native column whose effective mode names it, or, for ultra, the built-in
// OMP ultra profile, which reads quality.presets.ultra.
func composePresetFeedsTable(cfg *config.HarnessConfig, name string) bool {
	for platform, provider := range map[string]string{
		"claude-code": config.QualityProviderClaude,
		"codex":       config.QualityProviderCodex,
	} {
		if containsPlatform(cfg.Platforms, platform) && cfg.Quality.EffectiveMode(provider) == name {
			return true
		}
	}
	if name != "ultra" || !containsPlatform(cfg.Platforms, "omp") || cfg.RoleModelPolicy.Profile != "ultra" {
		return false
	}
	_, _, defined := cfg.RoleModelPolicy.SelectedRoleModelProfile()
	return !defined
}

// composeCurrentPreset returns a fresh copy of a built-in preset's current
// code default, the single source of the current defaults.
func composeCurrentPreset(name string) config.QualityPreset {
	return config.DefaultFullConfig("").Quality.Presets[name]
}
