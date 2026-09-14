package codex

import (
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

func shouldEmitCodexRepoSkillTemplate(skillFile string, cfg *config.HarnessConfig) (bool, error) {
	if cfg == nil || cfg.Skills.Compiler.EffectiveMode() == config.SkillCompilerModeFull {
		return true, nil
	}

	name := strings.TrimSuffix(skillFile, ".md")
	if name == "" || strings.HasPrefix(name, "auto-") {
		return true, nil
	}

	catalog, err := pkgcontent.LoadSkillCatalogFromFS(contentfs.FS, "skills")
	if err != nil {
		return false, err
	}
	entry, ok := catalog.Get(name)
	if !ok {
		return true, nil
	}
	// A catalog entry scoped away from Codex never compiles here and has no
	// long-tail sink either, so this repo template is the only Codex-native
	// source for it. Gating it on catalog placement would delete the file the
	// generated route bodies point at.
	if !codexIsCompileTarget(entry) {
		return true, nil
	}

	state := pkgcontent.ResolveCatalogSkillState(entry, "codex", cfg)
	return filepath.ToSlash(state.TargetPath) == filepath.ToSlash(codexProjectSkillPath(name)), nil
}

func codexIsCompileTarget(entry pkgcontent.CatalogSkill) bool {
	for _, target := range entry.CompileTargets {
		if target == "codex" {
			return true
		}
	}
	return false
}
