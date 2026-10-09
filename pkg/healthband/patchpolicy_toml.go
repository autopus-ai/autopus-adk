package healthband

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// A TOML 1.0 reader for the Cargo manifests that Patch Policy item 6 reads
// at the base (RR-7). It builds the whole document, so a key counts only
// where TOML puts it: `proc-macro = true` inside a string, a comment, or
// another table is not the [lib] key. Anything it cannot read is errTOML,
// which the caller treats as a manifest band cannot reason about. It
// accepts a superset of TOML 1.0 only where that cannot change what a valid
// document means: newlines, comments, and a trailing comma inside an inline
// table (TOML 1.1), the \e and \xHH escapes (TOML 1.1), and control
// characters in comments and literal strings.

var errTOML = errors.New("healthband: manifest is not TOML that band reads")

// tomlMaxDepth bounds the nesting of arrays and inline tables.
const tomlMaxDepth = 64

// tomlTable is one table: a [header], an inline table, an element of an
// array of tables, or one that a header or a dotted key created implicitly.
type tomlTable struct {
	kv       map[string]any
	explicit bool // defined by its own [header]
	dotted   bool // defined by a dotted key
	inline   bool // an inline table, closed to every later key
}

// tomlArray is an array value, or an array of tables ([[header]]).
type tomlArray struct {
	items  []any
	tables bool
}

// tomlOther is a number, a date, or a time, which no rule reads.
type tomlOther struct{}

func newTOMLTable() *tomlTable { return &tomlTable{kv: map[string]any{}} }

type tomlParser struct {
	src   string
	pos   int
	depth int
	root  *tomlTable
}

// parseTOML reads src, valid UTF-8 with an optional byte order mark.
func parseTOML(src string) (*tomlTable, error) {
	if !utf8.ValidString(src) {
		return nil, errTOML
	}
	p := &tomlParser{src: strings.TrimPrefix(src, "\ufeff"), root: newTOMLTable()}
	current := p.root
	for {
		p.skipSpace()
		if p.eof() {
			return p.root, nil
		}
		var err error
		switch c := p.src[p.pos]; {
		case c == '\n' || c == '\r' || c == '#':
		case c == '[':
			current, err = p.header()
		default:
			err = p.keyValue(current)
		}
		if err == nil {
			err = p.endLine()
		}
		if err != nil {
			return nil, err
		}
	}
}

func (p *tomlParser) eof() bool           { return p.pos >= len(p.src) }
func (p *tomlParser) peek(c byte) bool    { return !p.eof() && p.src[p.pos] == c }
func (p *tomlParser) ahead(s string) bool { return strings.HasPrefix(p.src[p.pos:], s) }

func (p *tomlParser) skipSpace() {
	for p.peek(' ') || p.peek('\t') {
		p.pos++
	}
}

// endLine reads the rest of a line: blanks, a comment, then LF, CRLF, or
// the end of the document.
func (p *tomlParser) endLine() error {
	p.skipSpace()
	if p.peek('#') {
		for !p.eof() && !p.peek('\n') && !p.ahead("\r\n") {
			p.pos++
		}
	}
	switch {
	case p.eof():
	case p.peek('\n'):
		p.pos++
	case p.ahead("\r\n"):
		p.pos += 2
	default:
		return errTOML
	}
	return nil
}

// skipBlank skips blanks, newlines, and comments inside an array or an
// inline table.
func (p *tomlParser) skipBlank() error {
	for !p.eof() {
		switch c := p.src[p.pos]; {
		case c == ' ' || c == '\t' || c == '\n':
			p.pos++
		case c == '\r':
			if !p.ahead("\r\n") {
				return errTOML
			}
			p.pos += 2
		case c == '#':
			for !p.eof() && !p.peek('\n') && !p.ahead("\r\n") {
				p.pos++
			}
		default:
			return nil
		}
	}
	return nil
}

// header reads [key] or [[key]] and returns the table it opens.
func (p *tomlParser) header() (*tomlTable, error) {
	p.pos++
	array := p.peek('[')
	if array {
		p.pos++
	}
	p.skipSpace()
	keys, err := p.key()
	if err != nil {
		return nil, err
	}
	closing := "]"
	if array {
		closing = "]]"
	}
	if !p.ahead(closing) {
		return nil, errTOML
	}
	p.pos += len(closing)
	return p.root.open(keys, array)
}

// open walks a header's keys from the root: a missing table on the way is
// created implicitly, an array of tables gives its last element, and the
// last key is defined once (or, for [[key]], gets one more element).
func (t *tomlTable) open(keys []string, array bool) (*tomlTable, error) {
	for _, k := range keys[:len(keys)-1] {
		switch v := t.kv[k].(type) {
		case nil:
			next := newTOMLTable()
			t.kv[k], t = next, next
		case *tomlTable:
			if v.inline {
				return nil, errTOML
			}
			t = v
		case *tomlArray:
			if !v.tables {
				return nil, errTOML
			}
			t = v.items[len(v.items)-1].(*tomlTable)
		default:
			return nil, errTOML
		}
	}
	last := keys[len(keys)-1]
	next := newTOMLTable()
	switch v := t.kv[last].(type) {
	case nil:
		if array {
			t.kv[last] = &tomlArray{items: []any{next}, tables: true}
		} else {
			next.explicit = true
			t.kv[last] = next
		}
		return next, nil
	case *tomlTable:
		if array || v.explicit || v.dotted || v.inline {
			return nil, errTOML
		}
		v.explicit = true
		return v, nil
	case *tomlArray:
		if !array || !v.tables {
			return nil, errTOML
		}
		v.items = append(v.items, next)
		return next, nil
	}
	return nil, errTOML
}

// keyValue reads `key = value` into t.
func (p *tomlParser) keyValue(t *tomlTable) error {
	keys, err := p.key()
	if err != nil {
		return err
	}
	p.skipSpace()
	if !p.peek('=') {
		return errTOML
	}
	p.pos++
	p.skipSpace()
	value, err := p.value()
	if err != nil {
		return err
	}
	return t.assign(keys, value)
}

// assign sets a dotted key once; a dotted key extends only tables that
// dotted keys created.
func (t *tomlTable) assign(keys []string, value any) error {
	for _, k := range keys[:len(keys)-1] {
		switch v := t.kv[k].(type) {
		case nil:
			next := newTOMLTable()
			next.dotted = true
			t.kv[k], t = next, next
		case *tomlTable:
			if !v.dotted || v.inline {
				return errTOML
			}
			t = v
		default:
			return errTOML
		}
	}
	last := keys[len(keys)-1]
	if _, taken := t.kv[last]; taken {
		return errTOML
	}
	t.kv[last] = value
	return nil
}

// key reads a dotted key: simple keys joined by dots, blanks allowed
// around each dot.
func (p *tomlParser) key() ([]string, error) {
	var keys []string
	for {
		k, err := p.simpleKey()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
		p.skipSpace()
		if !p.peek('.') {
			return keys, nil
		}
		p.pos++
		p.skipSpace()
	}
}

func (p *tomlParser) simpleKey() (string, error) {
	switch {
	case p.ahead(`"""`) || p.ahead("'''"):
		return "", errTOML
	case p.peek('"'):
		return p.basicString()
	case p.peek('\''):
		return p.literalString()
	}
	start := p.pos
	for !p.eof() && bareKeyChar(p.src[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return "", errTOML
	}
	return p.src[start:p.pos], nil
}

func bareKeyChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}
