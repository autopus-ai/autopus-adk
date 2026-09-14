package content

import (
	"io/fs"
	"regexp"
	"strings"
	"sync"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/templates"
)

// skillRefRe matches every managed reference form that names a canonical skill:
// the legacy flat `.claude/skills/autopus/<name>.md` form and the installed
// `.claude/skills/<name>/SKILL.md` directory form. Reference rewriting still
// uses canonicalSkillRefRe alone; this wider pattern exists only to discover
// which skills an always-installed surface depends on.
var skillRefRe = regexp.MustCompile(`\.claude/skills/(?:autopus/([a-z0-9-]+)\.md|([a-z0-9-]+)/SKILL\.md)`)

// pinnedSurfaceDirs are the embedded trees that ship on every install. A skill
// any of them links to must stay installed, or the compacted default surface
// would leave a reference pointing at a file no adapter wrote.
var pinnedSurfaceDirs = []string{"agents", "rules"}

var (
	pinnedSkillsOnce sync.Once
	pinnedSkills     map[string]bool
)

// pinnedSkillDependencies returns the skills that are reachable from an
// always-installed surface and therefore cannot be compacted away.
func pinnedSkillDependencies() map[string]bool {
	pinnedSkillsOnce.Do(func() {
		pinnedSkills = computePinnedSkills(contentfs.FS)
	})
	return pinnedSkills
}

var (
	embeddedCatalogOnce sync.Once
	embeddedCatalog     *SkillCatalog
	embeddedCatalogErr  error
)

// embeddedSkillCatalog returns the built-in catalog, parsed once per process.
// Callers that only need reference resolution should not re-read and re-parse
// every embedded skill on each lookup.
func embeddedSkillCatalog() (*SkillCatalog, error) {
	embeddedCatalogOnce.Do(func() {
		embeddedCatalog, embeddedCatalogErr = LoadSkillCatalogFromFS(contentfs.FS, "skills")
	})
	return embeddedCatalog, embeddedCatalogErr
}

// computePinnedSkills walks the always-installed surfaces, collects the skills
// they reference, and follows those skills' own references transitively.
//
// It deliberately consults coreSkillSet and IsRouteSkill directly rather than
// IsCoreSkill: IsCoreSkill reads the result of this computation, and routing it
// back through here would deadlock the sync.Once that guards the cache.
func computePinnedSkills(fsys fs.FS) map[string]bool {
	deps := skillReferenceGraph(fsys)

	frontier := make([]string, 0, len(deps))
	for name, refs := range deps {
		if coreSkillSet[name] || IsRouteSkill(name) {
			frontier = append(frontier, refs...)
		}
	}
	for _, dir := range pinnedSurfaceDirs {
		frontier = append(frontier, referencesInDir(fsys, dir)...)
	}
	frontier = append(frontier, referencesInTemplates()...)

	pinned := make(map[string]bool)
	for len(frontier) > 0 {
		name := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		if pinned[name] || coreSkillSet[name] || IsRouteSkill(name) {
			continue
		}
		if _, known := deps[name]; !known {
			// A reference to something the catalog does not ship cannot be
			// satisfied by pinning; leave it for the reference rewriter.
			continue
		}
		pinned[name] = true
		frontier = append(frontier, deps[name]...)
	}
	return pinned
}

// skillReferenceGraph maps each canonical skill to the skills its body links to.
func skillReferenceGraph(fsys fs.FS) map[string][]string {
	entries, err := fs.ReadDir(fsys, "skills")
	if err != nil {
		return map[string][]string{}
	}

	graph := make(map[string][]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := fs.ReadFile(fsys, "skills/"+entry.Name())
		if err != nil {
			continue
		}
		parsed, err := parseSkillMeta(data, entry.Name())
		if err != nil {
			continue
		}
		graph[parsed.meta.Name] = skillReferencesFromBody(parsed.body)
	}
	return graph
}

// referencesInDir collects skill references from every markdown file in dir.
func referencesInDir(fsys fs.FS, dir string) []string {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}

	var refs []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := fs.ReadFile(fsys, dir+"/"+entry.Name())
		if err != nil {
			continue
		}
		refs = append(refs, skillReferencesFromBody(string(data))...)
	}
	return refs
}

// referencesInTemplates collects skill references from the generated command,
// route, and rule templates. A `/auto` route body that inlines a skill file is
// installed on every project, so the skill it names has to be installed too.
func referencesInTemplates() []string {
	var refs []string
	_ = fs.WalkDir(templates.FS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}
		data, readErr := fs.ReadFile(templates.FS, path)
		if readErr != nil {
			return nil
		}
		refs = append(refs, skillReferencesFromBody(string(data))...)
		return nil
	})
	return refs
}

// skillReferencesFromBody returns the distinct canonical skill names a body
// links to, in first-seen order.
func skillReferencesFromBody(body string) []string {
	matches := skillRefRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(matches))
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		name := match[1]
		if name == "" {
			name = match[2]
		}
		if name == "" || name == "autopus" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}
