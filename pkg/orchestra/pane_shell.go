package orchestra

import "strings"

// shellEscapeArg wraps a string in single quotes for safe shell interpolation.
// Any embedded single quotes use the standard POSIX quote-break pattern:
// close quote, emit an escaped quote, then reopen the quote.
func shellEscapeArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellEscapeArgs applies shellEscapeArg to each element and joins with spaces.
func shellEscapeArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	escaped := make([]string, len(args))
	for i, a := range args {
		escaped[i] = shellEscapeArg(a)
	}
	return strings.Join(escaped, " ")
}

// uniqueHeredocDelimiter returns a heredoc delimiter that does not appear in content.
// Falls back to appending a random suffix if the base delimiter is found in content.
func uniqueHeredocDelimiter(base, content, randomSuffix string) string {
	delim := base
	if strings.Contains(content, delim) {
		delim = base + "_" + randomSuffix
	}
	return delim
}
