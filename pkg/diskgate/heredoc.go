package diskgate

import (
	"path/filepath"
	"strings"
)

// heredoc is a here-document whose body starts at the next newline. at is
// the index in the token list where its << stood.
type heredoc struct {
	delim     string
	stripTabs bool
	at        int
}

// readHeredocWord reads the delimiter word after `<<` at i, an optional `-`
// and blanks before it, and returns the index just past it. Quotes and
// backslashes in the word are removed, as the shell removes them.
func readHeredocWord(line string, i int) (heredoc, int) {
	var doc heredoc
	if i < len(line) && line[i] == '-' {
		doc.stripTabs = true
		i++
	}
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	var word strings.Builder
	for ; i < len(line) && strings.IndexByte(" \t\n;&|<>()", line[i]) < 0; i++ {
		switch c := line[i]; c {
		case '\'', '"':
			end := strings.IndexByte(line[i+1:], c)
			if end < 0 {
				word.WriteString(line[i+1:])
				i = len(line)
				continue
			}
			word.WriteString(line[i+1 : i+1+end])
			i += end + 1
		case '\\':
			if i+1 < len(line) {
				word.WriteByte(line[i+1])
				i++
			}
		default:
			word.WriteByte(c)
		}
	}
	doc.delim = word.String()
	return doc, i
}

// readHeredocs returns the body of each of docs, whose bodies follow one
// another from i, and the index just past the terminator line of the last.
func readHeredocs(line string, i int, docs []heredoc) ([]string, int) {
	bodies := make([]string, len(docs))
	for k, doc := range docs {
		var body strings.Builder
		for i < len(line) {
			text, next := line[i:], len(line)
			if end := strings.IndexByte(line[i:], '\n'); end >= 0 {
				text, next = line[i:i+end], i+end+1
			}
			if doc.stripTabs {
				text = strings.TrimLeft(text, "\t")
			}
			i = next
			if text == doc.delim {
				break
			}
			body.WriteString(text + "\n")
		}
		bodies[k] = body.String()
	}
	return bodies, i
}

// heredocIsScript reports whether the here-document whose << stood at index
// at of tokens, the tokens of its line, is the script of a shell: the
// command it feeds is sh, bash, zsh or dash reading commands from stdin, or
// is piped, through any number of `|`, into such a shell. `ssh host <<EOF`
// runs its body on another machine, so it is not.
func heredocIsScript(tokens []token, at int) bool {
	start := at
	for start > 0 && !tokens[start-1].op {
		start--
	}
	for {
		end := start
		var words []string
		for ; end < len(tokens) && !tokens[end].op; end++ {
			words = append(words, tokens[end].text)
		}
		if stdinShell(words) {
			return true
		}
		if end >= len(tokens) || tokens[end].text != "|" {
			return false
		}
		start = end + 1
	}
}

// stdinShell reports whether words run a shell that reads its script from
// stdin: no -c and no script file operand, or -s, or a lone `-`. The
// redirections are already gone from words (the lexer drops them).
func stdinShell(words []string) bool {
	words = stripPrefixes(words)
	if len(words) == 0 || !isShell(filepath.Base(words[0])) {
		return false
	}
	args := words[1:]
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-":
			return true
		case a == "--":
			return i+1 == len(args)
		case a == "--rcfile" || a == "--init-file":
			i++ // the file
		case strings.HasPrefix(a, "--"):
		case strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+"):
			if a[0] == '-' && strings.ContainsRune(a[1:], 'c') {
				return false
			}
			if a[0] == '-' && strings.ContainsRune(a[1:], 's') {
				return true
			}
			if strings.HasSuffix(a, "o") || strings.HasSuffix(a, "O") {
				i++ // -o, -euo, +O and the like take an option name
			}
		default:
			return false // a script file reads the here-document as data
		}
	}
	return true
}

// arithmeticEnd returns the index just past the `))` that closes the
// arithmetic expansion whose `$((` ends before i, or -1 when the text is not
// one: `$((cmd) 2>&1)` is a command substitution of a subshell, told apart
// by the `)` that closes the inner parenthesis not being followed by `)`.
func arithmeticEnd(line string, i int) int {
	for depth := 0; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
				continue
			}
			if i+1 < len(line) && line[i+1] == ')' {
				return i + 2
			}
			return -1
		}
	}
	return -1
}
