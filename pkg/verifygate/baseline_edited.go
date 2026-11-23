package verifygate

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// editedTestFuncs returns what the change edited of the tests, keyed
// "dir\x00Name" for a top-level test function (Test, Example, Fuzz,
// Benchmark) whose source differs from HEAD's or that HEAD does not have,
// and "dir\x00" for a package whose test support changed: any other
// declaration of a changed _test.go file (a case table, a helper, a type,
// init), or a file under its testdata. dir is as path.Dir gives it.
//
// A failing test the change edited is the change's failure even when HEAD's
// version failed too: HEAD runs the old version, which says nothing about
// the new one. Edited support can change what every test of the package
// does (a table row moves under the same subtest name), so it charges all
// of the package's failures.
func editedTestFuncs(ctx context.Context, root string, changed []string) map[string]bool {
	edited := map[string]bool{}
	for _, f := range changed {
		if strings.HasPrefix(f, "vendor/") {
			continue
		}
		if i := strings.Index("/"+f, "/testdata/"); i >= 0 {
			dir := "."
			if i > 0 {
				dir = f[:i-1]
			}
			edited[dir+"\x00"] = true
			continue
		}
		if !strings.HasSuffix(f, "_test.go") {
			continue
		}
		now, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			// Deleted: none of its tests can fail.
			continue
		}
		var before testDecls
		if old, err := gitOutput(ctx, root, "show", "HEAD:"+f); err == nil {
			before = parseTestDecls(old)
		}
		after := parseTestDecls(now)
		for name, src := range after.tests {
			if old, ok := before.tests[name]; !ok || old != src {
				edited[path.Dir(f)+"\x00"+name] = true
			}
		}
		if !slices.Equal(before.support, after.support) {
			edited[path.Dir(f)+"\x00"] = true
		}
	}
	return edited
}

// testFuncName is a function go test runs by name.
var testFuncName = regexp.MustCompile(`^(Test|Example|Fuzz|Benchmark)([A-Z0-9_].*)?$`)

// testDecls is a Go test file's top-level test functions by name, and the
// sorted source of every other declaration except imports.
type testDecls struct {
	tests   map[string]string
	support []string
}

// parseTestDecls splits a test file's declarations. A file that does not
// parse yields nothing; the build reports the parse error, and a package
// that does not build is never pre-existing unless HEAD fails it the same.
func parseTestDecls(src []byte) testDecls {
	out := testDecls{tests: map[string]string{}}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return out
	}
	for _, decl := range file.Decls {
		text := string(src[fset.Position(decl.Pos()).Offset:fset.Position(decl.End()).Offset])
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && testFuncName.MatchString(d.Name.Name) {
				out.tests[d.Name.Name] = text
				continue
			}
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
		}
		out.support = append(out.support, text)
	}
	slices.Sort(out.support)
	return out
}

// isEditedTest reports whether a failed key's top-level test, or its
// package's test support, is one the change edited. A package-level key
// counts as edited when the package's test support was.
func isEditedTest(root, key string, edited map[string]bool) bool {
	pkg, top, ok := splitTestKey(key)
	if !ok {
		pkg = key
	}
	dir := packageDir(goModule(root), pkg)
	if dir == "" {
		return false
	}
	dir = strings.TrimPrefix(dir, "./")
	return edited[dir+"\x00"] || (ok && edited[dir+"\x00"+top])
}
