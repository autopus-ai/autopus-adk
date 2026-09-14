package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// skipDirs are directories excluded from context collection.
var skipDirs = map[string]bool{
	".git":         true,
	".cache":       true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	".autopus":     true,
	".claude":      true,
	"templates":    true,
	"__pycache__":  true,
	".next":        true,
	".nuxt":        true,
	"build":        true,
	"coverage":     true,
	".svelte-kit":  true,
}

var sourcePathPattern = regexp.MustCompile(`([A-Za-z0-9_./-]+\.(go|py|ts|js|rs|java|rb))`)

// CollectContext recursively reads source files from a directory up to maxLines total.
func CollectContext(dir string, maxLines int) (string, error) {
	var sb strings.Builder
	lineCount := 0

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if lineCount >= maxLines {
			return filepath.SkipAll
		}
		if !isSourceFile(d.Name()) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(dir, path)
		if relPath == "" {
			relPath = d.Name()
		}

		lines := strings.Split(string(content), "\n")
		remaining := maxLines - lineCount
		if remaining <= 0 {
			return filepath.SkipAll
		}

		fmt.Fprintf(&sb, "--- %s ---\n", relPath)
		lineCount++

		end := min(len(lines), remaining)
		for _, line := range lines[:end] {
			sb.WriteString(line)
			sb.WriteString("\n")
			lineCount++
		}
		sb.WriteString("\n")
		lineCount++
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	return sb.String(), nil
}

// CollectContextForSpec reads only the files explicitly referenced by the SPEC plan/research docs.
func CollectContextForSpec(projectRoot, specDir string, maxLines int) (string, error) {
	targets := extractSpecContextTargets(specDir)
	if len(targets) == 0 {
		return "", nil
	}

	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(specDir)))
	seen := make(map[string]bool, len(targets))
	var files []string
	for _, target := range targets {
		resolved := resolveSpecTargetPath(projectRoot, moduleRoot, target)
		if resolved == "" || seen[resolved] {
			continue
		}
		seen[resolved] = true
		files = append(files, resolved)
	}
	if len(files) == 0 {
		return "", nil
	}
	return collectFilesContext(projectRoot, files, maxLines)
}

func extractSpecContextTargets(specDir string) []string {
	var targets []string
	for _, name := range []string{"research.md", "plan.md"} {
		data, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil {
			continue
		}
		targets = append(targets, extractSourcePaths(string(data))...)
	}
	return uniqueStrings(targets)
}

func extractSourcePaths(markdown string) []string {
	var paths []string
	for _, line := range strings.Split(markdown, "\n") {
		for _, location := range sourcePathPattern.FindAllStringIndex(line, -1) {
			// The pattern accepts `/` but not `:`, so a cited URL arrives here
			// with its scheme cut off: `https://example.com/docs/guide.go`
			// matches from the slashes on, and normalizeSourcePath's own "://"
			// guard can no longer recognise it. Judge the match by what
			// precedes it: a colon before the leading slashes means a scheme.
			if strings.HasSuffix(strings.TrimRight(line[:location[0]], "/"), ":") {
				continue
			}
			if path := normalizeSourcePath(line[location[0]:location[1]]); path != "" {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func normalizeSourcePath(candidate string) string {
	candidate = strings.Trim(candidate, " \t`'\"[](){}")
	candidate = strings.TrimPrefix(candidate, "./")
	if candidate == "" || !isSourceFile(candidate) || strings.Contains(candidate, "://") {
		return ""
	}
	return filepath.Clean(candidate)
}

func resolveSpecTargetPath(projectRoot, moduleRoot, target string) string {
	for _, base := range []string{moduleRoot, projectRoot} {
		if base == "" {
			continue
		}
		path := target
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, target)
		}
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return filepath.Clean(path)
		}
	}
	return ""
}

func collectFilesContext(projectRoot string, files []string, maxLines int) (string, error) {
	var sb strings.Builder
	lineCount := 0
	for _, path := range files {
		if lineCount >= maxLines {
			break
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		relPath, err := filepath.Rel(projectRoot, path)
		if err != nil || strings.HasPrefix(relPath, "..") {
			relPath = filepath.Base(path)
		}
		lines := strings.Split(string(content), "\n")
		remaining := maxLines - lineCount
		if remaining <= 0 {
			break
		}
		fmt.Fprintf(&sb, "--- %s ---\n", relPath)
		lineCount++
		end := min(len(lines), remaining)
		for _, line := range lines[:end] {
			sb.WriteString(line)
			sb.WriteString("\n")
			lineCount++
		}
		sb.WriteString("\n")
		lineCount++
	}
	return sb.String(), nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// isSourceFile returns true if the filename is a recognized source file.
func isSourceFile(name string) bool {
	exts := []string{".go", ".py", ".ts", ".js", ".rs", ".java", ".rb"}
	for _, ext := range exts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}
