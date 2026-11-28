package debt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func rvGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func rvRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	rvGit(t, root, "init", "-q")
	for n, b := range files {
		write(t, root, n, b)
	}
	rvGit(t, root, "add", "-A")
	rvGit(t, root, "commit", "-qm", "base")
	return root
}

func rvNew(t *testing.T, root string) []Item {
	t.Helper()
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	base, err := LoadBaseline(root)
	require.NoError(t, err)
	block, _ := Blocking(New(items, base))
	return block
}

func rvBaseline(t *testing.T, root string) {
	t.Helper()
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	_, _, err = WriteBaseline(root, items, false)
	require.NoError(t, err)
}

func longBlock(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\tresult%d := computeSomethingLong(input%d, options.Threshold, options.Limit)\n", i, i)
	}
	return b.String()
}

// B: fixing ONE copy of a baseline duplicate (less duplication) reports new duplicates.
func TestDebtRegression_EditInsideDuplicateIsNewDebt(t *testing.T) {
	body := longBlock(20)
	root := rvRepo(t, map[string]string{
		"b.go": "package a\n\nfunc B() {\n" + body + "}\n",
		"c.go": "package a\n\nfunc C() {\n" + body + "}\n",
	})
	rvBaseline(t, root)
	lines := strings.Split(body, "\n")
	lines[10] = "\tresult10 := somethingElseEntirely(input10)"
	write(t, root, "c.go", "package a\n\nfunc C() {\n"+strings.Join(lines, "\n")+"}\n")
	got := rvNew(t, root)
	for _, it := range got {
		t.Logf("NEW after editing one copy: %s", it)
	}
	if len(got) > 0 {
		t.Errorf("debt-reducing edit inside a baseline duplicate reports %d new blocking item(s)", len(got))
	}
}

// B2: renaming an identifier in BOTH copies (same duplication) reports a new duplicate.
func TestDebtRegression_RenameInsideDuplicateIsNewDebt(t *testing.T) {
	body := longBlock(8)
	root := rvRepo(t, map[string]string{
		"b.go": "package a\n\nfunc B() {\n" + body + "}\n",
		"c.go": "package a\n\nfunc C() {\n" + body + "}\n",
	})
	rvBaseline(t, root)
	renamed := strings.ReplaceAll(body, "options.", "opts.")
	write(t, root, "b.go", "package a\n\nfunc B() {\n"+renamed+"}\n")
	write(t, root, "c.go", "package a\n\nfunc C() {\n"+renamed+"}\n")
	got := rvNew(t, root)
	for _, it := range got {
		t.Logf("NEW after renaming in both copies: %s", it)
	}
	if len(got) > 0 {
		t.Errorf("a rename across an existing duplicate reports %d new blocking item(s)", len(got))
	}
}

// C: git mv of a file with a baseline TODO makes the TODO new.
func TestDebtRegression_RenamedFileMarkersAreNew(t *testing.T) {
	root := rvRepo(t, map[string]string{"util.go": "package a\n\n// TODO handle unicode\nfunc U() {}\n"})
	rvBaseline(t, root)
	rvGit(t, root, "mv", "util.go", "strutil.go")
	got := rvNew(t, root)
	for _, it := range got {
		t.Logf("NEW after git mv: %s", it)
	}
	if len(got) > 0 {
		t.Errorf("renaming a file reports its %d baseline marker(s) as new", len(got))
	}
}

// E: comment openers inside strings and SQL LIKE patterns count as markers.
func TestDebtRegression_MarkerFalsePositives(t *testing.T) {
	root := rvRepo(t, map[string]string{
		"q.sql":    "SELECT id FROM notes WHERE body LIKE '%TODO%';\n",
		"repo.go":  "package a\n\nconst q = `SELECT id FROM tasks WHERE title LIKE '%FIXME%'`\nconst endpoint = \"https://XXX.execute-api.us-east-1.amazonaws.com\"\nvar help = \"usage: tool // TODO is printed literally\"\n",
		"tmpl.go":  "package a\n\nconst doc = `\n## TODO\n- item\n`\n",
		"run.sh":   "#!/bin/sh\ngit checkout -- TODO.md\n",
		"style.py": "x = 10 % XXX_LIMIT if False else 1\nprint('# HACK night is on friday')\n",
	})
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	for _, it := range items {
		if it.Kind == KindMarker {
			t.Logf("MARKER: %s", it)
		}
	}
}

// F: a new file importing a sorted subset of a common import block is a "new duplicate".
func TestDebtRegression_ImportSubsetIsNewDuplicate(t *testing.T) {
	imps := []string{
		"\"github.com/example/project/pkg/adapter\"",
		"\"github.com/example/project/pkg/adapter/antigravity\"",
		"\"github.com/example/project/pkg/adapter/claude\"",
		"\"github.com/example/project/pkg/adapter/codex\"",
		"\"github.com/example/project/pkg/adapter/omp\"",
		"\"github.com/example/project/pkg/adapter/opencode\"",
		"\"github.com/example/project/pkg/config\"",
		"\"github.com/example/project/pkg/verifygate\"",
	}
	file := func(pkgBody string, set []string) string {
		return "package a\n\nimport (\n\t" + strings.Join(set, "\n\t") + "\n)\n\n" + pkgBody
	}
	root := rvRepo(t, map[string]string{
		"x.go": file("func X() {}\n", imps),
		"y.go": file("func Y() {}\n", imps),
	})
	rvBaseline(t, root)
	write(t, root, "z.go", file("func Z() { /* new feature */ }\n", imps[:7]))
	got := rvNew(t, root)
	for _, it := range got {
		t.Logf("NEW after adding a file with a common import subset: %s", it)
	}
}

// G: a same-size edit whose mtime is restored stays stale in the index.
func TestDebtRegression_IndexStaleOnSameSizeAndMtime(t *testing.T) {
	root := rvRepo(t, map[string]string{"a.go": "package a\n\n// NOTE handle x\nfunc A() {}\n"})
	_, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	p := filepath.Join(root, "a.go")
	fi, err := os.Stat(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, []byte("package a\n\n// TODO handle x\nfunc A() {}\n"), 0o644))
	require.NoError(t, os.Chtimes(p, fi.ModTime(), fi.ModTime()))
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	t.Logf("items after a same-size, same-mtime edit that adds a TODO: %v", items)
}

// H + I: corrupt index and concurrent scans.
func TestDebtRegression_CorruptAndConcurrentIndex(t *testing.T) {
	root := rvRepo(t, map[string]string{"a.go": "package a\n\n// TODO x\nfunc A() {}\n"})
	dst := filepath.Join(root, filepath.FromSlash(indexPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
	require.NoError(t, os.WriteFile(dst, []byte("garbage-not-a-gob"), 0o644))
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	require.Len(t, items, 1)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			write(t, root, fmt.Sprintf("f%d.go", i), fmt.Sprintf("package a\n// TODO item %d\n", i))
			if _, err := Scan(context.Background(), root, 300); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent scan: %v", err)
	}
	items, err = Scan(context.Background(), root, 300)
	require.NoError(t, err)
	require.Len(t, items, 17)
	left, _ := filepath.Glob(dst + ".*.tmp")
	t.Logf("leftover tmp files: %v", left)
}

// J: one big changed file blows the 0.8s stop budget, since the deadline is only checked between files.
func TestDebtRegression_BigFileIgnoresBudget(t *testing.T) {
	root := rvRepo(t, map[string]string{"small.go": "package a\n"})
	var b strings.Builder
	for i := 0; b.Len() < 120<<20; i++ {
		fmt.Fprintf(&b, "  var value%d = computeSomething(argumentNumber%d, anotherArgument, yetAnotherOne);\n", i, i)
	}
	write(t, root, "dist/bundle.js", b.String())
	start := time.Now()
	_, err := ScanWith(context.Background(), root, Options{Deadline: time.Now().Add(800 * time.Millisecond), SizeOf: func(string) bool { return false }})
	t.Logf("scan with a 0.8s deadline over one 120 MiB .js file took %v (err=%v)", time.Since(start), err)
	fi, _ := os.Stat(filepath.Join(root, filepath.FromSlash(indexPath)))
	if fi != nil {
		t.Logf("index size: %d bytes", fi.Size())
	}
	start = time.Now()
	_, err = ScanWith(context.Background(), root, Options{Deadline: time.Now().Add(800 * time.Millisecond), SizeOf: func(string) bool { return false }})
	t.Logf("warm scan (nothing changed) took %v (err=%v)", time.Since(start), err)
}

// K: a vendored dependency bump brings upstream TODOs in as new blocking debt.
func TestDebtRegression_VendorBumpIsNewDebt(t *testing.T) {
	root := rvRepo(t, map[string]string{
		"go.mod":                          "module m\n\ngo 1.22\n",
		"main.go":                         "package main\n\nfunc main() {}\n",
		"vendor/github.com/up/lib/lib.go": "package lib\n\n// TODO(upstream-dev): remove in v2\nfunc F() {}\n",
		"vendor/modules.txt":              "# github.com/up/lib v1.0.0\n",
	})
	rvBaseline(t, root)
	write(t, root, "vendor/github.com/up/lib/lib.go", "package lib\n\n// TODO(upstream-dev): remove in v2\n// FIXME: racy under load\nfunc F() {}\n// XXX deprecated\nfunc G() {}\n")
	got := rvNew(t, root)
	for _, it := range got {
		t.Logf("NEW after a vendor bump: %s", it)
	}
}

// L: the deadline path converges over several scans.
func TestDebtRegression_DeadlineConverges(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 3000; i++ {
		files[fmt.Sprintf("p%d/f%d.go", i%50, i)] = fmt.Sprintf("package p\n\n// TODO item %d\nfunc F%d() {}\n%s", i, i, longBlock(3))
	}
	root := rvRepo(t, files)
	rounds := 0
	for {
		rounds++
		_, err := ScanWith(context.Background(), root, Options{Deadline: time.Now().Add(30 * time.Millisecond)})
		if err == nil {
			break
		}
		require.ErrorIs(t, err, ErrIncomplete)
		require.Less(t, rounds, 500, "did not converge")
	}
	t.Logf("3000 files indexed over %d scans with a 30ms budget", rounds)
}
