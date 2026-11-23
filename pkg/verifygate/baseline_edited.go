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
// and "dir\x00" for a package whose existing test support changed: a
// declaration of its changed _test.go files other than a test function (a
// case table, a helper, a type, a var) that was modified or deleted, an
// added or changed init or TestMain, or a file under its testdata. dir is
// as path.Dir gives it.
//
// A failing test the change edited is the change's failure even when HEAD's
// version failed too: HEAD runs the old version, which says nothing about
// the new one. Edited support can change what every test of the package
// does (a table row moves under the same subtest name), so it charges all
// of the package's failures. A newly added helper, type or var cannot
// change how a test HEAD already had behaves (redefining a name does not
// compile), so it charges nothing; init and TestMain run for every test,
// so adding one does. Declarations are compared across the package's
// changed files together, so a helper moved between them is not an edit.
func editedTestFuncs(ctx context.Context, root string, changed []string) map[string]bool {
	edited := map[string]bool{}
	before, after := map[string]*testDecls{}, map[string]*testDecls{}
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
		dir := path.Dir(f)
		if before[dir] == nil {
			before[dir], after[dir] = newTestDecls(), newTestDecls()
		}
		if old, err := gitOutput(ctx, root, "show", "HEAD:"+f); err == nil {
			before[dir].add(old)
		}
		// A deleted file adds nothing: its support counts as deleted.
		if now, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			after[dir].add(now)
		}
	}
	for dir, now := range after {
		old := before[dir]
		for name, src := range now.tests {
			if prev, ok := old.tests[name]; !ok || prev != src {
				edited[dir+"\x00"+name] = true
			}
		}
		if supportEdited(old.support, now.support) {
			edited[dir+"\x00"] = true
		}
	}
	return edited
}

// supportEdited reports a support declaration HEAD had that is gone or
// differs, or an init, TestMain or blank var (whose initializer runs at
// package init, like init) that is new or differs.
func supportEdited(before, after map[string][]string) bool {
	for key, src := range before {
		if !slices.Equal(src, after[key]) {
			return true
		}
	}
	for _, key := range []string{"func init", "func TestMain", "var _"} {
		if !slices.Equal(before[key], after[key]) {
			return true
		}
	}
	return false
}

// testFuncName is a function go test runs by name. TestMain is not one: it
// runs around all of them.
var testFuncName = regexp.MustCompile(`^(Test|Example|Fuzz|Benchmark)([A-Z0-9_].*)?$`)

// testDecls is a package's top-level test functions by name, and the
// source of every other declaration except imports, keyed by what it
// declares ("func name", "method Recv.name", "var a,b", "type T"); init
// may repeat, so each key holds a sorted list.
type testDecls struct {
	tests   map[string]string
	support map[string][]string
}

func newTestDecls() *testDecls {
	return &testDecls{tests: map[string]string{}, support: map[string][]string{}}
}

// add parses one test file into d. A file that does not parse adds
// nothing; the build reports the parse error, and a package that does not
// build is never pre-existing unless HEAD fails it the same.
func (d *testDecls) add(src []byte) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return
	}
	text := func(n ast.Node) string {
		return string(src[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
	}
	put := func(key, s string) {
		d.support[key] = append(d.support[key], s)
		slices.Sort(d.support[key])
	}
	for _, decl := range file.Decls {
		switch dl := decl.(type) {
		case *ast.FuncDecl:
			switch {
			case dl.Recv != nil:
				put("method "+recvName(dl.Recv)+"."+dl.Name.Name, text(dl))
			case dl.Name.Name != "TestMain" && testFuncName.MatchString(dl.Name.Name):
				d.tests[dl.Name.Name] = text(dl)
			default:
				put("func "+dl.Name.Name, text(dl))
			}
		case *ast.GenDecl:
			if dl.Tok == token.IMPORT {
				continue
			}
			for _, spec := range dl.Specs {
				put(specKey(dl.Tok, spec), text(spec))
			}
		}
	}
}

func specKey(tok token.Token, spec ast.Spec) string {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return "type " + s.Name.Name
	case *ast.ValueSpec:
		names := make([]string, 0, len(s.Names))
		for _, n := range s.Names {
			names = append(names, n.Name)
		}
		return tok.String() + " " + strings.Join(names, ",")
	}
	return tok.String()
}

func recvName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if idx, ok := t.(*ast.IndexExpr); ok {
		t = idx.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
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
