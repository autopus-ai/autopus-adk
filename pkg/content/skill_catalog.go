package content

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SkillVisibilityShared       = "shared"
	SkillVisibilityCodexOnly    = "codex-only"
	SkillVisibilityOpenCodeOnly = "opencode-only"
	SkillVisibilityClaudeOnly   = "claude-only"
	SkillVisibilityExplicitOnly = "explicit-only"
)

var canonicalSkillRefRe = regexp.MustCompile(`\.claude/skills/autopus/([a-z0-9-]+)\.md`)

// CatalogSkill is the registry-layer representation of a canonical skill.
type CatalogSkill struct {
	Name           string
	Description    string
	Category       string
	SourcePath     string
	Bundles        []string
	Visibility     string
	CompileTargets []string
	Dependencies   []string
}

// SkillCatalog stores canonical skill metadata separately from compiled output.
type SkillCatalog struct {
	skills map[string]CatalogSkill
}

// LoadSkillCatalog loads canonical skill metadata from a directory on disk.
func LoadSkillCatalog(dir string) (*SkillCatalog, error) {
	return loadSkillCatalog(dir, os.ReadDir, os.ReadFile)
}

// LoadSkillCatalogFromFS loads canonical skill metadata from an embedded filesystem.
func LoadSkillCatalogFromFS(fsys fs.FS, dir string) (*SkillCatalog, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read skill catalog dir %s: %w", dir, err)
	}

	catalog := &SkillCatalog{skills: make(map[string]CatalogSkill, len(entries))}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.ToSlash(filepath.Join(dir, entry.Name()))
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("read skill catalog file %s: %w", path, err)
		}

		skill, err := buildCatalogSkill(path, data)
		if err != nil {
			return nil, err
		}
		catalog.skills[skill.Name] = skill
	}

	return catalog, nil
}

func loadSkillCatalog(
	dir string,
	readDir func(string) ([]os.DirEntry, error),
	readFile func(string) ([]byte, error),
) (*SkillCatalog, error) {
	entries, err := readDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read skill catalog dir %s: %w", dir, err)
	}

	catalog := &SkillCatalog{skills: make(map[string]CatalogSkill, len(entries))}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := readFile(path)
		if err != nil {
			return nil, fmt.Errorf("read skill catalog file %s: %w", path, err)
		}

		skill, err := buildCatalogSkill(filepath.ToSlash(path), data)
		if err != nil {
			return nil, err
		}
		catalog.skills[skill.Name] = skill
	}

	return catalog, nil
}

func buildCatalogSkill(path string, data []byte) (CatalogSkill, error) {
	parsed, err := parseSkillMeta(data, filepath.Base(path))
	if err != nil {
		return CatalogSkill{}, fmt.Errorf("parse skill catalog %s: %w", path, err)
	}

	return CatalogSkill{
		Name:           parsed.meta.Name,
		Description:    parsed.meta.Description,
		Category:       parsed.meta.Category,
		SourcePath:     path,
		Bundles:        coalesceStringSlice(parsed.meta.Bundles, bundlesForSkill(parsed.meta.Name, parsed.meta.Category)),
		Visibility:     coalesceString(parsed.meta.Visibility, visibilityForSkill(parsed.meta.Name)),
		CompileTargets: coalesceStringSlice(parsed.meta.Platforms, compileTargetsForSkill(parsed.meta.Name)),
		Dependencies:   dependenciesFromBody(parsed.body),
	}, nil
}

// dependenciesFromBody lists every canonical skill the body links to, sorted so
// catalog output stays deterministic. It sees both the legacy flat form and the
// installed <name>/SKILL.md directory form, because a compacted default surface
// must pin whichever one an always-installed body actually uses.
func dependenciesFromBody(body string) []string {
	deps := skillReferencesFromBody(body)
	if len(deps) == 0 {
		return nil
	}
	sort.Strings(deps)
	return deps
}

// Get returns a catalog skill by name.
func (c *SkillCatalog) Get(name string) (CatalogSkill, bool) {
	if c == nil {
		return CatalogSkill{}, false
	}
	skill, ok := c.skills[name]
	return skill, ok
}

// List returns all catalog skills in deterministic order.
func (c *SkillCatalog) List() []CatalogSkill {
	if c == nil {
		return nil
	}

	names := make([]string, 0, len(c.skills))
	for name := range c.skills {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]CatalogSkill, 0, len(names))
	for _, name := range names {
		out = append(out, c.skills[name])
	}
	return out
}

func coalesceString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func coalesceStringSlice(value, fallback []string) []string {
	if len(value) == 0 {
		return fallback
	}
	return value
}
