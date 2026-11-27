package verifygate

import (
	"bytes"
	"context"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// editedTestFuncs returns what the change edited of the tests, keyed
// "dir\x00Name" for a top-level test function (Test, Example, Fuzz,
// Benchmark) whose source differs from HEAD's or that HEAD does not have,
// and "dir\x00" for a package whose existing test support changed (see
// supportEdited) or that had a file under its testdata changed. dir is as
// path.Dir gives it.
//
// A failing test the change edited is the change's failure even when HEAD's
// version failed too: HEAD runs the old version, which says nothing about
// the new one. Edited support can change what every test of the package
// does (a table row moves under the same subtest name), so it charges all
// of the package's failures. Declarations are compared across the
// package's changed and deleted test files together, so a helper moved
// between them is not an edit, and a deleted file's support is deleted.
// A package and its external test package (x and x_test) in one directory
// are compared apart: they are two packages, so a helper moved from one to
// the other is a deletion from the first, and a test moved from one to the
// other is a new test there (its names can resolve to other declarations).
func editedTestFuncs(ctx context.Context, root string, changed []string) map[string]bool {
	edited := map[string]bool{}
	before, after := map[string]*testDecls{}, map[string]*testDecls{}
	files := append(slices.Clone(changed), deletedPaths(ctx, root)...)
	for _, f := range files {
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
		if old, err := gitOutput(ctx, root, "show", "HEAD:"+f); err == nil {
			declsFor(before, dir, old).add(old)
		}
		// A deleted file adds nothing: its support counts as deleted.
		if now, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			declsFor(after, dir, now).add(now)
		}
	}
	for group := range before {
		if after[group] == nil {
			after[group] = newTestDecls()
		}
	}
	for group, now := range after {
		dir, _, _ := strings.Cut(group, "\x00")
		old := before[group]
		if old == nil {
			old = newTestDecls()
		}
		for name, src := range now.tests {
			if prev, ok := old.tests[name]; !ok || prev != src {
				edited[dir+"\x00"+name] = true
			}
		}
		if supportEdited(old, now) {
			edited[dir+"\x00"] = true
		}
	}
	return edited
}

// declsFor returns the declarations of the package a test file's source
// declares in dir, keyed "dir\x00package", creating them on first use.
func declsFor(groups map[string]*testDecls, dir string, src []byte) *testDecls {
	key := dir + "\x00" + packageName(src)
	if groups[key] == nil {
		groups[key] = newTestDecls()
	}
	return groups[key]
}

// packageName is the package clause of a Go source, or "" when it does not
// parse.
func packageName(src []byte) string {
	file, err := parser.ParseFile(token.NewFileSet(), "", src, parser.PackageClauseOnly)
	if err != nil {
		return ""
	}
	return file.Name.Name
}

// deletedPaths lists the tracked paths the work tree no longer has: deleted
// files and the sources of renames. ChangedFiles leaves them out, since
// nothing there can be built, but a deleted test file can still change
// how the tests left beside it behave.
func deletedPaths(ctx context.Context, root string) []string {
	out, err := gitOutput(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=no")
	if err != nil {
		return nil
	}
	var paths []string
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		status, p := string(entry[:2]), string(entry[3:])
		switch {
		case status[0] == 'R' && i+1 < len(fields):
			i++
			paths = append(paths, string(fields[i]))
		case status[0] == 'C':
			i++
		case status[0] == 'D' || status[1] == 'D':
			paths = append(paths, p)
		}
	}
	return paths
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
