package verifygate

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// editedTestFuncs returns the top-level functions of the changed _test.go
// files whose source differs from HEAD's, or that HEAD does not have, keyed
// "dir\x00Name" with dir as path.Dir gives it. A failing test the change
// edited is the change's failure even when HEAD's version failed too: the
// agent rewrote it, so a HEAD run of the old version says nothing about the
// new one. A failing test whose own function is unchanged is compared on
// HEAD even when a helper beside it changed; HEAD runs HEAD's helper, so a
// failure there too means the test was failing before the change.
func editedTestFuncs(ctx context.Context, root string, changed []string) map[string]bool {
	edited := map[string]bool{}
	for _, f := range changed {
		if !strings.HasSuffix(f, "_test.go") || strings.Contains("/"+f, "/testdata/") || strings.HasPrefix(f, "vendor/") {
			continue
		}
		now, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			// Deleted: none of its tests can fail.
			continue
		}
		before := map[string]string{}
		if old, err := gitOutput(ctx, root, "show", "HEAD:"+f); err == nil {
			before = funcSources(old)
		}
		for name, src := range funcSources(now) {
			if old, ok := before[name]; !ok || old != src {
				edited[path.Dir(f)+"\x00"+name] = true
			}
		}
	}
	return edited
}

// funcSources maps each top-level function of a Go file to its source text.
// A file that does not parse maps nothing, so its tests count as edited
// only through the file being new; the build reports the parse error.
func funcSources(src []byte) map[string]string {
	out := map[string]string{}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return out
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		start, end := fset.Position(fn.Pos()).Offset, fset.Position(fn.End()).Offset
		out[fn.Name.Name] = string(src[start:end])
	}
	return out
}

// isEditedTest reports whether a failed key's top-level test is one the
// change edited. A package-level key is never an edited test.
func isEditedTest(root, key string, edited map[string]bool) bool {
	pkg, top, ok := splitTestKey(key)
	if !ok {
		return false
	}
	dir := packageDir(goModule(root), pkg)
	if dir == "" {
		return false
	}
	return edited[strings.TrimPrefix(dir, "./")+"\x00"+top]
}
