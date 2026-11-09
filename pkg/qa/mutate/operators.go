// Package mutate measures whether a project's exec test cases catch bugs. It
// plants one small fault at a time in the product's sources (a flipped
// comparison, a swapped boolean, an off-by-one literal), runs the cases
// against a copy of the project, and reports the faults no case noticed: each
// is a behaviour the scenarios do not actually check.
package mutate

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Mutant is one planted fault: Line (1-based) of File changes from Original
// to Mutated.
type Mutant struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Operator string `json:"operator"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
}

// sourceExts are the languages whose operators below mean the same thing.
var sourceExts = map[string]bool{
	".go": true, ".rs": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".py": true, ".java": true, ".kt": true, ".swift": true, ".c": true, ".cc": true, ".cpp": true, ".cs": true, ".rb": true,
}

// swaps replace a spaced operator. The surrounding spaces keep generics,
// arrows, channels, and shifts (`[]T`, `=>`, `<-`, `>>`) out of reach, and
// keep " == " from matching inside " === ".
var swaps = []struct{ name, from, to string }{
	{"negate-strict-equality", " === ", " !== "},
	{"negate-strict-inequality", " !== ", " === "},
	{"negate-equality", " == ", " != "},
	{"negate-inequality", " != ", " == "},
	{"boundary-le", " <= ", " < "},
	{"boundary-ge", " >= ", " > "},
	{"boundary-lt", " < ", " <= "},
	{"boundary-gt", " > ", " >= "},
	{"swap-and", " && ", " || "},
	{"swap-or", " || ", " && "},
	{"swap-python-and", " and ", " or "},
	{"swap-python-or", " or ", " and "},
}

var (
	boolWord   = regexp.MustCompile(`\b(true|false|True|False)\b`)
	intLiteral = regexp.MustCompile(`\b[0-9]+\b`)
)

// Candidates lists every fault the operators can plant in one file's source,
// ordered by line. Comment lines and text inside string literals are skipped.
func Candidates(file, source string) []Mutant {
	if !sourceExts[path.Ext(file)] {
		return nil
	}
	var out []Mutant
	lines := strings.Split(source, "\n")
	prev := ""
	for i, line := range lines {
		code := codePart(line)
		guard := prev
		if strings.TrimSpace(code) != "" {
			prev = code
		}
		if strings.TrimSpace(code) == "" {
			continue
		}
		add := func(op string, at int, from, to string) {
			out = append(out, Mutant{File: file, Line: i + 1, Operator: op, Original: line,
				Mutated: line[:at] + to + line[at+len(from):]})
		}
		for _, s := range swaps {
			if at := strings.Index(code, s.from); at >= 0 && !excludedEquality(guard, code, at, s.from) {
				add(s.name, at, s.from, s.to)
			}
		}
		if loc := boolWord.FindStringIndex(code); loc != nil {
			word := code[loc[0]:loc[1]]
			add("flip-boolean", loc[0], word, flipBool(word))
		}
		if loc := intLiteral.FindStringIndex(code); loc != nil && !partOfIdentifier(code, loc[0]) {
			n, err := strconv.Atoi(code[loc[0]:loc[1]])
			if err == nil {
				add("off-by-one", loc[0], code[loc[0]:loc[1]], strconv.Itoa(n+1))
			}
		}
	}
	return out
}

// Sample picks at most n mutants spread evenly over all of them, so a long
// file does not take the whole budget. The choice is deterministic.
func Sample(all []Mutant, n int) []Mutant {
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	if n <= 0 || len(all) <= n {
		return all
	}
	out := make([]Mutant, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, all[i*len(all)/n])
	}
	return out
}

// codePart blanks string literals and cuts a trailing comment, keeping byte
// offsets so a match in it is a match in the original line.
func codePart(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"//", "#", "/*", "*", "import ", "package ", "use "} {
		if strings.HasPrefix(trimmed, prefix) {
			return ""
		}
	}
	b := []byte(line)
	var quote byte
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case quote != 0:
			if c == '\\' && i+1 < len(b) {
				b[i], b[i+1] = ' ', ' '
				i++
				continue
			}
			if c == quote {
				quote = 0
			} else {
				b[i] = ' '
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '/' && i+1 < len(b) && b[i+1] == '/', c == '#' && i > 0 && b[i-1] == ' ':
			return string(b[:i])
		}
	}
	return string(b)
}

// excludedEquality reports a boundary swap on a comparison whose two sides
// the previous line has just shown to differ, as in a sort comparator:
//
//	if a != b {
//	    return a > b
//
// Turning > into >= there changes nothing, so the mutant could only survive.
func excludedEquality(guard, code string, at int, op string) bool {
	if !strings.HasPrefix(op, " <") && !strings.HasPrefix(op, " >") {
		return false
	}
	left := lastOperand(code[:at])
	right := firstOperand(code[at+len(op):])
	if left == "" || right == "" {
		return false
	}
	return strings.Contains(guard, left+" != "+right) || strings.Contains(guard, right+" != "+left)
}

func lastOperand(s string) string {
	s = strings.TrimSpace(s)
	for _, kw := range []string{"return ", "if ", "("} {
		if i := strings.LastIndex(s, kw); i >= 0 {
			s = s[i+len(kw):]
		}
	}
	return strings.TrimSpace(s)
}

func firstOperand(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " {);"); i >= 0 {
		s = s[:i]
	}
	return s
}

func partOfIdentifier(code string, at int) bool {
	if at == 0 {
		return false
	}
	c := code[at-1]
	return c == '_' || c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func flipBool(word string) string {
	switch word {
	case "true":
		return "false"
	case "false":
		return "true"
	case "True":
		return "False"
	}
	return "True"
}
