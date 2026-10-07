package secretscan

import (
	"regexp"
	"strings"
)

// selector picks the secret part of one match. m is the submatch index slice
// from FindAllStringSubmatchIndex; a negative start means "no span".
type selector func(s string, m []int) (start, end int)

type detector struct {
	re         *regexp.Regexp
	kind       kind
	selectSpan selector
}

func newDetector(source string, k kind, sel selector) detector {
	return detector{re: regexp.MustCompile(source), kind: k, selectSpan: sel}
}

// detectors copies, verbatim and in order, the 14 regexes of
// evidence.SecretDetectorSources() followed by the 11 patterns of
// security.DefaultPatternSources(). This package imports neither: the drift
// test compares the sources, so an upstream change fails the test.
//
// The qa detectors select the part RedactText replaces; the worker patterns
// redact the whole match. The qa prose exemption is deliberately not applied.
var detectors = []detector{
	// pkg/qa/evidence secretPatterns.
	newDetector(`\bBearer\s+[A-Za-z0-9._~+/=-]{12,}\b`, kindSecret, bearerValue),
	newDetector(`\bsk-(?:proj-)?[A-Za-z0-9_-]{16,}\b`, kindSecret, wholeMatch),
	newDetector(`\bsk-ant-[A-Za-z0-9_-]{16,}\b`, kindSecret, wholeMatch),
	newDetector(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{20,}\b`, kindSecret, wholeMatch),
	newDetector(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`, kindSecret, wholeMatch),
	// sensitiveAssignmentRe: the value group.
	newDetector(`(?i)\b([A-Z0-9_.-]*(TOKEN|SECRET|PASSWORD|PASSWD|PWD|API[_-]?KEY|PRIVATE[_-]?KEY|ACCESS[_-]?KEY|CREDENTIAL|COOKIE|SESSION|AUTH)[A-Z0-9_.-]*)(\s*[:=]\s*)(["']?)([^\s"',}\]]{3,})(["']?)`, kindSecret, group(5)),
	// sensitiveFlagValueRe: the flag value group.
	newDetector(`(?i)(^|[\s"'({\[])(--?[A-Z0-9_.-]*(TOKEN|SECRET|PASSWORD|PASSWD|PWD|API[_-]?KEY|PRIVATE[_-]?KEY|ACCESS[_-]?KEY|CREDENTIAL|COOKIE|SESSION|AUTH|KEY|PASS)[A-Z0-9_.-]*(?:=|\s+))("[^"]*"|'[^']*'|[^\s"',}\]]{3,})`, kindSecret, group(4)),
	// jsonSensitiveRe: the JSON value, inside its quotes.
	newDetector(`(?i)("[^"]*(token|secret|password|passwd|pwd|api[_-]?key|apikey|private[_-]?key|access[_-]?key|credential|cookie|session|authorization|auth)[^"]*"\s*:\s*)("[^"]*"|[^",\n}\]]+)`, kindSecret, unquotedGroup(3)),
	// credentialURLRe: the userinfo between the scheme and the trailing '@'.
	newDetector(`(?i)\b([a-z][a-z0-9+.\-]*://)[^/\s:@]+:[^/\s@]+@`, kindSecret, credentialUserinfo),
	// secretQueryRe: the query value.
	newDetector(`(?i)([?&][^=\s&]*(TOKEN|SECRET|PASSWORD|PASSWD|PWD|API[_-]?KEY|PRIVATE[_-]?KEY|ACCESS[_-]?KEY|CREDENTIAL|COOKIE|SESSION|AUTH|KEY|PASS)[^=\s&]*=)[^&\s"']+`, kindSecret, afterGroup(1)),
	// privateNoteRe and jsonPrivateNoteRe: the note value.
	newDetector(`(?im)\b((local[_ -]?vault[_ -]?note|vault[_ -]?note|private[_ -]?note|private_note_body|note[_ -]?body|localNoteBody|vaultNoteBody|vaultNoteContent)[^:=\n"{}]*\s*[:=]\s*)([^,\n\r}]*)`, kindPrivateNote, group(3)),
	newDetector(`(?i)("[^"]*(localNote|vaultNote|privateNote|noteBody|note_body|noteContent|note_content)[^"]*"\s*:\s*)("[^"]*"|[^",\n}\]]+)`, kindPrivateNote, unquotedGroup(3)),
	// userPathRe and windowsUserPathRe: the user name.
	newDetector(`(file://)?/(Users|home)/([^/\s:"']+)(/[^\s"',)]*)?`, kindUser, group(3)),
	newDetector(`([A-Za-z]:\\+Users\\+)([^\\\s:"']+)((?:\\+[^\s"',)]*)?)`, kindUser, group(2)),

	// pkg/worker/security default patterns.
	newDetector(`sk-[a-zA-Z0-9]{20,}`, kindSecret, wholeMatch),
	newDetector(`AKIA[A-Z0-9]{16}`, kindSecret, wholeMatch),
	newDetector(`ghp_[a-zA-Z0-9]{36}`, kindSecret, wholeMatch),
	newDetector(`gho_[a-zA-Z0-9]{36}`, kindSecret, wholeMatch),
	newDetector(`Bearer [a-zA-Z0-9._\-]+`, kindSecret, wholeMatch),
	newDetector(`(?i)(password|secret|api_key|apikey|token)\s*[=:]\s*\S+`, kindSecret, wholeMatch),
	newDetector(`(?i)(aws|secret).{0,20}[a-zA-Z0-9/+=]{40}`, kindSecret, wholeMatch),
	newDetector(`"private_key[_a-z]*"\s*:\s*"[^"]+`, kindSecret, wholeMatch),
	newDetector(`(?i)azure.{0,20}(client.?secret|tenant.?id)\s*[=:]\s*\S+`, kindSecret, wholeMatch),
	newDetector(`-----BEGIN[A-Z ]*PRIVATE KEY-----`, kindSecret, wholeMatch),
	newDetector(`apjwt_[a-zA-Z0-9_\-]+\.[a-zA-Z0-9_\-]+\.[a-zA-Z0-9_\-]+`, kindSecret, wholeMatch),
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
