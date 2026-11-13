package verifygate

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/linecount"
)

// Behavior is how much of a change can alter what a user runs: changed code
// lines in product source files, and product source files that are new.
// Tests, comment-only and blank lines, generated files and harness output
// are left out, so a change that only adds tests or documents code does not
// count, while a one-line logic edit does (as two lines: removed and added).
type Behavior struct {
	Lines    int      `json:"lines"`
	NewFiles []string `json:"new_files,omitempty"`
	// Unmeasured is set when git could not produce the diff; the change is
	// then treated as large rather than as nothing.
	Unmeasured bool `json:"unmeasured,omitempty"`
	// Paths are the product source paths counted, for a fingerprint of the
	// behavior change alone: a doc edit or chmod elsewhere does not make it
	// a different change.
	Paths []string `json:"-"`
}

// None reports a change with no behavior a user could observe.
func (b Behavior) None() bool { return b.Lines == 0 && len(b.NewFiles) == 0 && !b.Unmeasured }

// BehaviorChange measures the behavior-bearing part of the working tree's
// change against HEAD. Paths come from git's NUL-separated listings, never
// from parsing diff headers, so quoting, prefixes, renames and mode changes
// cannot turn an edited file into a "new" one. A file moved with plain mv
// (a deletion plus an untracked file with the same content) is a rename too.
func BehaviorChange(ctx context.Context, root string) Behavior {
	return behaviorOf(ctx, root, true)
}

// ProductPaths lists the product source paths BehaviorChange would count,
// without counting lines: enough to fingerprint the change cheaply.
func ProductPaths(ctx context.Context, root string) []string {
	return behaviorOf(ctx, root, false).Paths
}

func behaviorOf(ctx context.Context, root string, count bool) Behavior {
	var b Behavior
	entries := rawEntries(ctx, root)
	untracked := untrackedProductFiles(ctx, root)
	deletedBlobs := map[string]int{}
	for i, e := range entries {
		// Only a product file can be the source of a rename; a test moved
		// into a product path is new behavior.
		if e.status == 'D' && productSource(root, e.path, true) {
			deletedBlobs[e.oldBlob] = i
		}
	}
	moved := map[int]bool{}
	var added []string
	for f, blob := range hashObjects(ctx, root, untracked) {
		if i, ok := deletedBlobs[blob]; ok && !moved[i] {
			moved[i] = true
			continue
		}
		added = append(added, f)
	}
	var hashPaths, slashPaths []string
	for i, e := range entries {
		if moved[i] || !productSource(root, e.path, e.status == 'D') {
			continue
		}
		b.Paths = append(b.Paths, e.path)
		if hashComments(e.path) {
			hashPaths = append(hashPaths, e.paths...)
		} else {
			slashPaths = append(slashPaths, e.paths...)
		}
		if e.status == 'A' {
			b.NewFiles = append(b.NewFiles, e.path)
		}
	}
	if !count {
		return Behavior{Paths: append(b.Paths, added...)}
	}
	hashLines, okHash := trackedCodeLines(ctx, root, hashPaths, true)
	slashLines, okSlash := trackedCodeLines(ctx, root, slashPaths, false)
	b.Lines, b.Unmeasured = hashLines+slashLines, !okHash || !okSlash
	slices.Sort(added)
	for _, f := range added {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil || bytes.Contains(data, []byte{0}) {
			continue
		}
		b.NewFiles = append(b.NewFiles, f)
		b.Paths = append(b.Paths, f)
		for _, line := range strings.Split(string(data), "\n") {
			if isCodeLine(line, hashComments(f)) {
				b.Lines++
			}
		}
	}
	return b
}

type rawEntry struct {
	status  byte
	oldBlob string
	path    string   // the path after the change (the old path for a deletion)
	paths   []string // every path the diff must cover: both sides of a rename
}

func rawEntries(ctx context.Context, root string) []rawEntry {
	out, err := gitOutput(ctx, root, "diff", "--raw", "-z", "--no-abbrev", "-M", "HEAD", "--")
	if err != nil {
		return nil
	}
	return parseRawZ(out)
}

// parseRawZ reads `git diff --raw -z`: ":srcmode dstmode srcsha dstsha
// STATUS", then one path, or two for a rename or copy (old, then new).
func parseRawZ(out []byte) []rawEntry {
	fields := strings.Split(string(out), "\x00")
	var entries []rawEntry
	for i := 0; i+1 < len(fields); i++ {
		meta := strings.Fields(strings.TrimPrefix(fields[i], ":"))
		if !strings.HasPrefix(fields[i], ":") || len(meta) < 5 || meta[4] == "" {
			continue
		}
		e := rawEntry{status: meta[4][0], oldBlob: meta[2]}
		if (e.status == 'R' || e.status == 'C') && i+2 < len(fields) {
			e.path, e.paths = fields[i+2], []string{fields[i+1], fields[i+2]}
			i += 2
		} else {
			e.path, e.paths = fields[i+1], []string{fields[i+1]}
			i++
		}
		entries = append(entries, e)
	}
	return entries
}

func untrackedProductFiles(ctx context.Context, root string) []string {
	out, err := gitOutput(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" && productSource(root, f, false) {
			files = append(files, f)
		}
	}
	return files
}

// hashObjects returns each file's blob id, so an untracked file can be
// matched with the deleted file it was moved from. A file git cannot hash
// keeps an empty id and is treated as new.
func hashObjects(ctx context.Context, root string, files []string) map[string]string {
	ids := make(map[string]string, len(files))
	for _, batch := range batches(files) {
		out, err := gitOutput(ctx, root, append([]string{"hash-object", "--"}, batch...)...)
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, f := range batch {
			ids[f] = ""
			if err == nil && len(lines) == len(batch) {
				ids[f] = lines[i]
			}
		}
	}
	return ids
}

// pathBatch keeps one git invocation's argv well under the OS argument
// limit (1 MiB on macOS) however many files a change touches.
const pathBatch = 500

func batches(paths []string) [][]string {
	var out [][]string
	for len(paths) > pathBatch {
		out, paths = append(out, paths[:pathBatch]), paths[pathBatch:]
	}
	if len(paths) > 0 {
		out = append(out, paths)
	}
	return out
}

// trackedCodeLines counts changed code lines in tracked paths that share one
// comment syntax. Only hunk bodies are read, so diff headers and prefixes
// never matter; --text keeps a file marked -diff countable, and
// --ignore-cr-at-eol keeps a line-ending conversion from counting at all.
// ok is false when git could not produce the diff: the caller must not read
// that as "nothing changed".
func trackedCodeLines(ctx context.Context, root string, paths []string, hash bool) (lines int, ok bool) {
	for _, batch := range batches(paths) {
		args := []string{"diff", "-U0", "-M", "--no-color", "--no-ext-diff", "--no-textconv", "--text", "--ignore-cr-at-eol", "HEAD", "--"}
		for _, p := range batch {
			args = append(args, ":(literal)"+p)
		}
		out, err := gitOutput(ctx, root, args...)
		if err != nil {
			return lines, false
		}
		lines += hunkCodeLines(out, hash)
	}
	return lines, true
}

func hunkCodeLines(diff []byte, hash bool) int {
	lines, inHunk := 0, false
	sc := bufio.NewScanner(bytes.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case inHunk && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")):
			if isCodeLine(line[1:], hash) {
				lines++
			}
		}
	}
	return lines
}

// productSource is a code file in any language the file-size limit knows,
// plus the toolchains the build gate runs, that is neither a test, generated
// code, nor harness output. A Java or shell project's logic change is
// behavior too. A deleted file cannot be read, so it is not checked for a
// generated-code marker.
func productSource(root, file string, deleted bool) bool {
	if slices.ContainsFunc(harnessDirs, func(dir string) bool { return strings.HasPrefix(file, dir) }) {
		return false
	}
	if _, known := linecount.Language(path.Ext(file)); !known && !IsSource(file) {
		return false
	}
	return !testPath(file) && (deleted || !generated(root, file))
}

// testPath applies each language's own test layout. A Go package named spec
// or a file named limits_spec.go is product code; only _test.go and testdata
// are Go tests.
func testPath(file string) bool {
	if IsTest(file) {
		return true
	}
	slashed := "/" + file
	inDir := func(dirs ...string) bool {
		return slices.ContainsFunc(dirs, func(d string) bool { return strings.Contains(slashed, d) })
	}
	stem := strings.TrimSuffix(path.Base(file), path.Ext(file))
	suffix := func(suffixes ...string) bool {
		return slices.ContainsFunc(suffixes, func(s string) bool { return strings.HasSuffix(stem, s) })
	}
	switch path.Ext(file) {
	case ".go":
		return false
	case ".py":
		return inDir("/tests/", "/test/") || path.Base(file) == "conftest.py"
	case ".rb":
		return inDir("/spec/", "/test/") || suffix("_spec", "_test")
	case ".java", ".kt", ".kts":
		return inDir("/src/test/") || suffix("Test", "Tests", "Spec", "IT")
	case ".swift":
		return inDir("/Tests/") || suffix("Test", "Tests", "Spec")
	case ".cs":
		return suffix("Test", "Tests")
	case ".c", ".cc", ".cpp", ".cxx", ".h", ".hpp", ".sh":
		return inDir("/test/", "/tests/") || suffix("_test", "_unittest")
	}
	return inDir("/test/", "/tests/")
}

// generated reads the Go convention marker ("Code generated ... DO NOT
// EDIT.") and common generated names. The files the ADK generates in a
// project live under harnessDirs, which are excluded already; matching its
// header here would also hide the ADK's own template sources.
func generated(root, file string) bool {
	base := path.Base(file)
	if strings.HasSuffix(base, ".pb.go") || strings.HasPrefix(base, "zz_generated") || strings.Contains(base, ".gen.") {
		return true
	}
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 1024)
	n, _ := f.Read(head)
	text := string(head[:n])
	return strings.Contains(text, "Code generated") && strings.Contains(text, "DO NOT EDIT")
}

func hashComments(file string) bool {
	switch path.Ext(file) {
	case ".py", ".rb", ".sh":
		return true
	}
	return false
}

// isCodeLine drops blank lines and lines that start a line comment. It is
// deliberately conservative: block-comment bodies and lines that open with
// "*" or "/*" count as code, because operator-leading continuation lines
// ("* rate"), CSS's "*" selector and code after an inline "/* */" look the
// same. Over-counting a comment is safer than hiding a logic change. Go
// compiler directives (//go:build, //go:embed) are code.
func isCodeLine(line string, hash bool) bool {
	t := strings.TrimSpace(line)
	switch {
	case t == "":
		return false
	case hash:
		return !strings.HasPrefix(t, "#")
	case strings.HasPrefix(t, "//go:"):
		return true
	}
	return !strings.HasPrefix(t, "//")
}
