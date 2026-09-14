package cli

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

var archSourceExtensions = map[string]bool{
	".c":     true,
	".cc":    true,
	".cjs":   true,
	".cpp":   true,
	".cs":    true,
	".css":   true,
	".cxx":   true,
	".go":    true,
	".h":     true,
	".hpp":   true,
	".java":  true,
	".js":    true,
	".jsx":   true,
	".kt":    true,
	".kts":   true,
	".less":  true,
	".mjs":   true,
	".php":   true,
	".py":    true,
	".rb":    true,
	".rs":    true,
	".sass":  true,
	".scss":  true,
	".sh":    true,
	".swift": true,
	".ts":    true,
	".tsx":   true,
	".vue":   true,
}

var archSkipDirs = map[string]bool{
	".agents":           true,
	".autopus":          true,
	".claude":           true,
	".codex":            true,
	".gemini":           true,
	".git":              true,
	".next":             true,
	".nuxt":             true,
	".opencode":         true,
	".output":           true,
	".svelte-kit":       true,
	".worktrees":        true,
	"build":             true,
	"coverage":          true,
	"dist":              true,
	"node_modules":      true,
	"playwright-report": true,
	"target":            true,
	"test-results":      true,
	"vendor":            true,
}

// checkArchStaged checks only git-staged source files for size limits.
func checkArchStaged(dir string, out io.Writer, quiet bool, limit int) bool {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACM")
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		// No git or no staged files — pass silently.
		return true
	}

	passed := true
	for _, rel := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if rel == "" {
			continue
		}
		if shouldSkipArchRel(rel) {
			continue
		}
		if !isArchSourceFile(rel) {
			continue
		}
		if isGeneratedSourceFile(filepath.Base(rel)) {
			continue
		}

		lines, err := countStagedLines(dir, rel)
		if err != nil {
			tui.FAIL(out, fmt.Sprintf("%s (line count failed: %v)", rel, err))
			passed = false
			continue
		}

		if !reportSourceSize(out, rel, lines, limit, quiet) {
			passed = false
		}
	}
	return passed
}

// checkArchWalk walks the directory tree checking all source files.
// Skips nested repositories, submodules, generated harness dirs, and worktree dirs.
func checkArchWalk(dir string, out io.Writer, quiet bool, limit int) bool {
	passed := true
	root := filepath.Clean(dir)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if archSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			// Skip nested Git repositories. Submodules usually contain a .git file,
			// while sibling repos in a meta workspace contain a .git directory.
			if filepath.Clean(path) != root {
				gitPath := filepath.Join(path, ".git")
				if _, statErr := os.Lstat(gitPath); statErr == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !isArchSourceFile(path) {
			return nil
		}
		if isGeneratedSourceFile(d.Name()) {
			return nil
		}

		lines, countErr := countLines(path)
		if countErr != nil {
			return countErr
		}

		rel, _ := filepath.Rel(dir, path)
		if !reportSourceSize(out, rel, lines, limit, quiet) {
			passed = false
		}
		return nil
	})

	if err != nil {
		tui.Error(out, fmt.Sprintf("arch check error: %v", err))
		return false
	}
	return passed
}

func reportSourceSize(out io.Writer, path string, lines, limit int, quiet bool) bool {
	if limit > 0 && lines > limit {
		tui.FAIL(out, fmt.Sprintf("%s (%d code lines — exceeds project limit %d)", path, lines, limit))
		return false
	}
	if !quiet {
		switch {
		case lines > advisoryLineLimit:
			tui.Warn(out, fmt.Sprintf("%s (%d code lines — advisory; review cohesion before splitting)", path, lines))
		case lines > warnLineLimit:
			tui.SKIP(out, fmt.Sprintf("%s (%d lines — consider splitting)", path, lines))
		default:
			tui.OK(out, fmt.Sprintf("%s (%d lines)", path, lines))
		}
	}
	return true
}

func isArchSourceFile(path string) bool {
	return archSourceExtensions[strings.ToLower(filepath.Ext(path))]
}

func isGeneratedSourceFile(name string) bool {
	lower := strings.ToLower(name)
	if isGeneratedGoFile(lower) {
		return true
	}
	if strings.HasSuffix(lower, ".d.ts") ||
		strings.HasSuffix(lower, ".min.css") ||
		strings.HasSuffix(lower, ".min.js") ||
		lower == "build.rs" ||
		strings.HasPrefix(lower, "mock_") && strings.HasSuffix(lower, ".go") {
		return true
	}
	for _, marker := range []string{"_generated.", ".generated.", "_gen.", ".gen.", ".pb."} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func shouldSkipArchRel(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if archSkipDirs[part] {
			return true
		}
	}
	return false
}

func countStagedLines(dir, rel string) (int, error) {
	cmd := exec.Command("git", "show", ":"+rel)
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return countSourceLines(rel, data)
}
