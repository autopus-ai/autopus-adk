package secretscan

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// selector picks the secret part of one match. m is the submatch index slice
// from FindAllStringSubmatchIndex; a negative start means "no span".
type selector func(s string, m []int) (start, end int)

// detector is one table row: the regex, the kind of span it yields, the part
// of a match Redact selects, and how its upstream redactor rewrites a match.
// prepare, when set, builds selectSpan for one scanned text, so a selector
// that looks past its match indexes the text once instead of once per match.
// alphabet marks a value drawn from a fixed alphabet (Bearer, sk-, ghp_,
// AKIA, ...) that the filler of the neutral copy is not part of.
type detector struct {
	re         *regexp.Regexp
	kind       kind
	selectSpan selector
	prepare    func(s string) selector
	alphabet   bool
	rewrite    rewrite
}

func newDetector(source string, k kind, sel selector, rw rewrite) detector {
	return detector{re: regexp.MustCompile(source), kind: k, selectSpan: sel, rewrite: rw}
}

// preparedBy returns d with prepare set.
func (d detector) preparedBy(prepare func(s string) selector) detector {
	d.prepare = prepare
	return d
}

// ofAlphabet returns d with alphabet set.
func (d detector) ofAlphabet() detector {
	d.alphabet = true
	return d
}

// detectors copies, verbatim and in order, the 14 regexes of
// evidence.SecretDetectorSources() followed by the 11 patterns of
// security.DefaultPatternSources(). This package imports neither: the drift
// test compares the sources, so an upstream change fails the test.
//
// The qa detectors select the part RedactText replaces; the worker patterns
// redact the whole match, a private key header with its key body. The qa
// prose exemption is deliberately not applied.
var detectors = []detector{
	// pkg/qa/evidence secretPatterns.
	newDetector(`\bBearer\s+[A-Za-z0-9._~+/=-]{12,}\b`, kindSecret, bearerValue, rewriteSpan).ofAlphabet(),
	newDetector(`\bsk-(?:proj-)?[A-Za-z0-9_-]{16,}\b`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	newDetector(`\bsk-ant-[A-Za-z0-9_-]{16,}\b`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	newDetector(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{20,}\b`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	newDetector(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	// sensitiveAssignmentRe: the value group.
	newDetector(`(?i)\b([A-Z0-9_.-]*(TOKEN|SECRET|PASSWORD|PASSWD|PWD|API[_-]?KEY|PRIVATE[_-]?KEY|ACCESS[_-]?KEY|CREDENTIAL|COOKIE|SESSION|AUTH)[A-Z0-9_.-]*)(\s*[:=]\s*)(["']?)([^\s"',}\]]{3,})(["']?)`, kindSecret, group(5), rewriteUnredacted),
	// sensitiveFlagValueRe: the flag value group.
	newDetector(`(?i)(^|[\s"'({\[])(--?[A-Z0-9_.-]*(TOKEN|SECRET|PASSWORD|PASSWD|PWD|API[_-]?KEY|PRIVATE[_-]?KEY|ACCESS[_-]?KEY|CREDENTIAL|COOKIE|SESSION|AUTH|KEY|PASS)[A-Z0-9_.-]*(?:=|\s+))("[^"]*"|'[^']*'|[^\s"',}\]]{3,})`, kindSecret, group(4), rewriteUnredacted),
	// jsonSensitiveRe: the JSON value, inside its quotes.
	newDetector(`(?i)("[^"]*(token|secret|password|passwd|pwd|api[_-]?key|apikey|private[_-]?key|access[_-]?key|credential|cookie|session|authorization|auth)[^"]*"\s*:\s*)("[^"]*"|[^",\n}\]]+)`, kindSecret, unquotedGroup(3), rewriteJSONValue),
	// credentialURLRe: the userinfo between the scheme and the trailing '@'.
	newDetector(`(?i)\b([a-z][a-z0-9+.\-]*://)[^/\s:@]+:[^/\s@]+@`, kindSecret, credentialUserinfo, rewriteSpan),
	// secretQueryRe: the query value.
	newDetector(`(?i)([?&][^=\s&]*(TOKEN|SECRET|PASSWORD|PASSWD|PWD|API[_-]?KEY|PRIVATE[_-]?KEY|ACCESS[_-]?KEY|CREDENTIAL|COOKIE|SESSION|AUTH|KEY|PASS)[^=\s&]*=)[^&\s"']+`, kindSecret, afterGroup(1), rewriteSpan),
	// privateNoteRe and jsonPrivateNoteRe: the note value.
	newDetector(`(?im)\b((local[_ -]?vault[_ -]?note|vault[_ -]?note|private[_ -]?note|private_note_body|note[_ -]?body|localNoteBody|vaultNoteBody|vaultNoteContent)[^:=\n"{}]*\s*[:=]\s*)([^,\n\r}]*)`, kindPrivateNote, group(3), rewriteSpan),
	newDetector(`(?i)("[^"]*(localNote|vaultNote|privateNote|noteBody|note_body|noteContent|note_content)[^"]*"\s*:\s*)("[^"]*"|[^",\n}\]]+)`, kindPrivateNote, unquotedGroup(3), rewriteJSONValue),
	// userPathRe and windowsUserPathRe: the user name.
	newDetector(`(file://)?/(Users|home)/([^/\s:"']+)(/[^\s"',)]*)?`, kindUser, group(3), rewriteUserName),
	newDetector(`([A-Za-z]:\\+Users\\+)([^\\\s:"']+)((?:\\+[^\s"',)]*)?)`, kindUser, group(2), rewriteSpan),

	// pkg/worker/security default patterns.
	newDetector(`sk-[a-zA-Z0-9]{20,}`, kindSecret, wholeMatch, rewriteWhole).ofAlphabet(),
	newDetector(`AKIA[A-Z0-9]{16}`, kindSecret, wholeMatch, rewriteWhole).ofAlphabet(),
	newDetector(`ghp_[a-zA-Z0-9]{36}`, kindSecret, wholeMatch, rewriteWhole).ofAlphabet(),
	newDetector(`gho_[a-zA-Z0-9]{36}`, kindSecret, wholeMatch, rewriteWhole).ofAlphabet(),
	newDetector(`Bearer [a-zA-Z0-9._\-]+`, kindSecret, wholeMatch, rewriteWhole).ofAlphabet(),
	newDetector(`(?i)(password|secret|api_key|apikey|token)\s*[=:]\s*\S+`, kindSecret, wholeMatch, rewriteWhole),
	newDetector(`(?i)(aws|secret).{0,20}[a-zA-Z0-9/+=]{40}`, kindSecret, wholeMatch, rewriteWhole).ofAlphabet(),
	newDetector(`"private_key[_a-z]*"\s*:\s*"[^"]+`, kindSecret, wholeMatch, rewriteWhole),
	newDetector(`(?i)azure.{0,20}(client.?secret|tenant.?id)\s*[=:]\s*\S+`, kindSecret, wholeMatch, rewriteWhole),
	newDetector(`-----BEGIN[A-Z ]*PRIVATE KEY-----`, kindSecret, pemBlock, rewriteWhole).preparedBy(pemBlocks),
	newDetector(`apjwt_[a-zA-Z0-9_\-]+\.[a-zA-Z0-9_\-]+\.[a-zA-Z0-9_\-]+`, kindSecret, wholeMatch, rewriteWhole).ofAlphabet(),
}

// Bounds of the extra searches one detector makes inside its own matches:
// they may scan researchFactor times the text plus researchFloor bytes.
const (
	researchFactor = 8
	researchFloor  = 4096
)

// detect returns the span every detector selects in s. After each match a
// detector searches again from inside the part it selected (one byte past the
// match start when it selects the whole match), so a value that runs over the
// next key cannot hide that key's own match; FindAll alone resumes after the
// value. Past the search bound, the rest of the text from where the searches
// stopped is one secret span: deeply nested input is over-redacted instead of
// scanned quadratically.
func detect(s string) []span {
	var spans []span
	for _, d := range detectors {
		spans = d.collect(s, spans)
	}
	return spans
}

// detectAlphabet returns the spans the fixed-alphabet detectors select in s.
func detectAlphabet(s string) []span {
	var spans []span
	for _, d := range detectors {
		if d.alphabet {
			spans = d.collect(s, spans)
		}
	}
	return spans
}

// collect appends the spans of d's matches in s, nested matches included.
func (d detector) collect(s string, spans []span) []span {
	matches := d.re.FindAllStringSubmatchIndex(s, -1)
	if len(matches) > 0 && d.prepare != nil {
		d.selectSpan = d.prepare(s)
	}
	budget := researchFactor*len(s) + researchFloor
	for _, m := range matches {
		spans = d.add(s, m, spans)
		for from := d.restart(s, m); ; {
			loc := d.re.FindStringSubmatchIndex(s[from:])
			scanned := len(s) - from
			if loc != nil {
				shift(loc, from)
				scanned = loc[1] - from
			}
			if budget -= scanned; budget < 0 {
				return append(spans, span{start: from, end: len(s), kind: kindSecret})
			}
			if loc == nil || loc[0] >= m[1] {
				break
			}
			spans = d.add(s, loc, spans)
			from = d.restart(s, loc)
		}
	}
	return spans
}

// add appends the non-empty span d selects from match m.
func (d detector) add(s string, m []int, spans []span) []span {
	start, end := d.selectSpan(s, m)
	if start < 0 || end <= start {
		return spans
	}
	return append(spans, span{start: start, end: end, kind: d.kind})
}

// restart is where the search for matches nested in m begins: the start of
// the selected part, or one byte past the match start.
func (d detector) restart(s string, m []int) int {
	if start, _ := d.selectSpan(s, m); start > m[0] {
		return start
	}
	return m[0] + 1
}

// shift moves the submatch indexes of a match found in s[from:] onto s.
func shift(loc []int, from int) {
	for i := range loc {
		if loc[i] >= 0 {
			loc[i] += from
		}
	}
}

func wholeMatch(_ string, m []int) (int, int) { return m[0], m[1] }

// group selects submatch n; a group that did not participate yields -1.
func group(n int) selector {
	return func(_ string, m []int) (int, int) { return m[2*n], m[2*n+1] }
}

// unquotedGroup selects submatch n without its surrounding double quotes, so
// a JSON string value keeps its quotes around the placeholder.
func unquotedGroup(n int) selector {
	return func(s string, m []int) (int, int) {
		start, end := m[2*n], m[2*n+1]
		if start >= 0 && end-start >= 2 && s[start] == '"' && s[end-1] == '"' {
			return start + 1, end - 1
		}
		return start, end
	}
}

// afterGroup selects from the end of submatch n to the end of the match.
func afterGroup(n int) selector {
	return func(_ string, m []int) (int, int) { return m[2*n+1], m[1] }
}

// credentialUserinfo selects the userinfo after the scheme group, excluding
// the trailing '@' that ends the match.
func credentialUserinfo(_ string, m []int) (int, int) { return m[3], m[1] - 1 }

// bearerValue mirrors RedactText: a match that starts with "Bearer " keeps
// the keyword and redacts the rest; any other match is redacted whole.
func bearerValue(s string, m []int) (int, int) {
	const prefix = "Bearer "
	if strings.HasPrefix(s[m[0]:m[1]], prefix) {
		return m[0] + len(prefix), m[1]
	}
	return m[0], m[1]
}

// pemFooter and pemBody bound the key material after a private key header.
// Without a footer the material is the RFC 1421 header lines of an encrypted
// key (Proc-Type: 4,ENCRYPTED and DEK-Info: <cipher>,<iv>), each on its own
// line or flattened onto one, then the base64 body. A header name or value
// never holds "--", so the material cannot run over the dashes of the next
// private key header, and each header scans only up to it.
var (
	pemFooter = regexp.MustCompile(`-----END[A-Z ]*PRIVATE KEY-----`)
	pemBody   = regexp.MustCompile(`^(?:\s*[A-Za-z](?:[A-Za-z0-9]|-[A-Za-z0-9])*:[ \t]*(?:[A-Za-z0-9+/=,]|-[A-Za-z0-9+/=,])*)*[A-Za-z0-9+/=\s]*`)
)

// pemBlock extends a private key header over its key material: through the
// footer when one follows, or else over the header lines and base64 body
// after the header, trailing whitespace excluded. It indexes s for one match;
// collect prepares the selector once per text with pemBlocks instead.
func pemBlock(s string, m []int) (int, int) { return pemBlocks(s)(s, m) }

// pemBlocks returns the pemBlock selector for s. It finds every footer in s
// once, overlapping ones included, so a header looks up the first footer
// starting at or after its end, the one a search from there finds, instead of
// searching the rest of the text again: that made many headers without a
// footer quadratic.
func pemBlocks(s string) selector {
	var footers [][2]int
	for from := 0; ; {
		loc := pemFooter.FindStringIndex(s[from:])
		if loc == nil {
			break
		}
		footers = append(footers, [2]int{from + loc[0], from + loc[1]})
		from += loc[0] + 1
	}
	return func(_ string, m []int) (int, int) {
		i := sort.Search(len(footers), func(i int) bool { return footers[i][0] >= m[1] })
		if i < len(footers) {
			return m[0], footers[i][1]
		}
		body := strings.TrimRightFunc(pemBody.FindString(s[m[1]:]), unicode.IsSpace)
		return m[0], m[1] + len(body)
	}
}
