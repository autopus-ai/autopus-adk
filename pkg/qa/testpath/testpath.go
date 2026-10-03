// Package testpath decides whether a project-relative path is test code. The
// QA loop's diff guard and triage share it, so the file the guard keeps a
// product fix away from is the same file triage blames for a test defect.
package testpath

import (
	"path"
	"strings"
)

// dirNames hold tests at any depth below the project root.
var dirNames = map[string]bool{"e2e": true, "tests": true, "test": true, "__tests__": true, "spec": true}

// scriptExts are the JavaScript and TypeScript extensions whose _test and
// _spec stems mark a test (Deno, Jasmine, and similar runners).
var scriptExts = map[string]bool{".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true, ".mts": true, ".cts": true}

// IsTestPath reports whether rel, a slash-separated path relative to the
// project, names test code: a test file name (*_test.go, *.spec.*, *.test.*,
// *_spec.rb, test_*.py, *_test.py, and the *_test / *_spec script forms) or
// any file below an e2e/, tests/, test/, __tests__/, or spec/ directory.
//
// Directory names count only inside rel, so a caller holding an absolute path
// must make it project-relative first, or pass the base name alone.
func IsTestPath(rel string) bool {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if isTestFileName(path.Base(rel)) {
		return true
	}
	for _, segment := range strings.Split(path.Dir(rel), "/") {
		if dirNames[segment] {
			return true
		}
	}
	return false
}

func isTestFileName(base string) bool {
	if strings.Contains(base, ".spec.") || strings.Contains(base, ".test.") {
		return true
	}
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	switch {
	case strings.HasSuffix(stem, "_test"):
		return ext == ".go" || ext == ".py" || scriptExts[ext]
	case strings.HasSuffix(stem, "_spec"):
		return ext == ".rb" || scriptExts[ext]
	}
	return ext == ".py" && strings.HasPrefix(stem, "test_")
}
