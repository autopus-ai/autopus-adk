package cli

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// terminalSafe returns s with every rune that strconv.IsPrint rejects (control
// characters such as ESC and BEL, and format characters such as bidi
// overrides) replaced by its Go escape, and every byte that is not valid
// UTF-8 by \xNN. Text read from a user file, such as a provider key or a
// settings event name, then cannot move the cursor, retitle the terminal, or
// hide itself when a line prints it. Printable text, including non-ASCII, is
// unchanged, and the result is printable, so applying it twice changes
// nothing.
func terminalSafe(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r == utf8.RuneError || !strconv.IsPrint(r) }) {
		return s
	}
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[0])
		case strconv.IsPrint(r):
			b.WriteString(s[:size])
		default:
			quoted := strconv.QuoteRune(r)
			b.WriteString(quoted[1 : len(quoted)-1])
		}
		s = s[size:]
	}
	return b.String()
}
