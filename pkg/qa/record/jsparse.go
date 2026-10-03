package record

import (
	"errors"
	"fmt"
)

type valueKind int

const (
	valString valueKind = iota
	valNumber
	valBool
	valObject
	valChain
)

// jsValue is one argument value: a literal, an object literal, or a member
// chain such as page.getByText('x').
type jsValue struct {
	kind   valueKind
	text   string
	truth  bool
	fields []jsField
	chain  []segment
}

type jsField struct {
	key   string
	value jsValue
}

// segment is one link of a member chain: a name, optionally called.
type segment struct {
	name string
	call bool
	args []jsValue
}

type jsParser struct {
	toks []token
	pos  int
}

// parseAwait parses `await <chain>;`, the only statement shape codegen emits
// for a recorded step, and returns the chain.
func parseAwait(stmt string) ([]segment, error) {
	toks, err := lexJS(stmt)
	if err != nil {
		return nil, err
	}
	p := &jsParser{toks: toks}
	if t := p.peek(); t.kind != tokIdent || t.text != "await" {
		return nil, errors.New("only awaited page and expect calls are recorded")
	}
	p.pos++
	chain, err := p.chain()
	if err != nil {
		return nil, err
	}
	if p.punct(";") {
		p.pos++
	}
	if p.peek().kind != tokEOF {
		return nil, errors.New("unexpected syntax after the call")
	}
	return chain.chain, nil
}

func (p *jsParser) peek() token {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return token{kind: tokEOF}
}

func (p *jsParser) punct(text string) bool {
	t := p.peek()
	return t.kind == tokPunct && t.text == text
}

func (p *jsParser) value() (jsValue, error) {
	t := p.peek()
	switch {
	case t.kind == tokString:
		p.pos++
		return jsValue{kind: valString, text: t.text}, nil
	case t.kind == tokNumber:
		p.pos++
		return jsValue{kind: valNumber, text: t.text}, nil
	case t.kind == tokIdent && (t.text == "true" || t.text == "false"):
		p.pos++
		return jsValue{kind: valBool, truth: t.text == "true"}, nil
	case t.kind == tokIdent:
		return p.chain()
	case p.punct("{"):
		return p.object()
	}
	return jsValue{}, fmt.Errorf("unsupported argument %q", t.text)
}

// chain parses an identifier followed by member accesses and calls.
func (p *jsParser) chain() (jsValue, error) {
	root := p.peek()
	if root.kind != tokIdent {
		return jsValue{}, errors.New("expected a call")
	}
	p.pos++
	segs := []segment{{name: root.text}}
	for {
		switch {
		case p.punct("("):
			last := &segs[len(segs)-1]
			if last.call {
				return jsValue{}, errors.New("a call result cannot be called again")
			}
			args, err := p.args()
			if err != nil {
				return jsValue{}, err
			}
			last.call, last.args = true, args
		case p.punct("."):
			p.pos++
			name := p.peek()
			if name.kind != tokIdent {
				return jsValue{}, errors.New("expected a member name after '.'")
			}
			p.pos++
			segs = append(segs, segment{name: name.text})
		default:
			return jsValue{kind: valChain, chain: segs}, nil
		}
	}
}

// args parses a parenthesised, comma-separated argument list.
func (p *jsParser) args() ([]jsValue, error) {
	p.pos++ // the opening parenthesis, checked by the caller
	var out []jsValue
	for !p.punct(")") {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		if p.punct(",") {
			p.pos++
		} else if !p.punct(")") {
			return nil, errors.New("expected ',' or ')'")
		}
	}
	p.pos++
	return out, nil
}

// object parses an object literal of name: value options.
func (p *jsParser) object() (jsValue, error) {
	p.pos++ // the opening brace, checked by the caller
	var fields []jsField
	for !p.punct("}") {
		key := p.peek()
		if key.kind != tokIdent && key.kind != tokString {
			return jsValue{}, errors.New("expected an option name")
		}
		p.pos++
		if !p.punct(":") {
			return jsValue{}, errors.New("expected ':' after an option name")
		}
		p.pos++
		v, err := p.value()
		if err != nil {
			return jsValue{}, err
		}
		fields = append(fields, jsField{key: key.text, value: v})
		if p.punct(",") {
			p.pos++
		} else if !p.punct("}") {
			return jsValue{}, errors.New("expected ',' or '}'")
		}
	}
	p.pos++
	return jsValue{kind: valObject, fields: fields}, nil
}

// isFirstPick reports whether s is .first() or .nth(0), the two ways a chain
// narrows a locator to its first match.
func isFirstPick(s segment) bool {
	switch {
	case !s.call:
		return false
	case s.name == "first":
		return len(s.args) == 0
	case s.name == "nth":
		return len(s.args) == 1 && s.args[0].kind == valNumber && s.args[0].text == "0"
	}
	return false
}
