package diskspace

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SuggestionWithheld replaces the removal command of a worktree whose path
// holds a control character, which no shell quoting renders safely on a
// terminal (REQ-DISK-WT-02).
const SuggestionWithheld = "suggestion withheld: path contains control characters"

// DisplayPath replaces with '?' every rune strconv.IsPrint rejects, control
// characters and bidirectional overrides included, and every byte that is
// not valid UTF-8: the form in which a path is ever printed (REQ-DISK-WT-02).
func DisplayPath(path string) string {
	var b strings.Builder
	for len(path) > 0 {
		r, size := utf8.DecodeRuneInString(path)
		if r == utf8.RuneError && size == 1 || !strconv.IsPrint(r) {
			b.WriteByte('?')
		} else {
			b.WriteString(path[:size])
		}
		path = path[size:]
	}
	return b.String()
}

func hasControl(path string) bool { return DisplayPath(path) != path }

// ShellQuote renders s as one POSIX single-quoted word; a single quote
// inside is written as close quote, backslash quote, open quote.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func removalSuggestion(path, base string, bytes int64) string {
	if hasControl(path) {
		return SuggestionWithheld
	}
	return fmt.Sprintf("candidate: git worktree remove -- %s (merged into %s, clean, %s)", ShellQuote(path), base, SizeString(bytes))
}
