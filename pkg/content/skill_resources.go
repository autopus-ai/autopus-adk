package content

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/config"
)

// skillResourceDir is the canonical source tree for skill reference bodies:
// content/skills/references/<skill-name>/<file>.md. Skill loaders skip
// directories, so nothing under it is ever registered as a skill of its own.
const skillResourceDir = "skills/references"

// SkillResourceDirName is the directory a rendered resource occupies next to
// the generated SKILL.md. Skill bodies address resources through this relative
// form, so the same link resolves on every platform, and platform validators
// use it to tell a legitimate resource apart from a malformed entrypoint.
const SkillResourceDirName = "references"

// RenderSkillResources returns the reference bodies that must be installed
// alongside a generated skill, keyed by their clean slash-relative path
// ("references/<file>.md") under the skill's own directory.
//
// Bodies go through the same platform rewriting as the skill itself, so a
// resource never leaks a Claude-native path onto a foreign surface. A skill
// with no reference tree yields a nil map and no error, which lets every
// emitter call this unconditionally inside its mapping loop.
func RenderSkillResources(name, platform string, cfg *config.HarnessConfig) (map[string][]byte, error) {
	if normalizeCatalogPlatform(platform) == "" {
		return nil, fmt.Errorf("render skill resources for %s: unsupported platform %q", name, platform)
	}
	// A skill name is a single embedded directory segment; anything else would
	// let a malformed catalog entry read outside the resource tree.
	if name == "" || strings.ContainsAny(name, `/\`) || name == ".." {
		return nil, nil
	}

	dir := path.Join(skillResourceDir, name)
	entries, err := fs.ReadDir(contentfs.FS, dir)
	if err != nil {
		return nil, nil
	}

	catalog, err := embeddedSkillCatalog()
	if err != nil {
		return nil, err
	}

	resources := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := fs.ReadFile(contentfs.FS, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read skill resource %s/%s: %w", dir, entry.Name(), err)
		}
		body := rewriteCanonicalSkillReferences(string(data), func(ref string) string {
			return ResolveCatalogSkillRefPath(catalog, ref, platform, cfg)
		})
		body = ReplacePlatformReferences(body, platform)
		resources[path.Join(SkillResourceDirName, entry.Name())] = []byte(body)
	}

	if len(resources) == 0 {
		return nil, nil
	}
	return resources, nil
}

// SkillResourceNames returns the resource keys for a skill in deterministic
// order, so emitters can produce stable file mappings and checksums.
func SkillResourceNames(resources map[string][]byte) []string {
	names := make([]string, 0, len(resources))
	for name := range resources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SkillResourceRelPaths returns the slash-relative resource keys a skill ships,
// in deterministic order and without rendering any body. Clean allowlists and
// ownership checks need the paths, not the content.
func SkillResourceRelPaths(name string) []string {
	if name == "" || strings.ContainsAny(name, `/\`) || name == ".." {
		return nil
	}
	entries, err := fs.ReadDir(contentfs.FS, path.Join(skillResourceDir, name))
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, path.Join(SkillResourceDirName, entry.Name()))
	}
	sort.Strings(names)
	return names
}
