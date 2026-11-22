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
// so it is skipped through its terminator line, unless a shell reads it as
// its script (heredocIsScript): then its lines are lexed as commands.
// Arithmetic expansion $(( ... )) is one word, so its << is a shift, not a
// here-document. Unquoted redirections and their targets are dropped, and
// a backslash-newline is removed. An unterminated quote or here-document ends at the end of
// the input.
func lex(line string) []token {
	var out []token
	var pending []heredoc
	var word strings.Builder
	inWord, quoted, lineStart := false, false, 0
	flush := func() {
		if inWord {
			out = append(out, token{text: word.String()})
			word.Reset()
			inWord = false
		}
		quoted = false
	}
	op := func(text string) {
		flush()
		out = append(out, token{text: text, op: true})
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\'':
			inWord, quoted = true, true
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				word.WriteString(line[i+1:])
				i = len(line)
				continue
			}
			word.WriteString(line[i+1 : i+1+end])
			i += end + 1
		case c == '"':
			inWord, quoted = true, true
			i = readDouble(line, i+1, &word)
		case c == '\\' && i+1 < len(line) && line[i+1] == '\n':
			// A line continuation is removed; it adds nothing to the word.
			i++
		case c == '\\' && i+1 < len(line):
			inWord, quoted = true, true
			word.WriteByte(line[i+1])
			i++
		case c == ' ' || c == '\t':
			flush()
		case c == '#' && !inWord:
			// A comment runs to the end of the line.
			for i < len(line) && line[i] != '\n' {
				i++
			}
			i--
		case strings.HasPrefix(line[i:], "$((") && arithmeticEnd(line, i+3) > 0:
			end := arithmeticEnd(line, i+3)
			inWord, quoted = true, true
			word.WriteString(line[i:end])
			// Command substitutions inside the expression still run.
			for _, command := range substitutions(line[i+3 : end-2]) {
				flush()
				out = append(out, token{text: "$(", op: true})
				out = append(out, lex(command)...)
				out = append(out, token{text: ")", op: true})
			}
			i = end - 1
		case c == '$' && i+1 < len(line) && line[i+1] == '(':
			op("$(")
			i++
		case c == '&' && strings.HasPrefix(line[i+1:], ">"):
			// &>file and &>>file redirect both streams.
			flush()
			i = readRedirection(line, i+1) - 1
		case redirectionAt(line, i):
			// A redirection and its target are not words of the command; an
			// unquoted descriptor number right before it belongs to it.
			if inWord && !quoted && strings.Trim(word.String(), "0123456789") == "" {
				word.Reset()
				inWord = false
			}
			flush()
			i = readRedirection(line, i) - 1
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
				doc.at = len(out)
				pending = append(pending, doc)
			}
			i = next - 1
		case c == '\n':
			op("\n")
			if len(pending) > 0 {
				bodies, next := readHeredocs(line, i+1, pending)
				head := out[lineStart : len(out)-1]
				for k, doc := range pending {
					if heredocIsScript(head, doc.at-lineStart) {
						out = append(out, lex(bodies[k])...)
						out = append(out, token{text: "\n", op: true})
					}
				}
				i, pending = next-1, nil
			}
			lineStart = len(out)
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
