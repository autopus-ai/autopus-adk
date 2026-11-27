package verifygate

import (
	"bufio"
	"bytes"
	"context"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// headView turns HEAD's failure lines into the work tree's coordinates: a
// moved file's new name (see fileMoves) and, for a file the change edited,
// the line its unchanged line has now. A failure the change only moved down
// its file (a test added above it) then reads the same as HEAD's, while a
// different assertion with the same message, on another line, does not.
type headView struct {
	moves  map[string]string
	hunks  map[string][]lineHunk
	module string
}

// lineHunk is one `@@ -old,oldN +new,newN @@` hunk of `git diff -U0`.
type lineHunk struct{ old, oldN, newN int }

func headViewOf(ctx context.Context, root string) headView {
	return headView{moves: fileMoves(ctx, root), hunks: goHunks(ctx, root), module: goModule(root)}
}

func (v headView) runs(runs map[string][]testRun) map[string][]testRun {
	runs = movedRuns(runs, v.moves)
	if len(v.hunks) == 0 {
		return runs
	}
	out := make(map[string][]testRun, len(runs))
	for key, list := range runs {
		pkg, _, ok := splitTestKey(key)
		if !ok {
			pkg = key
		}
		dir := strings.TrimPrefix(packageDir(v.module, pkg), "./")
		for _, r := range list {
			lines := make([]string, 0, len(r.Lines))
			for _, l := range r.Lines {
				lines = append(lines, v.shifted(l, dir))
			}
			out[key] = append(out[key], testRun{Failed: r.Failed, Lines: lines})
		}
	}
	return out
}

// lineAt is a failure line's file and line number: "x_test.go:183: ...",
// "./a.go:3:2: ..." or "$ROOT/pkg/a.go:7: ...".
var lineAt = regexp.MustCompile(`^((?:[^\s:]*/)?)([^/\s:]+\.go):(\d+)`)

// shifted moves one failure line of a test in package dir to the line it
// has now. A line inside a hunk the change rewrote has no line now, so it
// is marked "?" and matches no current failure. A file outside the
// package's directory is located only when the line names it under $ROOT.
// A bare non-test name is left as it is (see below).
func (v headView) shifted(l, dir string) string {
	m := lineAt.FindStringSubmatchIndex(l)
	if m == nil || dir == "" && !strings.HasPrefix(l, "$ROOT/") {
		return l
	}
	prefix, base := l[m[2]:m[3]], l[m[4]:m[5]]
	// A bare name that is not a _test.go file may be a helper of another
	// package (one without t.Helper), so it is compared as it is; another
	// package's _test.go cannot be called.
	if prefix == "" && !strings.HasSuffix(base, "_test.go") {
		return l
	}
	file := path.Join(dir, base)
	if rest, ok := strings.CutPrefix(prefix, "$ROOT/"); ok {
		file = path.Join(rest, base)
	}
	hunks, ok := v.hunks[file]
	if !ok {
		return l
	}
	n, _ := strconv.Atoi(l[m[6]:m[7]])
	now := "?"
	if moved, ok := shiftLine(n, hunks); ok {
		now = strconv.Itoa(moved)
	}
	return l[:m[6]] + now + l[m[7]:]
}

// shiftLine maps line n of HEAD's file to the work tree's, through hunks
// in file order; false when the change rewrote or deleted it.
func shiftLine(n int, hunks []lineHunk) (int, bool) {
	off := 0
	for _, h := range hunks {
		// A pure insertion (-old,0) goes after line old.
		if h.oldN == 0 && n <= h.old || h.oldN > 0 && n < h.old {
			break
		}
		if h.oldN > 0 && n < h.old+h.oldN {
			return 0, false
		}
		off += h.newN - h.oldN
	}
	return n + off, true
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+\d+(?:,(\d+))? @@`)

// goHunks reads `git diff -U0 HEAD` of the tracked Go files the change
// edited, keyed by path. A file HEAD does not have under its path (added,
// or the target of a rename) has no HEAD lines to move and is left out.
func goHunks(ctx context.Context, root string) map[string][]lineHunk {
	// The prefixes and hunk context are fixed: diff.mnemonicPrefix,
	// diff.noprefix and diff.interHunkContext would change what is parsed.
	out, err := gitOutput(ctx, root, "-c", "core.quotepath=off", "diff", "-U0", "--inter-hunk-context=0",
		"--src-prefix=a/", "--dst-prefix=b/", "--no-color", "--no-ext-diff", "--no-renames", "HEAD", "--", "*.go")
	if err != nil {
		return nil
	}
	hunks := map[string][]lineHunk{}
	var file string
	added := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			file, added = "", false
		case line == "--- /dev/null":
			added = true
		case strings.HasPrefix(line, "+++ b/") && !added:
			file = strings.TrimPrefix(line, "+++ b/")
			hunks[file] = nil
		case file != "":
			if m := hunkHeader.FindStringSubmatch(line); m != nil {
				hunks[file] = append(hunks[file], lineHunk{old: atoiOr(m[1], 1), oldN: atoiOr(m[2], 1), newN: atoiOr(m[3], 1)})
			}
		}
	}
	return hunks
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, _ := strconv.Atoi(s)
	return n
}
