package healthband

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Patch Policy items 2–3 for names: a header path that starts with a quote
// is C-quoted, its escapes (\a \b \f \n \r \t \v \\ \" and three octal
// digits) decode to bytes, and every check runs on the decoded bytes. A
// decoded path is refused when it is empty, absolute, not valid UTF-8, holds
// an empty, `.`, or `..` segment, or holds a code point of the item 7 set,
// TAB and CR included (CD-3 N1).

// cEscapes are the single-letter escapes of git's C quoting.
var cEscapes = map[byte]byte{'a': '\a', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v', '\\': '\\', '"': '"'}

// unquoteC decodes the C-quoted name at the start of s and returns it with
// the bytes it used.
func unquoteC(s string) (string, int, bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", 0, false
	}
	var out []byte
	for i := 1; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			return string(out), i + 1, true
		case '\\':
			if i+1 >= len(s) {
				return "", 0, false
			}
			i++
			if decoded, ok := cEscapes[s[i]]; ok {
				out = append(out, decoded)
				continue
			}
			if i+2 >= len(s) || s[i] < '0' || s[i] > '3' || !isOctal(s[i+1]) || !isOctal(s[i+2]) {
				return "", 0, false
			}
			out = append(out, (s[i]-'0')<<6|(s[i+1]-'0')<<3|(s[i+2]-'0'))
			i += 2
		default:
			out = append(out, c)
		}
	}
	return "", 0, false
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

// gitHeaderPath reads the two names of a `diff --git` line, each C-quoted
// when it starts with a quote, and returns the one path both name below
// their a/ and b/ prefixes; two different paths are a rename or copy.
func gitHeaderPath(rest string) (string, bool) {
	var a, b string
	switch quote := strings.Index(rest, ` "`); {
	case strings.HasPrefix(rest, `"`):
		first, used, ok := unquoteC(rest)
		if !ok || !strings.HasPrefix(rest[used:], " ") {
			return "", false
		}
		a, b = first, rest[used+1:]
	case quote >= 0:
		a, b = rest[:quote], rest[quote+1:]
	default:
		// Unquoted on both sides: the line splits at its middle space.
		half := (len(rest) - 1) / 2
		if len(rest) < 7 || len(rest)%2 == 0 || rest[half] != ' ' {
			return "", false
		}
		a, b = rest[:half], rest[half+1:]
	}
	if strings.HasPrefix(b, `"`) {
		second, used, ok := unquoteC(b)
		if !ok || used != len(b) {
			return "", false
		}
		b = second
	}
	pathA, okA := strings.CutPrefix(a, "a/")
	pathB, okB := strings.CutPrefix(b, "b/")
	return pathB, okA && okB && pathA == pathB && pathB != ""
}

// validPaths applies the in-process path rules of item 3 to every section.
func validPaths(files []*diffFile) bool {
	for _, file := range files {
		if !validPath(file.path) {
			return false
		}
	}
	return true
}

func validPath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || !utf8.ValidString(path) {
		return false
	}
	for _, r := range path {
		if inControlSet(r) {
			return false
		}
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// controlTables is the item 7 set: general categories Cc, Cf, Zl, Zp, and
// Co, and Default_Ignorable_Code_Point. Go has no derived table of that
// property, so the union adds Other_Default_Ignorable_Code_Point and
// Variation_Selector to Cf, a superset that holds U+2028, U+2029, the Hangul
// fillers, U+00AD, U+034F, U+180E, U+2061–U+2064, the variation selectors,
// the whole tag block, and every bidi, zero-width, C0, DEL, and C1 control.
var controlTables = []*unicode.RangeTable{
	unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Co,
	unicode.Other_Default_Ignorable_Code_Point, unicode.Variation_Selector,
}

// inControlSet reports a code point of the item 7 set, TAB included; the
// added-line check allows TAB itself.
func inControlSet(r rune) bool { return unicode.In(r, controlTables...) }

// pathPrefixes returns the directory prefixes of a slash path, shortest first.
func pathPrefixes(path string) []string {
	var prefixes []string
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			prefixes = append(prefixes, path[:i])
		}
	}
	return prefixes
}
