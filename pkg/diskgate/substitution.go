package diskgate

import "strings"

// substitutions returns the commands of the $( ... ) and backquote spans in
// expr, an arithmetic expression, which the shell runs before it computes.
func substitutions(expr string) []string {
	var commands []string
	for i := 0; i < len(expr); i++ {
		switch {
		case strings.HasPrefix(expr[i:], "$("):
			depth, j := 1, i+2
			for ; j < len(expr) && depth > 0; j++ {
				switch expr[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
			}
			end := j
			if depth == 0 {
				end = j - 1
			}
			commands = append(commands, expr[i+2:end])
			i = j - 1
		case expr[i] == '`':
			end := strings.IndexByte(expr[i+1:], '`')
			if end < 0 {
				return append(commands, expr[i+1:])
			}
			commands = append(commands, expr[i+1:i+1+end])
			i += end + 1
		}
	}
	return commands
}
