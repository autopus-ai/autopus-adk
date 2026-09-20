package content

import "github.com/insajin/autopus-adk/pkg/config"

// SkillSelection explains compiler policy, not actual runtime loading.
type SkillSelection struct {
	Reason string            `json:"reason"`
	State  SkillSurfaceState `json:"state"`
}

// ExplainSkillSelection describes the current compiler decision without changing it.
func ExplainSkillSelection(skill CatalogSkill, platform string, cfg *config.HarnessConfig) SkillSelection {
	if cfg == nil {
		cfg = &config.HarnessConfig{}
	}
	state := ResolveCatalogSkillState(skill, platform, cfg)
	reason := "not_selected"
	normalized := normalizeCatalogPlatform(platform)
	switch {
	case normalized == "" || !containsString(skill.CompileTargets, normalized) || !visibilityAllowsPlatform(skill.Visibility, normalized):
		reason = "unsupported"
	case !state.Compiled:
	case containsString(cfg.Skills.Compiler.ExplicitSkills, skill.Name):
		reason = "opt_in"
	case coreSkillSet[skill.Name]:
		reason = "core"
	case IsRouteSkill(skill.Name):
		reason = "route"
	case pinnedSkillDependencies()[skill.Name]:
		reason = "dependency"
	default:
		reason = "opt_in"
	}
	return SkillSelection{Reason: reason, State: state}
}
