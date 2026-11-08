package execcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/testpath"
)

// sourceLocation finds a compiler-style "file.ext:line" reference, or the
// "file.ext(line,col)" form tsc and the C# compiler print.
var sourceLocation = regexp.MustCompile(`([^\s:'"()\[\]<>,]+\.(?:go|rs|ts|tsx|mts|cts|js|jsx|mjs|cjs|py|java|kt|swift|c|cc|cpp|h|hpp|cs|rb))(?::\d+|\(\d+,\d+\))`)

// productBuildFailure reports whether a failed build names a source file of
// the product itself, such as "./main.go:12:3: undefined: x". That is what a
// user would hit too, so it is a product verdict the loop may fix. A build
// that names no product file (a wrong package path, a missing toolchain) more
// likely means the case's build step is wrong, and stays "cannot run".
func productBuildFailure(project, output string) bool {
	for _, match := range sourceLocation.FindAllStringSubmatch(output, -1) {
		rel := filepath.ToSlash(strings.TrimPrefix(match[1], "./"))
		if abs, ok := strings.CutPrefix(rel, filepath.ToSlash(project)+"/"); ok {
			rel = abs
		}
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") || testpath.IsTestPath(rel) {
			continue
		}
		if _, err := os.Stat(filepath.Join(project, filepath.FromSlash(rel))); err == nil {
			return true
		}
	}
	return false
}
