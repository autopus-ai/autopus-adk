package content

import (
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// SkillSurfaceState separates registry, compile, and visibility state.
type SkillSurfaceState struct {
	Registered bool
	Compiled   bool
	Visible    bool
	TargetPath string
}

// ResolveCatalogSkillState evaluates registered/compiled/visible state for a skill.
func ResolveCatalogSkillState(skill CatalogSkill, platform string, cfg *config.HarnessConfig) SkillSurfaceState {
	targetPath := resolveCatalogSkillTarget(skill, platform, cfg)
	if targetPath == "" {
		return SkillSurfaceState{Registered: true}
	}

	compiled := shouldCompileCatalogSkill(skill, platform, cfg)
	return SkillSurfaceState{
		Registered: true,
		Compiled:   compiled,
		Visible:    compiled && isRepoVisibleSkillTarget(targetPath),
		TargetPath: targetPath,
	}
}

// ResolveCatalogSkillRefPath resolves platform-native skill references from the registry graph.
func ResolveCatalogSkillRefPath(catalog *SkillCatalog, name, platform string, cfg *config.HarnessConfig) string {
	if catalog != nil {
		if skill, ok := catalog.Get(name); ok {
			return resolveCatalogSkillTarget(skill, platform, cfg)
		}
	}
	return resolveDefaultSkillTarget(name, platform)
}

func shouldCompileCatalogSkill(skill CatalogSkill, platform string, cfg *config.HarnessConfig) bool {
	normalizedPlatform := normalizeCatalogPlatform(platform)
	if normalizedPlatform == "" || !containsString(skill.CompileTargets, normalizedPlatform) {
		return false
	}
	if !visibilityAllowsPlatform(skill.Visibility, normalizedPlatform) {
		return false
	}

	compiler := cfg.Skills.Compiler
	if skill.Visibility == SkillVisibilityExplicitOnly && !containsString(compiler.ExplicitSkills, skill.Name) {
		return false
	}
	if compiler.EffectiveMode() == config.SkillCompilerModeFull && normalizedPlatform == "opencode" && !sharedSurfaceIncludesSkill(skill, cfg) {
		return false
	}
	if containsString(compiler.ExplicitSkills, skill.Name) {
		return true
	}
	if IsCoreSkill(skill.Name) {
		return true
	}
	if len(compiler.Bundles) == 0 {
		// Compact default. Split mode without a bundle selection installs the
		// core surface only, on every platform: an uninstalled skill costs no
		// name/description slot in any native session prompt, and stays one
		// bundle or explicit_skills entry away from coming back. Full mode is
		// the documented opt-in that publishes the whole library everywhere.
		return compiler.EffectiveMode() == config.SkillCompilerModeFull
	}
	for _, bundle := range skill.Bundles {
		if containsString(compiler.Bundles, bundle) {
			return true
		}
	}
	return false
}

func sharedSurfaceIncludesSkill(skill CatalogSkill, cfg *config.HarnessConfig) bool {
	switch cfg.Skills.EffectiveSharedSurface() {
	case config.SharedSurfaceCore:
		return IsCoreSkill(skill.Name)
	case config.SharedSurfaceAuto:
		if containsString(cfg.Platforms, "codex") && containsString(cfg.Platforms, "opencode") {
			return IsCoreSkill(skill.Name)
		}
	}
	return true
}

func resolveCatalogSkillTarget(skill CatalogSkill, platform string, cfg *config.HarnessConfig) string {
	normalizedPlatform := normalizeCatalogPlatform(platform)
	if normalizedPlatform == "" {
		return ""
	}
	if cfg == nil {
		return resolveDefaultSkillTarget(skill.Name, normalizedPlatform)
	}
	if !shouldCompileCatalogSkill(skill, normalizedPlatform, cfg) {
		return ""
	}
	if cfg.Skills.Compiler.EffectiveMode() == config.SkillCompilerModeFull {
		return resolveDefaultSkillTarget(skill.Name, normalizedPlatform)
	}

	switch normalizedPlatform {
	case "opencode":
		if IsCoreSkill(skill.Name) || cfg.Skills.Compiler.EffectiveOpenCodeLongTailTarget() == config.SkillLongTailTargetShared {
			return filepath.ToSlash(filepath.Join(".agents", "skills", skill.Name, "SKILL.md"))
		}
		return filepath.ToSlash(filepath.Join(".opencode", "skills", skill.Name, "SKILL.md"))
	case "codex":
		if IsCoreSkill(skill.Name) || cfg.Skills.Compiler.EffectiveCodexLongTailTarget() == config.SkillLongTailTargetRepo {
			return filepath.ToSlash(filepath.Join(".codex", "skills", ResolveCatalogSkillOutputName(skill.Name, normalizedPlatform), "SKILL.md"))
		}
		return filepath.ToSlash(filepath.Join(".autopus", "plugins", "auto", "skills", skill.Name, "SKILL.md"))
	default:
		return resolveDefaultSkillTarget(skill.Name, normalizedPlatform)
	}
}

func resolveDefaultSkillTarget(name, platform string) string {
	switch normalizeCatalogPlatform(platform) {
	case "claude":
		return filepath.ToSlash(filepath.Join(".claude", "skills", name, "SKILL.md"))
	case "codex":
		return filepath.ToSlash(filepath.Join(".codex", "skills", ResolveCatalogSkillOutputName(name, platform), "SKILL.md"))
	case "gemini":
		return filepath.ToSlash(filepath.Join(".gemini", "skills", "autopus", name, "SKILL.md"))
	case "opencode":
		return filepath.ToSlash(filepath.Join(".agents", "skills", name, "SKILL.md"))
	case "omp":
		return filepath.ToSlash(filepath.Join(".omp", "skills", name, "SKILL.md"))
	default:
		return ""
	}
}

func normalizeCatalogPlatform(platform string) string {
	switch platform {
	case "claude", "claude-code":
		return "claude"
	case "gemini", "gemini-cli", "antigravity-cli":
		return "gemini"
	case "codex", "opencode", "omp":
		return platform
	default:
		return ""
	}
}

func visibilityAllowsPlatform(visibility, platform string) bool {
	switch visibility {
	case "", SkillVisibilityShared, SkillVisibilityExplicitOnly:
		return true
	case SkillVisibilityCodexOnly:
		return platform == "codex"
	case SkillVisibilityOpenCodeOnly:
		return platform == "opencode"
	case SkillVisibilityClaudeOnly:
		return platform == "claude"
	default:
		return false
	}
}

func isRepoVisibleSkillTarget(path string) bool {
	return !strings.HasPrefix(path, filepath.ToSlash(filepath.Join(".autopus", "plugins"))+"/")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
