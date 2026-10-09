// Package content provides skill transformation for multi-platform deployment.
package content

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillMeta holds parsed frontmatter metadata for platform compatibility checks.
type SkillMeta struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Platforms   []string `yaml:"platforms"`
	Triggers    []string `yaml:"triggers"`
	Category    string   `yaml:"category"`
	Bundles     []string `yaml:"bundles"`
	Visibility  string   `yaml:"visibility"`
}

// TransformedSkill represents a skill after platform transformation.
type TransformedSkill struct {
	Name        string
	Description string
	Content     string
}

// TransformReport summarizes which skills are compatible or not for a platform.
type TransformReport struct {
	Platform     string
	Compatible   []string
	Incompatible []string
}

// FormatTransformReport returns the user-facing platform transformation summary.
func FormatTransformReport(report *TransformReport) string {
	if report == nil {
		return ""
	}

	summary := fmt.Sprintf("  [%s] extended skills: %d compatible, %d platform-skipped",
		report.Platform, len(report.Compatible), len(report.Incompatible))
	if len(report.Incompatible) == 0 {
		return summary
	}

	skipped := append([]string(nil), report.Incompatible...)
	sort.Strings(skipped)
	return fmt.Sprintf("%s (%s)", summary, strings.Join(skipped, ", "))
}

// SkillTransformOptions controls optional canonical reference rewrites.
type SkillTransformOptions struct {
	ResolveSkillRef func(name string) string
	AllowSkill      func(meta SkillMeta) bool
}

// SkillTransformer loads skill files and transforms them for target platforms.
type SkillTransformer struct {
	skills []parsedSkill
}

// parsedSkill holds both metadata and raw content of a skill file.
type parsedSkill struct {
	meta SkillMeta
	body string
}

// supportedPlatforms lists the platforms the transformer can target.
var supportedPlatforms = map[string]bool{
	"claude":          true,
	"claude-code":     true,
	"codex":           true,
	"gemini":          true,
	"gemini-cli":      true,
	"antigravity-cli": true,
	"opencode":        true,
	"omp":             true,
}

// NewSkillTransformer creates a transformer by loading all .md files from dir.
func NewSkillTransformer(dir string) (*SkillTransformer, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return &SkillTransformer{}, nil
	}

	var skills []parsedSkill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read skill file %s: %w", path, err)
		}

		ps, err := parseSkillMeta(data, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("parse skill file %s: %w", path, err)
		}
		skills = append(skills, ps)
	}

	return &SkillTransformer{skills: skills}, nil
}

// TransformForPlatform returns transformed skills and a report for the target platform.
func (t *SkillTransformer) TransformForPlatform(platform string) ([]TransformedSkill, *TransformReport, error) {
	return t.TransformForPlatformWithOptions(platform, SkillTransformOptions{})
}

// TransformForPlatformWithOptions returns transformed skills and a report for the target platform.
func (t *SkillTransformer) TransformForPlatformWithOptions(platform string, opts SkillTransformOptions) ([]TransformedSkill, *TransformReport, error) {
	if !supportedPlatforms[platform] {
		return nil, nil, fmt.Errorf("unsupported platform: %q", platform)
	}

	report := &TransformReport{Platform: platform}
	var result []TransformedSkill

	for _, s := range t.skills {
		if !IsCompatible(s.meta, platform) {
			report.Incompatible = append(report.Incompatible, s.meta.Name)
			continue
		}
		if s.meta.Visibility == SkillVisibilityExplicitOnly && (opts.AllowSkill == nil || !opts.AllowSkill(s.meta)) {
			report.Incompatible = append(report.Incompatible, s.meta.Name)
			continue
		}

		report.Compatible = append(report.Compatible, s.meta.Name)
		body := s.body
		native, ok, err := nativeSkillBody(s.meta.Name, platform)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			body = native
		}
		filtered := rewriteCanonicalSkillReferences(body, opts.ResolveSkillRef)
		filtered = ReplacePlatformReferences(filtered, platform)
		result = append(result, TransformedSkill{
			Name:        s.meta.Name,
			Description: s.meta.Description,
			Content:     filtered,
		})
	}

	return result, report, nil
}

// IsCompatible checks whether a skill is compatible with the given platform.
// If platforms is empty/nil, the skill is compatible with all platforms (R6).
func IsCompatible(meta SkillMeta, platform string) bool {
	if len(meta.Platforms) == 0 {
		return true
	}
	for _, p := range meta.Platforms {
		if p == platform || (p == "claude" && platform == "claude-code") {
			return true
		}
	}
	return false
}

// NewSkillTransformerFromFS creates a transformer by loading .md files from an embedded FS.
func NewSkillTransformerFromFS(fsys fs.FS, dir string) (*SkillTransformer, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return &SkillTransformer{}, nil
	}

	var skills []parsedSkill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := fs.ReadFile(fsys, dir+"/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read skill file %s: %w", entry.Name(), err)
		}

		ps, err := parseSkillMeta(data, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("parse skill file %s: %w", entry.Name(), err)
		}
		skills = append(skills, ps)
	}

	return &SkillTransformer{skills: skills}, nil
}

// parseSkillMeta extracts frontmatter metadata and body from raw skill data.
func parseSkillMeta(data []byte, filename string) (parsedSkill, error) {
	raw := string(data)
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return parsedSkill{}, fmt.Errorf("frontmatter split: %w", err)
	}

	var meta SkillMeta
	if fm != "" {
		if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
			return parsedSkill{}, fmt.Errorf("yaml parse: %w", err)
		}
	}

	if meta.Name == "" {
		meta.Name = strings.TrimSuffix(filename, ".md")
	}

	return parsedSkill{meta: meta, body: strings.TrimSpace(body)}, nil
}
