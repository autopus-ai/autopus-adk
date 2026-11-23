package verifygate

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// tempPath matches a path under a temporary directory: os.TempDir(),
// $TMPDIR, $GOTMPDIR (and their symlink-resolved forms), /tmp, /private/tmp,
// and macOS's per-user /var/folders/<a>/<b>/T. Group 1 is the rest of the
// path, up to a space, quote, colon or comma.
var tempPath = sync.OnceValue(func() *regexp.Regexp {
	var roots []string
	for _, dir := range []string{os.TempDir(), os.Getenv("TMPDIR"), os.Getenv("GOTMPDIR")} {
		dir = strings.TrimRight(dir, "/")
		if dir == "" {
			continue
		}
		roots = append(roots, dir)
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			roots = append(roots, strings.TrimRight(real, "/"))
		}
	}
	// Longer roots first, so /private/var/... is not matched as /var/....
	slices.SortFunc(roots, func(a, b string) int { return len(b) - len(a) })
	alts := make([]string, 0, len(roots)+2)
	for _, r := range slices.Compact(roots) {
		alts = append(alts, regexp.QuoteMeta(r))
	}
	alts = append(alts, `(?:/private)?/var/folders/[^/\s]+/[^/\s]+/T`, `(?:/private)?/tmp`)
	return regexp.MustCompile(`(?:` + strings.Join(alts, "|") + `)((?:/[^\s:'"(),]*)?)`)
})

// volatileRun is what changes from run to run in the first component
// under a temporary root: the random suffix os.MkdirTemp and t.TempDir add
// (harneval-surface-3151427053, TestFoo3087128865), five digits or more.
// Below that component every name is the test's own (t.TempDir's /001
// sequence, srv-50000.sock, 002.json) and may be the value under test, so
// it is kept.
var volatileRun = regexp.MustCompile(`\d{5,}`)

// normalizeTemp rewrites the temporary paths of a failure line as
// <TMP>/..., with their volatile digit runs as <N>, so the same failure
// reads the same from two runs. Paths outside a temporary directory and
// numbers elsewhere in the line (an assertion's values) are left as they
// are. A root followed by more name characters (/tmpfoo) is not a match.
func normalizeTemp(line string) string {
	re := tempPath()
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
		start, end := m[0], m[1]
		if m[2] == m[3] && end < len(line) && isPathChar(line[end]) {
			continue
		}
		if start > 0 && isPathChar(line[start-1]) {
			continue
		}
		rest := line[m[2]:m[3]]
		first, below, isDir := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
		// Only a directory, or a name without an extension, is one that
		// MkdirTemp or TempDir made; srv-50000.sock is the test's own.
		if rest != "" && (isDir || path.Ext(first) == "") {
			rest = "/" + volatileRun.ReplaceAllString(first, "<N>")
			if isDir {
				rest += "/" + below
			}
		}
		b.WriteString(line[last:start])
		b.WriteString("<TMP>" + rest)
		last = end
	}
	b.WriteString(line[last:])
	return b.String()
}

func isPathChar(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c == '/' ||
		('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}
