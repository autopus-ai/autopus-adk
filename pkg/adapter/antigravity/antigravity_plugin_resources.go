package antigravity

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// antigravityPluginResourceDir is the skill subdirectory `agy` documents for
// bulky reference material. Keeping it beside SKILL.md is what makes
// progressive disclosure work: the CLI injects only name and description, and
// the agent opens a reference file when the linked step needs it.
const antigravityPluginResourceDir = "references"

// preparePluginSkillResources emits references beside each installed native and
// legacy Gemini skill entrypoint. Neither copy is a separately discoverable skill.
func preparePluginSkillResources(
	files []adapter.FileMapping,
	cfg *config.HarnessConfig,
) ([]adapter.FileMapping, error) {
	names := pluginSkillNames(files)
	resources := make([]adapter.FileMapping, 0, len(names))
	for _, name := range names {
		rendered, err := pkgcontent.RenderSkillResources(name, "gemini", cfg)
		if err != nil {
			return nil, fmt.Errorf("antigravity 스킬 리소스 렌더링 실패 %s: %w", name, err)
		}
		if len(rendered) == 0 {
			continue
		}
		legacyDir := filepath.Join(".gemini", "skills", "autopus", name)
		legacyInstalled := false
		for _, file := range files {
			if filepath.ToSlash(file.TargetPath) == filepath.ToSlash(filepath.Join(legacyDir, "SKILL.md")) {
				legacyInstalled = true
				break
			}
		}
		relatives := make([]string, 0, len(rendered))
		for relative := range rendered {
			relatives = append(relatives, relative)
		}
		sort.Strings(relatives)
		for _, relative := range relatives {
			target, err := pluginResourceTarget(name, relative)
			if err != nil {
				return nil, err
			}
			body := rewriteAntigravityPluginContent(string(rendered[relative]))
			resources = append(resources, adapter.FileMapping{
				TargetPath:      target,
				OverwritePolicy: adapter.OverwriteAlways,
				Checksum:        checksum(body),
				Content:         []byte(body),
			})
			if legacyInstalled {
				raw := rendered[relative]
				resources = append(resources, adapter.FileMapping{
					TargetPath:      filepath.Join(legacyDir, filepath.FromSlash(relative)),
					OverwritePolicy: adapter.OverwriteAlways,
					Checksum:        checksum(string(raw)),
					Content:         raw,
				})
			}
		}
	}
	return resources, nil
}

// pluginResourceTarget refuses any key that would land outside the skill's
// own references directory. The catalog promises clean relative keys; this
// keeps a future regression there from writing through the plugin boundary.
func pluginResourceTarget(skill, relative string) (string, error) {
	clean := path.Clean(filepath.ToSlash(relative))
	if !strings.HasPrefix(clean, antigravityPluginResourceDir+"/") ||
		strings.Contains(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf(
			"antigravity 스킬 리소스 경로가 %s/ 밖을 가리킴 %s: %s",
			antigravityPluginResourceDir, skill, relative)
	}
	return filepath.FromSlash(path.Join(antigravityPluginDir, "skills", skill, clean)), nil
}

// pluginSkillNames lists the skill directories already staged in the plugin,
// in deterministic order, so resource emission never depends on map ordering.
func pluginSkillNames(files []adapter.FileMapping) []string {
	prefix := antigravityPluginDir + "/skills/"
	seen := make(map[string]bool, len(files))
	names := make([]string, 0, len(files))
	for _, file := range files {
		target := filepath.ToSlash(file.TargetPath)
		if !strings.HasPrefix(target, prefix) || path.Base(target) != "SKILL.md" {
			continue
		}
		name := path.Base(path.Dir(target))
		if name == "" || name == "." || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
