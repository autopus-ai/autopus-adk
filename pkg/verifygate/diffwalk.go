package verifygate

import (
	"bytes"
	"strconv"
	"strings"
)

// walkAdded calls fn for every added line of `git diff --unified=0` output,
// with its repository path and new-side line number. Harness paths are
// skipped. A line of any length is read: a bufio.Scanner stopped at its
// buffer limit and silently dropped every file after one long line, which
// let a credential in a later file through. Lines longer than maxLine are
// counted but not handed to fn.
func walkAdded(diff []byte, maxLine int, fn func(file string, line int, text string)) {
	var file string
	var line int
	inHunk := false
	for len(diff) > 0 {
		var raw []byte
		if i := bytes.IndexByte(diff, '\n'); i >= 0 {
			raw, diff = diff[:i], diff[i+1:]
		} else {
			raw, diff = diff, nil
		}
		switch {
		case bytes.HasPrefix(raw, []byte("diff --git ")):
			file, inHunk = "", false
		case !inHunk && bytes.HasPrefix(raw, []byte("+++ ")):
			file = diffPath(string(raw[4:]))
			if len(withoutHarnessPaths([]string{file})) == 0 {
				file = ""
			}
		case bytes.HasPrefix(raw, []byte("@@ ")):
			inHunk, line = true, hunkStart(string(raw))
		case inHunk && len(raw) > 0 && raw[0] == '+':
			if file != "" && len(raw) <= maxLine {
				fn(file, line, string(raw[1:]))
			}
			line++
		}
	}
}

// diffPath turns a `+++ b/path` header into the repository path; /dev/null
// (a deletion) yields "". Git quotes a path with special characters
// ("b/a\tb.go") and ends a path that has a space with a tab.
func diffPath(header string) string {
	header = strings.TrimRight(header, "\t\r")
	if strings.HasPrefix(header, `"`) {
		if unq, err := strconv.Unquote(header); err == nil {
			header = unq
		} else {
			header = strings.Trim(header, `"`)
		}
	}
	if !strings.HasPrefix(header, "b/") {
		return ""
	}
	return strings.TrimPrefix(header, "b/")
}
