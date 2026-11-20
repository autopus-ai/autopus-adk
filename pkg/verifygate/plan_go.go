package verifygate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// goSteps builds the module, runs the tests the change edited, then runs
// the changed packages whole. A large package (one CI splits into shards)
// can outlast the budget, compiling its test binary included, so both test
// steps are BudgetBound: a failure seen in time blocks, running out of time
// without one skips instead of blocking every stop. The changed tests run
// first so their verdict arrives inside the budget, but with at most half of
// it: a slow new test must not keep the whole package from running.
func goSteps(root string, changed []string) []Step {
	pkgs := goPackages(root, changed)
	if len(pkgs) == 0 {
		return nil
	}
	steps := []Step{{Name: "go build", Argv: []string{"go", "build", "./..."}, Dir: root}}
	if names, testPkgs := changedGoTests(root, changed); len(names) > 0 {
		steps = append(steps, Step{Name: "go test (changed tests)",
			Argv: append([]string{"go", "test", "-json", "-run", "^(" + strings.Join(names, "|") + ")$"}, testPkgs...),
			Dir:  root, GoTestJSON: true, BudgetBound: true, Share: 0.5})
	}
	steps = append(steps, Step{Name: "go test", Argv: append([]string{"go", "test", "-json"}, pkgs...), Dir: root,
		GoTestJSON: true, BudgetBound: true})
	return steps
}

var goTestName = regexp.MustCompile(`^Test([A-Z0-9_].*)?$`)

// changedGoTests returns the top-level Test functions declared in changed
// _test.go files, and the packages that hold them. A file that does not
// parse contributes nothing; the whole-package step still compiles it.
func changedGoTests(root string, changed []string) ([]string, []string) {
	var names, pkgs []string
	fset := token.NewFileSet()
	for _, f := range changed {
		if !strings.HasSuffix(f, "_test.go") || strings.Contains("/"+f, "/testdata/") || strings.HasPrefix(f, "vendor/") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(f)), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		found := false
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !goTestName.MatchString(fn.Name.Name) {
				continue
			}
			found = true
			if !slices.Contains(names, fn.Name.Name) {
				names = append(names, fn.Name.Name)
			}
		}
		if dir := path.Dir(f); found {
			pattern := "./" + dir
			if dir == "." {
				pattern = "."
			}
			if !slices.Contains(pkgs, pattern) {
				pkgs = append(pkgs, pattern)
			}
		}
	}
	slices.Sort(names)
	slices.Sort(pkgs)
	return names, pkgs
}
