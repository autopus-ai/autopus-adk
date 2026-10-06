package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pane backend is retired: no production file in this package may call an
// orchestra entry point that launches provider panes (detach split, pane runner,
// interactive pane backend).
func TestCLIProductionCodeNeverCallsPaneEntryPoints(t *testing.T) {
	banned := map[string]bool{
		"RunPaneOrchestraDetached":    true,
		"RunPaneOrchestra":            true,
		"RunInteractivePaneOrchestra": true,
		"NewInteractivePaneBackend":   true,
	}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	var hits []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		file, err := parser.ParseFile(fset, name, src, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "orchestra" && banned[sel.Sel.Name] {
				hits = append(hits, fset.Position(sel.Pos()).String()+" "+sel.Sel.Name)
			}
			return true
		})
	}
	assert.Empty(t, hits, "pane entry points must stay unreachable from the CLI")
}
