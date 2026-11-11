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
// operators too, so the command they run is classified on its own. An
// unterminated quote ends at the end of the input.
func lex(line string) []token {
	var out []token
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
		case c == ';' || c == '\n' || c == '(' || c == ')' || c == '`':
			op(string(c))
		default:
			inWord = true
			word.WriteByte(c)
		}
	}
	flush()
	return out
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
