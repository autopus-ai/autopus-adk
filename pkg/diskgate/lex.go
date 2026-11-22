// Package diskgate classifies a shell command line as heavy (a build, test,
// install, or worktree step that can write gigabytes) or not, for the
// PreToolUse disk gate. It reads the text only: nothing is run or resolved.
package diskgate

import "strings"

// token is one word or one control operator of a command line.
type token struct {
	text string
	op   bool
}

// lex splits a command line into words and control operators the way a POSIX
// shell does for the parts the gate needs: single and double quotes, backslash
// escapes, and the operators && || ; | & ( ) and newline. Command
// substitution $( ... ) and backquotes outside quotes are treated as
// operators too, so the command they run is classified on its own. The body
// of a here-document (<<WORD, <<-WORD, quoted or not) is data, not commands,
// so it is skipped through its terminator line. An unterminated quote or
// here-document ends at the end of the input.
func lex(line string) []token {
	var out []token
	var pending []heredoc
	var word strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			out = append(out, token{text: word.String()})
			word.Reset()
			inWord = false
		}
	}
	op := func(text string) {
		flush()
		out = append(out, token{text: text, op: true})
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\'':
			inWord = true
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				word.WriteString(line[i+1:])
				i = len(line)
				continue
			}
			word.WriteString(line[i+1 : i+1+end])
			i += end + 1
		case c == '"':
			inWord = true
			i = readDouble(line, i+1, &word)
		case c == '\\' && i+1 < len(line):
			inWord = true
			if line[i+1] != '\n' {
				word.WriteByte(line[i+1])
			}
			i++
		case c == ' ' || c == '\t':
			flush()
		case c == '#' && !inWord:
			// A comment runs to the end of the line.
			for i < len(line) && line[i] != '\n' {
				i++
			}
			i--
		case c == '$' && i+1 < len(line) && line[i+1] == '(':
			op("$(")
			i++
		case c == '&' || c == '|':
			if i+1 < len(line) && line[i+1] == c {
				op(string([]byte{c, c}))
				i++
				continue
			}
			op(string(c))
		case c == '<' && strings.HasPrefix(line[i:], "<<<"):
			// A here-string is one word of input, not a here-document.
			inWord = true
			word.WriteString("<<<")
			i += 2
		case c == '<' && strings.HasPrefix(line[i:], "<<"):
			flush()
			doc, next := readHeredocWord(line, i+2)
			if doc.delim != "" {
				pending = append(pending, doc)
			}
			i = next - 1
		case c == '\n':
			op("\n")
			if len(pending) > 0 {
				i = skipHeredocs(line, i+1, pending) - 1
				pending = nil
			}
		case c == ';' || c == '(' || c == ')' || c == '`':
			op(string(c))
		default:
			inWord = true
			word.WriteByte(c)
		}
	}
	flush()
	return out
}

// heredoc is a here-document whose body starts at the next newline.
type heredoc struct {
	delim     string
	stripTabs bool
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

// skipHeredocs returns the index just past the terminator line of the last
// of docs, whose bodies follow one another from i.
func skipHeredocs(line string, i int, docs []heredoc) int {
	for _, doc := range docs {
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
		}
	}
	return i
}

// readDouble reads a double-quoted span starting after the opening quote and
// returns the index of the closing quote, or the last index of the input.
func readDouble(line string, i int, word *strings.Builder) int {
	for ; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '"':
			return i
		case c == '\\' && i+1 < len(line) && strings.IndexByte("\"\\$`\n", line[i+1]) >= 0:
			if line[i+1] != '\n' {
				word.WriteByte(line[i+1])
			}
			i++
		default:
			word.WriteByte(c)
		}
	}
	return len(line) - 1
}

// segments groups the words between control operators, one simple command
// each.
func segments(tokens []token) [][]string {
	var out [][]string
	var cur []string
	for _, t := range tokens {
		if t.op {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, t.text)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}
