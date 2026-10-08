package secretscan

import "strings"

// workerPlaceholder is what the worker SecretScanner writes for a match.
const workerPlaceholder = "***REDACTED***"

// rewrite is how the upstream redactor of a detector rewrites one match.
type rewrite int

const (
	// rewriteSpan writes the kind's placeholder over the selected span.
	rewriteSpan rewrite = iota
	// rewriteUnredacted is rewriteSpan unless the span already holds
	// "[REDACTED"; RedactText skips such assignment and flag values.
	rewriteUnredacted
	// rewriteJSONValue writes the placeholder in quotes: a quoted value
	// keeps its quotes, an unquoted one (group 3) is replaced with new ones.
	rewriteJSONValue
	// rewriteUserName is rewriteSpan unless the name is already
	// [REDACTED_USER].
	rewriteUserName
	// rewriteWhole writes the worker placeholder over the whole match.
	rewriteWhole
)

// trace is text an upstream redactor is rewriting step by step. origin[i] is
// where byte i stands in the scanned text: its own index for a kept byte, the
// start of the replaced range for a placeholder byte. Kept bytes and
// placeholders tile the scanned text in order, so bytes [start, end) stand
// for the scanned range from at(start) to at(end).
type trace struct {
	text   string
	origin []int
	size   int
}

func newTrace(s string) trace {
	origin := make([]int, len(s))
	for i := range origin {
		origin[i] = i
	}
	return trace{text: s, origin: origin, size: len(s)}
}

// at is the scanned position byte i of the text stands at.
func (t trace) at(i int) int {
	if i < len(t.origin) {
		return t.origin[i]
	}
	return t.size
}

// replaySpans replays both upstream redactors on s, each on its own copy:
// RedactText's detectors, then separately the worker patterns, each in table
// order. It returns the spans of s their replacements removed. Both rewrite
// text step by step, so a later step can match across an earlier placeholder:
// a worker value runs on through ***REDACTED***, and a qa JSON or query key
// can read [REDACTED_SECRET]. Masking these spans too keeps Redact a superset
// of what either redactor removes.
func replaySpans(s string) []span {
	var spans []span
	for _, worker := range []bool{false, true} {
		t := newTrace(s)
		for _, d := range detectors {
			if (d.rewrite == rewriteWhole) == worker {
				t, spans = d.replay(t, spans)
			}
		}
	}
	return spans
}

// replay rewrites every match of d in t the way its upstream redactor does
// and appends the scanned range each replacement removed.
func (d detector) replay(t trace, spans []span) (trace, []span) {
	matches := d.re.FindAllStringSubmatchIndex(t.text, -1)
	if len(matches) == 0 {
		return t, spans
	}
	var b strings.Builder
	origin := make([]int, 0, len(t.origin))
	last := 0
	for _, m := range matches {
		start, end, insert, ok := d.rewriteMatch(t.text, m)
		if !ok {
			continue
		}
		lo, hi := t.at(start), t.at(end)
		if hi > lo {
			spans = append(spans, span{start: lo, end: hi, kind: d.kind})
		}
		b.WriteString(t.text[last:start])
		b.WriteString(insert)
		origin = append(origin, t.origin[last:start]...)
		for range len(insert) {
			origin = append(origin, lo)
		}
		last = end
	}
	b.WriteString(t.text[last:])
	origin = append(origin, t.origin[last:]...)
	return trace{text: b.String(), origin: origin, size: t.size}, spans
}

// rewriteMatch returns the bytes [start, end) of text the upstream redactor
// replaces for match m and what it writes there, or false when it leaves the
// match alone.
func (d detector) rewriteMatch(text string, m []int) (int, int, string, bool) {
	if d.rewrite == rewriteWhole {
		return m[0], m[1], workerPlaceholder, true
	}
	start, end := d.selectSpan(text, m)
	if start < 0 {
		return 0, 0, "", false
	}
	insert, value := d.kind.placeholder(), text[start:end]
	switch {
	case d.rewrite == rewriteUnredacted && strings.Contains(value, "[REDACTED"),
		d.rewrite == rewriteUserName && value == PlaceholderUser:
		return 0, 0, "", false
	case d.rewrite == rewriteJSONValue && start == m[6]:
		insert = `"` + insert + `"`
	}
	return start, end, insert, true
}
