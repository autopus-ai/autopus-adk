// Package secretscan redacts secret-shaped text before the learn store
// persists it or a CLI command prints it (SPEC-HARNEVAL-002 REQ-HC-01).
//
// The detector table is the union of the pkg/qa/evidence redaction regexes
// and the pkg/worker/security default patterns. Every detector runs on the
// same text; the selected spans are merged and each merged span is replaced
// once. Applying the detectors one after another instead lets an early
// replacement erase the context a later detector needs, which leaks the value
// that detector would have caught.
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

// filler stands in for placeholder bytes in the neutral copy the detectors
// scan. It is neither a word nor a space character, and no detector keyword,
// token alphabet, or value terminator contains it.
const filler = '#'

// alphabetFiller stands in for placeholder bytes in the copy the
// fixed-alphabet detectors also scan. A digit is a letter of every such
// alphabet and of no keyword, so a placeholder inside a token reads as part
// of it and the raw characters glued to it join the same match.
const alphabetFiller = '0'

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
// reports whether it replaced anything. It is deterministic and idempotent:
// it repeats its pass until the pass changes nothing, so Redact applied to its
// own output returns that output unchanged.
//
// A span that overlaps a placeholder already in s is widened to that
// placeholder's bounds and redacted as a whole, so a placeholder cannot shield
// raw text inside the same or an adjacent match. Only a widened span that is
// exactly one placeholder is skipped.
func Redact(s string) (string, bool) { return redactWithin(s, 2*len(s)+1) }

// redactWithin runs at most passes passes. A changing pass lowers twice the
// raw bytes plus the placeholders of the text, so 2*len(s)+1 passes always
// reach the fixpoint; a text still changing past the cap is one secret.
func redactWithin(s string, passes int) (string, bool) {
	out := s
	for range passes {
		next, changed := redactPass(out)
		if !changed {
			return out, out != s
		}
		out = next
	}
	return PlaceholderSecret, true
}

// redactPass replaces each merged span once. Every changing pass removes raw
// bytes or joins placeholders, so the passes Redact repeats always end.
func redactPass(s string) (string, bool) {
	tokens := placeholderTokens(s)
	var spans []span
	for _, sp := range spansOf(s, tokens) {
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

// spansOf returns the spans one pass redacts in s, whose placeholders are
// tokens. The detectors and the upstream replay scan a neutral copy in which
// every placeholder is filler, so a placeholder is read as an opaque value:
// raw text glued to it joins its value, and its own letters (the SECRET of
// [REDACTED_SECRET]) are no keyword. Filler is outside every fixed alphabet,
// so those detectors also scan a copy filled with alphabetFiller. A span of s
// itself is added when it lies clear of every placeholder, or when it
// overlaps one and also overlaps a span of a neutral copy, which lets a match
// that starts inside a placeholder absorb it into the raw secret next to it.
// A span that only placeholder text explains is dropped, so the text after a
// placeholder is not redacted again.
func spansOf(s string, tokens []span) []span {
	scan := neutral(s, tokens, filler)
	spans := append(detect(scan), replaySpans(scan)...)
	if len(tokens) == 0 {
		return spans
	}
	spans = append(spans, glue(s, tokens, detectAlphabet(neutral(s, tokens, alphabetFiller)))...)
	confirmed := merge(append([]span(nil), spans...))
	for _, sp := range detect(s) {
		if !overlapsAny(sp, tokens) || overlapsAny(sp, confirmed) {
			spans = append(spans, sp)
		}
	}
	return spans
}

// neutral returns s with every token's bytes replaced by fill; positions are
// unchanged, so a span of the copy is a span of s.
func neutral(s string, tokens []span, fill byte) string {
	if len(tokens) == 0 {
		return s
	}
	b := []byte(s)
	for _, tok := range tokens {
		for i := tok.start; i < tok.end; i++ {
			b[i] = fill
		}
	}
	return string(b)
}

// glue extends every span that ends inside a placeholder, or at its end,
// over the letters and digits glued after that placeholder. A fixed-length
// value (AKIA and 16 characters) reads the placeholder as the rest of it and
// would otherwise stop there, leaving the raw characters after it.
func glue(s string, tokens, spans []span) []span {
	for i, sp := range spans {
		j := sort.Search(len(tokens), func(j int) bool { return tokens[j].end >= sp.end })
		if j == len(tokens) || tokens[j].start >= sp.end {
			continue
		}
		end := tokens[j].end
		for end < len(s) && isAlnum(s[end]) {
			end++
		}
		spans[i].end = max(sp.end, end)
	}
	return spans
}

func isAlnum(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// overlapsAny reports whether sp overlaps one of the disjoint, sorted spans.
func overlapsAny(sp span, sorted []span) bool {
	i := sort.Search(len(sorted), func(i int) bool { return sorted[i].end > sp.start })
	return i < len(sorted) && sorted[i].start < sp.end
}

// widen grows sp to the bounds of every placeholder it overlaps. The span
// also takes the kind of a covered placeholder when that kind is higher, so a
// replacement never relabels secret material as a lower kind.
func widen(sp span, tokens []span) span {
	i := sort.Search(len(tokens), func(i int) bool { return tokens[i].end > sp.start })
	for ; i < len(tokens) && tokens[i].start < sp.end; i++ {
		sp.start = min(sp.start, tokens[i].start)
		sp.end = max(sp.end, tokens[i].end)
		sp.kind = max(sp.kind, tokens[i].kind)
	}
	return sp
}

// merge sorts spans by start and joins spans that overlap or touch.
func merge(spans []span) []span {
	if len(spans) == 0 {
		return nil
	}
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
