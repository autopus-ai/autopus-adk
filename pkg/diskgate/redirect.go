package diskgate

import "strings"

// redirectionAt reports whether an unquoted redirection operator starts at
// i: > >> >| >& < <& <>. A here-document (<<), a here-string (<<<) and a
// process substitution <( or >( are not.
func redirectionAt(line string, i int) bool {
	c := line[i]
	if c != '<' && c != '>' {
		return false
	}
	if i+1 < len(line) && (line[i+1] == '(' || (c == '<' && line[i+1] == '<')) {
		return false
	}
	return true
}

// readRedirection reads the redirection operator at i and its target word,
// and returns the index just past them.
func readRedirection(line string, i int) int {
	c := line[i]
	i++
	if i < len(line) && ((c == '>' && strings.IndexByte(">|&", line[i]) >= 0) || (c == '<' && strings.IndexByte("&>", line[i]) >= 0)) {
		i++
	}
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	// A backquote ends the target, so the command it runs is lexed.
	for ; i < len(line) && strings.IndexByte(" \t\n;&|()<>`", line[i]) < 0; i++ {
		switch line[i] {
		case '\'':
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				return len(line)
			}
			i += end + 1
		case '"':
			var discard strings.Builder
			i = readDouble(line, i+1, &discard)
		case '\\':
			if i+1 < len(line) {
				i++
			}
		}
	}
	return i
}
