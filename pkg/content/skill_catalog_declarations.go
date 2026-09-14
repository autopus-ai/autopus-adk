package content

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

const skillsFrontmatterKey = "skills:"

// IsInstalledSkill reports whether the compiler actually writes this canonical
// skill to a file for the platform under the given configuration. An unknown
// name is treated as installed: only the catalog's own entries are subject to
// compiler placement, and a project-authored skill must never be stripped.
func IsInstalledSkill(name, platform string, cfg *config.HarnessConfig) bool {
	if cfg == nil || name == "" {
		return true
	}
	catalog, err := embeddedSkillCatalog()
	if err != nil {
		return true
	}
	skill, ok := catalog.Get(name)
	if !ok {
		return true
	}
	return ResolveCatalogSkillState(skill, platform, cfg).Compiled
}

// FilterDeclaredSkills rewrites the `skills:` frontmatter list of a generated
// agent contract so it names only skills the compiler installs for the
// platform.
//
// An agent may not declare a capability the installer did not write: the name
// resolves to nothing, and the agent behaves as if it had guidance it never
// received. Dropping the entry is the honest projection — the skill is still
// one `skills.compiler.explicit_skills` entry away from coming back, and the
// declaration returns with it. The key itself is removed when nothing survives,
// because an empty YAML list is not the same statement as no list.
func FilterDeclaredSkills(content, platform string, cfg *config.HarnessConfig) string {
	lines := strings.Split(content, "\n")
	end := frontmatterEnd(lines)
	if end < 0 {
		return content
	}

	out := make([]string, 0, len(lines))
	out = append(out, lines[0])
	for i := 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) != skillsFrontmatterKey {
			out = append(out, lines[i])
			continue
		}
		kept, next := filterDeclaredSkillItems(lines, i+1, end, platform, cfg)
		if len(kept) > 0 {
			out = append(out, lines[i])
			out = append(out, kept...)
		}
		i = next - 1
	}
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// frontmatterEnd returns the index of the closing `---`, or -1 when the content
// does not open with a frontmatter block.
func frontmatterEnd(lines []string) int {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return -1
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i
		}
	}
	return -1
}

// filterDeclaredSkillItems keeps the installed entries of one YAML sequence and
// returns the index of the first line after it.
func filterDeclaredSkillItems(
	lines []string,
	start, end int,
	platform string,
	cfg *config.HarnessConfig,
) ([]string, int) {
	kept := make([]string, 0, end-start)
	i := start
	for ; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "- ") {
			break
		}
		if IsInstalledSkill(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), platform, cfg) {
			kept = append(kept, lines[i])
		}
	}
	return kept, i
}
