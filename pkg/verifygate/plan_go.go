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

// goSteps returns the Go steps that run first, the build and the tests the
// change edited, and the whole changed packages, which Plan runs after every
// other toolchain. A large package (one CI splits into shards) can outlast
// the budget, compiling its test binary included, so both test steps are
// BudgetBound: a failure seen in time blocks, running out of time without
// one skips instead of blocking every stop. The changed tests get at most
// half the budget, so a slow new test cannot keep the package from running;
// the whole-package step runs them again, so their share running out leaves
// nothing unchecked.
func goSteps(root string, changed []string) (first, whole []Step) {
	pkgs := goPackages(root, changed)
	if len(pkgs) == 0 {
		return nil, nil
	}
	first = []Step{{Name: "go build", Argv: []string{"go", "build", "./..."}, Dir: root}}
	names, testPkgs := changedGoTests(root, changed)
	if pattern := "^(" + strings.Join(names, "|") + ")$"; len(names) > 0 && len(pattern) <= maxRunPattern {
		first = append(first, Step{Name: "go test (changed tests)",
			Argv: append([]string{"go", "test", "-json", "-run", pattern}, testPkgs...),
			Dir:  root, GoTestJSON: true, BudgetBound: true, Share: 0.5, CoveredLater: true})
	}
	whole = []Step{{Name: "go test", Argv: append([]string{"go", "test", "-json"}, pkgs...), Dir: root,
		GoTestJSON: true, BudgetBound: true, GoPackages: importPaths(root, pkgs)}}
	return first, whole
}

// importPaths turns ./dir patterns into import paths under the module
// go.mod declares; nil when it declares none.
func importPaths(root string, patterns []string) []string {
	module := goModule(root)
	if module == "" {
		return nil
	}
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if p == "." {
			out = append(out, module)
		} else {
			out = append(out, module+"/"+strings.TrimPrefix(p, "./"))
		}
	}
	return out
}

// maxRunPattern bounds the -run argument: one argument may not exceed 128
// KiB on Linux and a whole command line 32 KiB on Windows. A change that
// touches more tests than fit (a module rename) skips straight to the whole
// packages, which run the same tests.
const maxRunPattern = 4096

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
