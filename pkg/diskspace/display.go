package diskspace

import (
	"fmt"
	"strings"
)

// SuggestionWithheld replaces the removal command of a worktree whose path
// holds a control character, which no shell quoting renders safely on a
// terminal (REQ-DISK-WT-02).
const SuggestionWithheld = "suggestion withheld: path contains control characters"

// DisplayPath replaces every control character (a byte below 0x20, or 0x7f)
// with '?', the form in which a path is ever printed.
func DisplayPath(path string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, path)
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
