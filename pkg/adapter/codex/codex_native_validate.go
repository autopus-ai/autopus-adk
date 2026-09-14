package codex

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

func (a *Adapter) validateNativeCodexSurface(errs *[]adapter.ValidationError) {
	manifest, _ := adapter.LoadManifest(a.root, adapterName)
	expectedSkills := []string{codexProjectSkillPath("auto")}
	if manifest != nil {
		expectedSkills = expectedSkills[:0]
		for path := range manifest.Files {
			if strings.HasPrefix(filepath.ToSlash(path), ".codex/skills/") {
				expectedSkills = append(expectedSkills, path)
			}
		}
	}
	for _, path := range expectedSkills {
		validateNativeCodexSkillPath(a.root, path, errs)
	}

	for _, path := range []string{
		filepath.Join(".codex", "agents", "executor.toml"),
		filepath.Join(".codex", "hooks.json"),
		filepath.Join(".autopus", "plugins", "auto", ".codex-plugin", "plugin.json"),
	} {
		if info, err := os.Stat(filepath.Join(a.root, path)); err != nil || !info.Mode().IsRegular() {
			*errs = append(*errs, adapter.ValidationError{
				File: path, Message: "Codex native managed surface가 없거나 regular file이 아님", Level: "error",
			})
		}
	}
	validateObsoleteCodexSurface(a.root, a.openCodeOwnsRootDoc(), errs)
}

// validateNativeCodexSkillPath routes a manifest path under .codex/skills to
// the check that matches its role. An entrypoint must obey the native
// codex-*/SKILL.md contract; a reference body installed beside one is a
// legitimate resource, not a malformed entrypoint, and gets its own check.
func validateNativeCodexSkillPath(root, path string, errs *[]adapter.ValidationError) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) == 5 && parts[0] == ".codex" && parts[1] == "skills" &&
		strings.HasPrefix(parts[2], codexSkillDirPrefix) &&
		parts[3] == codexSkillResourceDir && strings.HasSuffix(parts[4], ".md") {
		validateNativeCodexSkillResource(root, parts[2], path, errs)
		return
	}
	validateNativeCodexSkill(root, path, errs)
}

// validateNativeCodexSkillResource keeps a resource honest without applying the
// entrypoint naming rule to it: the body must be readable, and the skill it
// belongs to must actually have an installed entrypoint, so a pruned skill can
// never leave its references behind as an orphaned surface.
func validateNativeCodexSkillResource(root, skillDir, path string, errs *[]adapter.ValidationError) {
	entrypoint := filepath.Join(".codex", "skills", skillDir, "SKILL.md")
	if info, err := os.Stat(filepath.Join(root, entrypoint)); err != nil || !info.Mode().IsRegular() {
		*errs = append(*errs, adapter.ValidationError{
			File: path, Message: "Codex skill reference에 대응하는 native SKILL.md entrypoint가 없음", Level: "error",
		})
		return
	}
	if info, err := os.Stat(filepath.Join(root, path)); err != nil || !info.Mode().IsRegular() {
		*errs = append(*errs, adapter.ValidationError{
			File: path, Message: "Codex skill reference를 읽을 수 없음", Level: "error",
		})
	}
}

func validateNativeCodexSkill(root, path string, errs *[]adapter.ValidationError) {
	clean := filepath.ToSlash(path)
	parts := strings.Split(clean, "/")
	if len(parts) != 4 || parts[0] != ".codex" || parts[1] != "skills" ||
		!strings.HasPrefix(parts[2], "codex-") || parts[3] != "SKILL.md" {
		*errs = append(*errs, adapter.ValidationError{
			File: path, Message: "Codex skill이 native codex-*/SKILL.md layout이 아님", Level: "error",
		})
		return
	}
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		*errs = append(*errs, adapter.ValidationError{
			File: path, Message: "Codex native skill을 읽을 수 없음", Level: "error",
		})
		return
	}
	if !strings.Contains(string(data), "name: "+parts[2]) {
		*errs = append(*errs, adapter.ValidationError{
			File: path, Message: "Codex native skill name이 directory와 일치하지 않음", Level: "error",
		})
	}
}

// validateObsoleteCodexSurface reports every retired Codex surface the detector
// names. Detection itself lives in obsoleteCodexSurfacePaths so the update
// prune removes exactly what this report claims is obsolete.
func validateObsoleteCodexSurface(root string, openCodeOwnsSharedSkills bool, errs *[]adapter.ValidationError) {
	for _, path := range obsoleteCodexSurfacePaths(root, openCodeOwnsSharedSkills) {
		*errs = append(*errs, adapter.ValidationError{
			File: path, Message: "obsolete Codex managed surface가 남아 있음", Level: "error",
		})
	}
}
