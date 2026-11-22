package content

import (
	"fmt"
	"io"
	"os"
	"sync"
)

var printedDiagnostics sync.Map

// PrintDiagnosticOnce writes a generation diagnostic to stderr the first time
// this process sees that exact line.
//
// One command can build the same platform adapter several times: doctor
// validates the surface, previews drift, and checks the context catalog, and
// each pass regenerates. A per-adapter guard then printed the same
// "[codex] extended skills" or "[omp] model routing" line two or three times,
// which reads like several problems. The line itself is the identity.
func PrintDiagnosticOnce(line string) {
	printDiagnosticOnce(os.Stderr, line)
}

func printDiagnosticOnce(w io.Writer, line string) {
	if line == "" {
		return
	}
	if _, seen := printedDiagnostics.LoadOrStore(line, struct{}{}); seen {
		return
	}
	fmt.Fprintln(w, line)
}
