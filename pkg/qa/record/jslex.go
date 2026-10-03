package record

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokNumber
	tokPunct
)

type token struct {
	kind tokenKind
	text string
}

var errBadEscape = errors.New("invalid escape sequence")

// simpleEscapes are the single-character escapes JavaScript defines. A
// backslash before a newline continues the line and contributes nothing.
var simpleEscapes = map[byte]string{
	'n': "\n", 't': "\t", 'r': "\r", 'b': "\b", 'f': "\f", 'v': "\v", '0': "\x00", '\n': "",
}

// lexJS tokenizes one statement of the codegen dialect: identifiers, string
// and number literals, and call punctuation. Regular expressions, operators,
// arrays, and template interpolation are refused; none has a scenario
// equivalent, so guessing at them would invent a step.
func lexJS(src string) ([]token, error) {
	var out []token
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '/' && strings.HasPrefix(src[i:], "//"):
			return out, nil
		case isIdentByte(c, true):
			j := i + 1
			for j < len(src) && isIdentByte(src[j], false) {
				j++
			}
			out = append(out, token{kind: tokIdent, text: src[i:j]})
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < len(src) && src[j] >= '0' && src[j] <= '9' {
				j++
			}
			out = append(out, token{kind: tokNumber, text: src[i:j]})
			i = j
		case c == '\'' || c == '"' || c == '`':
			value, n, err := readString(src[i:])
			if err != nil {
				return nil, err
			}
			out = append(out, token{kind: tokString, text: value})
			i += n
		case strings.IndexByte(".(){},:;", c) >= 0:
			out = append(out, token{kind: tokPunct, text: src[i : i+1]})
			i++
		case c == '/':
			return nil, errors.New("regular expressions are not supported")
		default:
			r, _ := utf8.DecodeRuneInString(src[i:])
			return nil, fmt.Errorf("unsupported syntax %q", r)
		}
	}
	return out, nil
}

func isIdentByte(c byte, first bool) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == '$':
		return true
	case c >= '0' && c <= '9':
		return !first
	}
	return false
}

// readString decodes the quoted literal at the start of s and returns its
// value and byte length. A template literal is accepted only without
// interpolation, because an interpolated value is unknown until run time.
func readString(s string) (string, int, error) {
	quote := s[0]
	var b strings.Builder
	for i := 1; i < len(s); {
		c := s[i]
		switch {
		case c == quote:
			return b.String(), i + 1, nil
		case c == '\\':
			n, err := readEscape(s[i+1:], &b)
			if err != nil {
				return "", 0, err
			}
			i += 1 + n
		case quote == '`' && strings.HasPrefix(s[i:], "${"):
			return "", 0, errors.New("template interpolation is not supported")
		case c == '\n' && quote != '`':
			return "", 0, errors.New("unterminated string")
		default:
			b.WriteByte(c)
			i++
		}
	}
	return "", 0, errors.New("unterminated string")
}

// readEscape decodes the escape after a backslash into b and returns how many
// bytes it consumed. An unknown escape stands for its own character, as in
// JavaScript, which covers \' \" \` \\ and \/.
func readEscape(s string, b *strings.Builder) (int, error) {
	if s == "" {
		return 0, errors.New("unterminated string")
	}
	if decoded, ok := simpleEscapes[s[0]]; ok {
		b.WriteString(decoded)
		return 1, nil
	}
	switch s[0] {
	case 'x':
		if len(s) < 3 {
			return 0, errBadEscape
		}
		v, err := strconv.ParseUint(s[1:3], 16, 8)
		if err != nil {
			return 0, errBadEscape
		}
		b.WriteRune(rune(v))
		return 3, nil
	case 'u':
		r, n, err := readUnicodeEscape(s)
		if err != nil {
			return 0, err
		}
		// A code point beyond the BMP arrives as two escaped surrogates.
		if utf16.IsSurrogate(r) && strings.HasPrefix(s[n:], "\\u") {
			if low, m, err := readUnicodeEscape(s[n+1:]); err == nil {
				if combined := utf16.DecodeRune(r, low); combined != utf8.RuneError {
					b.WriteRune(combined)
					return n + 1 + m, nil
				}
			}
		}
		b.WriteRune(r)
		return n, nil
	}
	r, size := utf8.DecodeRuneInString(s)
	b.WriteRune(r)
	return size, nil
}

// readUnicodeEscape reads `u` followed by four hex digits or a braced code
// point, returning the rune and the bytes consumed.
func readUnicodeEscape(s string) (rune, int, error) {
	if strings.HasPrefix(s, "u{") {
		end := strings.IndexByte(s, '}')
		if end < 3 {
			return 0, 0, errBadEscape
		}
		v, err := strconv.ParseUint(s[2:end], 16, 32)
		if err != nil || v > utf8.MaxRune {
			return 0, 0, errBadEscape
		}
		return rune(v), end + 1, nil
	}
	if len(s) < 5 {
		return 0, 0, errBadEscape
	}
	v, err := strconv.ParseUint(s[1:5], 16, 32)
	if err != nil {
		return 0, 0, errBadEscape
	}
	return rune(v), 5, nil
}
