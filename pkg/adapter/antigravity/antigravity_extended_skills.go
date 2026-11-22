package antigravity

import (
	"fmt"
	"path/filepath"

	"github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// renderExtendedSkills transforms embedded content skills for the Antigravity platform
// and returns file mappings for .gemini/skills/autopus/{skill-name}/SKILL.md.
func (a *Adapter) renderExtendedSkills(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	catalog, err := pkgcontent.LoadSkillCatalogFromFS(content.FS, "skills")
	if err != nil {
		return nil, fmt.Errorf("skill catalog init: %w", err)
	}
	transformer, err := pkgcontent.NewSkillTransformerFromFS(content.FS, "skills")
	if err != nil {
		return nil, fmt.Errorf("skill transformer init: %w", err)
	}

	skills, report, err := transformer.TransformForPlatformWithOptions("gemini", pkgcontent.SkillTransformOptions{
		ResolveSkillRef: func(name string) string {
			return pkgcontent.ResolveCatalogSkillRefPath(catalog, name, "gemini", cfg)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("skill transform for gemini: %w", err)
	}

	logTransformReport(report)

	var files []adapter.FileMapping
	for _, s := range skills {
		entry, ok := catalog.Get(s.Name)
		if !ok || !pkgcontent.ResolveCatalogSkillState(entry, "gemini", cfg).Compiled {
			continue
		}
		// Antigravity convention: each skill gets its own subdirectory
		relPath := filepath.Join(".gemini", "skills", "autopus", s.Name, "SKILL.md")
		files = append(files, adapter.FileMapping{
			TargetPath:      relPath,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(s.Content),
			Content:         []byte(s.Content),
		})
	}

	return files, nil
}

// logTransformReport prints a summary of skill transformation results.
func logTransformReport(report *pkgcontent.TransformReport) {
	// Diagnostics go to stderr so JSON consumers reading stdout stay parseable.
	pkgcontent.PrintDiagnosticOnce(pkgcontent.FormatTransformReport(report))
}
