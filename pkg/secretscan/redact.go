// Package secretscan redacts secret-shaped text before the learn store
// persists it or a CLI command prints it (SPEC-HARNEVAL-002 REQ-HC-01).
//
// The detector table is the union of the pkg/qa/evidence redaction regexes
// and the pkg/worker/security default patterns. Every detector runs on the
// original text; the selected spans are merged and each merged span is
// replaced once. Applying the detectors one after another instead lets an
// early replacement erase the context a later detector needs, which leaks the
// value that detector would have caught.
package secretscan

import (
	"sort"
	"strings"
)

// Placeholders written in place of a redacted span. They match the
// pkg/qa/evidence placeholders so redacted text reads the same everywhere.
const (
	PlaceholderSecret      = "[REDACTED_SECRET]"
	PlaceholderPrivateNote = "[REDACTED_PRIVATE_NOTE]"
	PlaceholderUser        = "[REDACTED_USER]"
)

// kind orders span sensitivity; a merged span takes the highest kind.
type kind int

const (
	kindUser kind = iota + 1
	kindPrivateNote
	kindSecret
)

var placeholders = [...]struct {
	text string
	kind kind
}{
	{PlaceholderSecret, kindSecret},
	{PlaceholderPrivateNote, kindPrivateNote},
	{PlaceholderUser, kindUser},
}

func (k kind) placeholder() string {
	switch k {
	case kindSecret:
		return PlaceholderSecret
	case kindPrivateNote:
		return PlaceholderPrivateNote
	default:
		return PlaceholderUser
	}
}

// span is a half-open byte range [start, end) of the scanned text.
type span struct {
	start, end int
	kind       kind
}

// Redact returns s with every detected secret replaced by a placeholder and
// reports whether it replaced anything. It is deterministic.
//
// A span that overlaps a placeholder already in s is widened to that
// placeholder's bounds and redacted as a whole, so a placeholder cannot shield
// raw text inside the same or an adjacent match. Only a widened span that is
// exactly one placeholder is skipped, which keeps redacted text stable.
func Redact(s string) (string, bool) {
	if s == "" {
		return s, false
	}
	tokens := placeholderTokens(s)
	var spans []span
	for _, sp := range detect(s) {
		sp = widen(sp, tokens)
		if isPlaceholder(s[sp.start:sp.end]) {
			continue
		}
		spans = append(spans, sp)
	}
	if len(spans) == 0 {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, sp := range merge(spans) {
		b.WriteString(s[last:sp.start])
		b.WriteString(sp.kind.placeholder())
		last = sp.end
	}
	b.WriteString(s[last:])
	return b.String(), true
}

// detect returns the raw span every detector selects in s.
func detect(s string) []span {
	var spans []span
	for _, d := range detectors {
		for _, m := range d.re.FindAllStringSubmatchIndex(s, -1) {
			start, end := d.selectSpan(s, m)
			if start < 0 || end <= start {
				continue
			}
			spans = append(spans, span{start: start, end: end, kind: d.kind})
		}
	}
	return spans
}

// widen grows sp to the bounds of every placeholder it overlaps. The span
// also takes the kind of a covered placeholder when that kind is higher, so a
// replacement never relabels secret material as a lower kind.
func widen(sp span, tokens []span) span {
	for _, tok := range tokens {
		if tok.start < sp.end && sp.start < tok.end {
			sp.start = min(sp.start, tok.start)
			sp.end = max(sp.end, tok.end)
			sp.kind = max(sp.kind, tok.kind)
		}
	}
	return sp
}

// merge sorts spans by start and joins spans that overlap or touch.
func merge(spans []span) []span {
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end > spans[j].end
	})
	merged := []span{spans[0]}
	for _, sp := range spans[1:] {
		last := &merged[len(merged)-1]
		if sp.start <= last.end {
			last.end = max(last.end, sp.end)
			last.kind = max(last.kind, sp.kind)
			continue
		}
		merged = append(merged, sp)
	}
	return merged
}

// placeholderTokens returns the canonical placeholders in s in text order.
// Placeholders cannot overlap: each opens with '[' and holds no other '['.
func placeholderTokens(s string) []span {
	var tokens []span
	for i := 0; i < len(s); {
		j := strings.Index(s[i:], "[REDACTED_")
		if j < 0 {
			break
		}
		i += j
		width := 1
		for _, p := range placeholders {
			if strings.HasPrefix(s[i:], p.text) {
				tokens = append(tokens, span{start: i, end: i + len(p.text), kind: p.kind})
				width = len(p.text)
				break
			}
		}
		i += width
	}
	return tokens
}

func isPlaceholder(s string) bool {
	for _, p := range placeholders {
		if s == p.text {
			return true
		}
	}
	return false
}
