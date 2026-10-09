package healthband

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The values of the TOML reader (patchpolicy_toml.go): strings with their
// escapes, booleans, arrays, inline tables, and the numbers, dates, and
// times that no rule reads.

var tomlDate = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

func (p *tomlParser) value() (any, error) {
	switch {
	case p.eof():
		return nil, errTOML
	case p.ahead(`"""`):
		return p.multiBasic()
	case p.peek('"'):
		return p.basicString()
	case p.ahead("'''"):
		return p.multiLiteral()
	case p.peek('\''):
		return p.literalString()
	case p.peek('['):
		return p.array()
	case p.peek('{'):
		return p.inlineTable()
	}
	return p.scalar()
}

// scalar reads a boolean, or a number, date, or time as tomlOther; a local
// date and a time may be joined by one space.
func (p *tomlParser) scalar() (any, error) {
	start := p.pos
	p.scanScalar()
	token := p.src[start:p.pos]
	if tomlDate.MatchString(token) && p.ahead(" ") && p.pos+1 < len(p.src) && isDigit(p.src[p.pos+1]) {
		p.pos++
		p.scanScalar()
	}
	switch unsigned := strings.TrimLeft(token, "+-"); {
	case token == "true":
		return true, nil
	case token == "false":
		return false, nil
	case len(unsigned) > 0 && len(token)-len(unsigned) <= 1 &&
		(isDigit(unsigned[0]) || unsigned == "inf" || unsigned == "nan"):
		return tomlOther{}, nil
	}
	return nil, errTOML
}

func (p *tomlParser) scanScalar() {
	for !p.eof() {
		c := p.src[p.pos]
		if !bareKeyChar(c) && c != '+' && c != '.' && c != ':' {
			return
		}
		p.pos++
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func (p *tomlParser) array() (any, error) {
	if p.depth++; p.depth > tomlMaxDepth {
		return nil, errTOML
	}
	defer func() { p.depth-- }()
	p.pos++
	array := &tomlArray{}
	for {
		if err := p.skipBlank(); err != nil {
			return nil, err
		}
		if p.peek(']') {
			p.pos++
			return array, nil
		}
		item, err := p.value()
		if err != nil {
			return nil, err
		}
		array.items = append(array.items, item)
		if err := p.skipBlank(); err != nil {
			return nil, err
		}
		switch {
		case p.peek(','):
			p.pos++
		case p.peek(']'):
			p.pos++
			return array, nil
		default:
			return nil, errTOML
		}
	}
}

func (p *tomlParser) inlineTable() (any, error) {
	if p.depth++; p.depth > tomlMaxDepth {
		return nil, errTOML
	}
	defer func() { p.depth-- }()
	p.pos++
	table := newTOMLTable()
	for {
		if err := p.skipBlank(); err != nil {
			return nil, err
		}
		if p.peek('}') {
			p.pos++
			table.inline = true
			return table, nil
		}
		if err := p.keyValue(table); err != nil {
			return nil, err
		}
		if err := p.skipBlank(); err != nil {
			return nil, err
		}
		switch {
		case p.peek(','):
			p.pos++
		case p.peek('}'):
			p.pos++
			table.inline = true
			return table, nil
		default:
			return nil, errTOML
		}
	}
}

// basicString reads "...": escapes, no newline, no control character but TAB.
func (p *tomlParser) basicString() (string, error) {
	p.pos++
	var b strings.Builder
	for !p.eof() {
		switch c := p.src[p.pos]; {
		case c == '"':
			p.pos++
			return b.String(), nil
		case c == '\\':
			if err := p.escape(&b); err != nil {
				return "", err
			}
		case c < 0x20 && c != '\t' || c == 0x7f:
			return "", errTOML
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
	return "", errTOML
}

// multiBasic reads a triple-double-quoted string: a newline right after
// the opening delimiter is dropped, a line-ending backslash drops the
// blanks and newlines after it, and up to two quotes before the closing
// delimiter are content.
func (p *tomlParser) multiBasic() (string, error) {
	p.pos += 3
	p.dropFirstNewline()
	var b strings.Builder
	for !p.eof() {
		switch c := p.src[p.pos]; {
		case p.ahead(`"""`):
			b.WriteString(strings.Repeat(`"`, p.closeQuotes('"')-3))
			return b.String(), nil
		case c == '\\':
			rest := strings.TrimLeft(p.src[p.pos+1:], " \t")
			if strings.HasPrefix(rest, "\n") || strings.HasPrefix(rest, "\r\n") {
				p.pos = len(p.src) - len(strings.TrimLeft(rest, " \t\r\n"))
				continue
			}
			if err := p.escape(&b); err != nil {
				return "", err
			}
		case c == '\r' && !p.ahead("\r\n"), c < 0x20 && c != '\t' && c != '\n' && c != '\r', c == 0x7f:
			return "", errTOML
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
	return "", errTOML
}

// literalString reads '...': no escape and no newline.
func (p *tomlParser) literalString() (string, error) {
	p.pos++
	end := strings.IndexAny(p.src[p.pos:], "'\n\r")
	if end < 0 || p.src[p.pos+end] != '\'' {
		return "", errTOML
	}
	s := p.src[p.pos : p.pos+end]
	p.pos += end + 1
	return s, nil
}

// multiLiteral reads a triple-single-quoted string like multiBasic,
// without escapes.
func (p *tomlParser) multiLiteral() (string, error) {
	p.pos += 3
	p.dropFirstNewline()
	end := strings.Index(p.src[p.pos:], "'''")
	if end < 0 {
		return "", errTOML
	}
	s := p.src[p.pos : p.pos+end]
	p.pos += end
	return s + strings.Repeat("'", p.closeQuotes('\'')-3), nil
}

// closeQuotes consumes a closing delimiter with up to two more quotes and
// returns how many quotes it consumed.
func (p *tomlParser) closeQuotes(quote byte) int {
	n := 3
	for n < 5 && p.pos+n < len(p.src) && p.src[p.pos+n] == quote {
		n++
	}
	p.pos += n
	return n
}

func (p *tomlParser) dropFirstNewline() {
	switch {
	case p.peek('\n'):
		p.pos++
	case p.ahead("\r\n"):
		p.pos += 2
	}
}

// escape reads one escape sequence after a backslash.
func (p *tomlParser) escape(b *strings.Builder) error {
	if p.pos+1 >= len(p.src) {
		return errTOML
	}
	c := p.src[p.pos+1]
	p.pos += 2
	if simple, ok := tomlEscapes[c]; ok {
		b.WriteByte(simple)
		return nil
	}
	digits := map[byte]int{'x': 2, 'u': 4, 'U': 8}[c]
	if digits == 0 || p.pos+digits > len(p.src) {
		return errTOML
	}
	hex := p.src[p.pos : p.pos+digits]
	value, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || strings.ContainsAny(hex, "+-_") || !utf8.ValidRune(rune(value)) {
		return errTOML
	}
	p.pos += digits
	b.WriteRune(rune(value))
	return nil
}

var tomlEscapes = map[byte]byte{
	'b': '\b', 't': '\t', 'n': '\n', 'f': '\f', 'r': '\r', 'e': 0x1b, '"': '"', '\\': '\\',
}
