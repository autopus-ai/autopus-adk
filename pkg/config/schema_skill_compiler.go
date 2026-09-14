package config

import "fmt"

const (
	SkillCompilerModeFull  = "full"
	SkillCompilerModeSplit = "split"

	SkillLongTailTargetShared  = "shared"
	SkillLongTailTargetProject = "project"
	SkillLongTailTargetRepo    = "repo"
	SkillLongTailTargetPlugin  = "plugin"
)

// SkillCompilerConf controls the opt-in surface compiler split behavior.
type SkillCompilerConf struct {
	Mode                   string   `yaml:"mode,omitempty"`
	Bundles                []string `yaml:"bundles,omitempty"`
	ExplicitSkills         []string `yaml:"explicit_skills,omitempty"`
	OpenCodeLongTailTarget string   `yaml:"opencode_long_tail_target,omitempty"`
	CodexLongTailTarget    string   `yaml:"codex_long_tail_target,omitempty"`
}

// EffectiveMode returns the normalized compiler mode.
//
// An unset mode resolves to split: a fresh install advertises the core skill
// surface plus the command routes, and reaches the rest of the library through
// bundles, explicit_skills, or mode: full. Writing "full" restores the legacy
// behavior of compiling every reusable skill onto every native surface.
func (c SkillCompilerConf) EffectiveMode() string {
	if c.Mode == "" {
		return SkillCompilerModeSplit
	}
	return c.Mode
}

// EffectiveOpenCodeLongTailTarget returns the normalized OpenCode long-tail target.
func (c SkillCompilerConf) EffectiveOpenCodeLongTailTarget() string {
	if c.OpenCodeLongTailTarget == "" {
		return SkillLongTailTargetProject
	}
	return c.OpenCodeLongTailTarget
}

// EffectiveCodexLongTailTarget returns the normalized Codex long-tail target.
func (c SkillCompilerConf) EffectiveCodexLongTailTarget() string {
	if c.CodexLongTailTarget == "" {
		return SkillLongTailTargetPlugin
	}
	return c.CodexLongTailTarget
}

func (c *HarnessConfig) validateSkillsConfig() error {
	if c.Skills.MaxActiveSkills < 0 {
		return fmt.Errorf("skills.max_active_skills must be non-negative, got %d", c.Skills.MaxActiveSkills)
	}
	switch c.Skills.EffectiveSharedSurface() {
	case SharedSurfaceAuto, SharedSurfaceFull, SharedSurfaceCore:
	default:
		return fmt.Errorf("skills.shared_surface %q is invalid: must be 'auto', 'full', or 'core'", c.Skills.SharedSurface)
	}
	switch c.Skills.Compiler.EffectiveMode() {
	case SkillCompilerModeFull, SkillCompilerModeSplit:
	default:
		return fmt.Errorf("skills.compiler.mode %q is invalid: must be 'full' or 'split'", c.Skills.Compiler.Mode)
	}
	switch c.Skills.Compiler.EffectiveOpenCodeLongTailTarget() {
	case SkillLongTailTargetShared, SkillLongTailTargetProject:
	default:
		return fmt.Errorf("skills.compiler.opencode_long_tail_target %q is invalid: must be 'shared' or 'project'", c.Skills.Compiler.OpenCodeLongTailTarget)
	}
	switch c.Skills.Compiler.EffectiveCodexLongTailTarget() {
	case SkillLongTailTargetRepo, SkillLongTailTargetPlugin:
	default:
		return fmt.Errorf("skills.compiler.codex_long_tail_target %q is invalid: must be 'repo' or 'plugin'", c.Skills.Compiler.CodexLongTailTarget)
	}
	return nil
}
