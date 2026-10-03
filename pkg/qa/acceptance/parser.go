package acceptance

import (
	"regexp"
	"strings"
)

var (
	// acTokenRe finds an id token. The leading class keeps "TRAC-12" from
	// yielding "AC-12" while still allowing markup such as "**AC-1**".
	acTokenRe  = regexp.MustCompile(`(?:^|[^A-Za-z0-9-])(AC-[A-Za-z0-9][A-Za-z0-9_-]*)`)
	scenarioRe = regexp.MustCompile(`^S(\d+)\s*:\s*(.*)$`)
	listItemRe = regexp.MustCompile(`^(?:[-*+]|\d{1,9}[.)])[ \t]+(?:\[[ xX]\][ \t]+)?(.*)$`)
)

type clauseKind int

const (
	noClause clauseKind = iota
	givenClause
	whenClause
	thenClause
)

// keywords maps each keyword to the clause it fills. AND and BUT map to
// noClause, meaning "extend the most recent clause".
var keywords = []struct {
	word string
	kind clauseKind
}{
	{"given", givenClause}, {"when", whenClause}, {"then", thenClause},
	{"and", noClause}, {"but", noClause},
}

type clauseLine struct {
	kind   clauseKind
	word   string // keyword as written, to tell "AND" from wrapped prose "and"
	text   string
	marked bool // bold markers wrapped the keyword
}

// draft collects one criterion. Each clause keeps segments: a repeated
// keyword or an AND/BUT line opens a segment ("; "-joined) and a wrapped line
// extends the current one (space-joined).
type draft struct {
	c     Criterion
	level int // heading level that opened it; 0 for a list item
	segs  [4][]string
	last  clauseKind
	keep  bool // false for a duplicate: its body is consumed, then dropped
}

type parser struct {
	criteria []Criterion
	problems []Problem
	seen     map[string]bool
	cur      *draft
	fence    string // open code fence marker; "" outside a fence
	cont     bool   // the previous line was a clause or its continuation
}

func (p *parser) feed(n int, raw string) {
	line := strings.TrimSpace(raw)
	if p.fence != "" {
		if strings.HasPrefix(line, p.fence) && strings.Trim(line, p.fence[:1]) == "" {
			p.fence = ""
		}
		return
	}
	if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
		p.fence, p.cont = line[:3], false
		return
	}
	if line == "" {
		p.cont = false
		return
	}
	if level, text, ok := parseHeading(line); ok {
		p.cont = false
		if id, title, ok := headingCriterion(level, text); ok {
			p.open(n, level, id, title)
		} else if p.cur != nil && (p.cur.level == 0 || level <= p.cur.level) {
			// A sibling or parent section ends the criterion so it cannot
			// absorb unrelated GIVEN/WHEN/THEN text further down.
			p.closeCurrent()
		}
		return
	}
	content, isItem := line, false
	if m := listItemRe.FindStringSubmatch(line); m != nil {
		content, isItem = m[1], true
	}
	if cl, ok := parseClause(content); ok && !p.wrappedConjunction(cl, isItem) {
		p.addClause(cl)
		p.cont = true
		return
	}
	if isItem {
		if id, start := acToken(content); id != "" {
			p.open(n, 0, id, titleAfter(content, id, start))
			p.cont = false
			return
		}
	}
	if p.cont {
		p.extend(line)
	}
}

// wrappedConjunction reports a plain lowercase "and"/"but" that continues a
// wrapped sentence. Both readings feed the same clause; reading it as prose
// keeps the word instead of turning ", and the run passes" into ",; the run".
func (p *parser) wrappedConjunction(cl clauseLine, isItem bool) bool {
	return p.cont && !isItem && !cl.marked && cl.kind == noClause && cl.word == strings.ToLower(cl.word)
}

func (p *parser) open(n, level int, id, title string) {
	p.closeCurrent()
	d := &draft{c: Criterion{ID: id, Title: title, Line: n}, level: level, keep: !p.seen[id]}
	if !d.keep {
		p.problems = append(p.problems, Problem{ID: id, Code: CodeDuplicateID, Line: n})
	}
	p.seen[id] = true
	p.cur = d
}

func (p *parser) addClause(cl clauseLine) {
	d := p.cur
	if d == nil {
		return
	}
	kind := cl.kind
	if kind == noClause {
		if kind = d.last; kind == noClause {
			return // AND/BUT with nothing to extend
		}
	}
	d.segs[kind] = append(d.segs[kind], cl.text)
	d.last = kind
}

func (p *parser) extend(text string) {
	d := p.cur
	if d == nil || d.last == noClause {
		return
	}
	segs := d.segs[d.last]
	if i := len(segs) - 1; segs[i] == "" {
		segs[i] = text
	} else {
		segs[i] += " " + text
	}
}

func (p *parser) closeCurrent() {
	d := p.cur
	p.cur = nil
	if d == nil || !d.keep {
		return
	}
	d.c.Given = joinSegments(d.segs[givenClause])
	d.c.When = joinSegments(d.segs[whenClause])
	d.c.Then = joinSegments(d.segs[thenClause])
	if d.c.Then == "" {
		p.problems = append(p.problems, Problem{ID: d.c.ID, Code: CodeMissingThen, Line: d.c.Line})
	}
	p.criteria = append(p.criteria, d.c)
}

func joinSegments(segs []string) string {
	var kept []string
	for _, s := range segs {
		if s = strings.TrimSpace(s); s != "" {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, "; ")
}

// parseHeading recognises an ATX heading and drops an optional closing "#"
// sequence.
func parseHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || (level < len(line) && line[level] != ' ' && line[level] != '\t') {
		return 0, "", false
	}
	text := strings.TrimSpace(line[level:])
	if t := strings.TrimRight(text, "#"); t != text && (t == "" || strings.HasSuffix(t, " ") || strings.HasSuffix(t, "\t")) {
		text = strings.TrimSpace(t)
	}
	return level, text, true
}

// headingCriterion applies REQ-1: "### S<n>: title" names S<n> unless the
// title carries an AC- token, and any level 2-4 heading with an AC- token
// names that token.
func headingCriterion(level int, text string) (string, string, bool) {
	if level == 3 || level == 4 {
		if m := scenarioRe.FindStringSubmatch(text); m != nil {
			if id, start := acToken(m[2]); id != "" {
				return id, titleAfter(m[2], id, start), true
			}
			return "S" + m[1], trimSeparators(m[2]), true
		}
	}
	if level >= 2 && level <= 4 {
		if id, start := acToken(text); id != "" {
			return id, titleAfter(text, id, start), true
		}
	}
	return "", "", false
}

// acToken returns the first AC- id in s and its byte offset. A trailing "_"
// or "-" is markup or a separator ("__AC-1__", "AC-1-"), never part of an id.
func acToken(s string) (string, int) {
	m := acTokenRe.FindStringSubmatchIndex(s)
	if m == nil {
		return "", -1
	}
	return strings.TrimRight(s[m[2]:m[3]], "_-"), m[2]
}

// titleAfter strips a leading id, with any emphasis or brackets around it,
// and the separator after it. A title that mentions the id later stays whole.
func titleAfter(text, id string, start int) string {
	if strings.Trim(text[:start], " \t*_`[(") == "" {
		text = strings.TrimLeft(text[start+len(id):], "*_`])")
	}
	return trimSeparators(text)
}

func trimSeparators(s string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), ":—–- \t"))
}

// parseClause matches a GIVEN/WHEN/THEN/AND/BUT line case-insensitively, with
// optional bold around the keyword and an optional colon after it.
func parseClause(s string) (clauseLine, bool) {
	marker := ""
	if strings.HasPrefix(s, "**") || strings.HasPrefix(s, "__") {
		marker, s = s[:2], s[2:]
	}
	for _, kw := range keywords {
		n := len(kw.word)
		if len(s) < n || !strings.EqualFold(s[:n], kw.word) {
			continue
		}
		rest, colon := strings.CutPrefix(s[n:], ":")
		if marker != "" {
			if after, ok := strings.CutPrefix(rest, marker); ok {
				rest = after
			} else {
				// "**Given a user**": the emphasis spans the whole clause.
				rest = strings.TrimSuffix(strings.TrimRight(rest, " \t"), marker)
			}
			if !colon {
				rest, colon = strings.CutPrefix(rest, ":")
			}
		}
		// The keyword must end the word: "Andrew" and "given/when" are prose.
		if !colon && rest != "" && rest[0] != ' ' && rest[0] != '\t' {
			return clauseLine{}, false
		}
		return clauseLine{kind: kw.kind, word: s[:n], text: strings.TrimSpace(rest), marked: marker != ""}, true
	}
	return clauseLine{}, false
}
