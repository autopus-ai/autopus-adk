package healthband

import "regexp"

// bandSecret is one band-specific redaction: every match becomes replace.
type bandSecret struct {
	pattern *regexp.Regexp
	replace string
}

// bandSecretPatterns extend the shared promptlayer redaction with the secret
// forms a CI log or a provider reply carries that the shared list misses
// (security review M2). Sanitize applies them to the whole captured text
// before SanitizeContent and before any cut, and a match records
// secret_risk. A URL keeps its scheme and host so the line stays readable.
var bandSecretPatterns = []bandSecret{
	// Private key blocks: PEM of any key type (hyphens included), PGP's
	// "PRIVATE KEY BLOCK", and the four-dash SSH2 form.
	{regexp.MustCompile(`(?is)` + keyMarkerPrefix + `BEGIN` + keyMarkerSuffix + `.*?` + keyMarkerPrefix + `END` + keyMarkerSuffix), redactedSecret},
	// JSON members whose key names a credential, whatever the value holds.
	{regexp.MustCompile(`(?i)"[a-z0-9_.-]*(?:pass(?:word|wd)?|pwd|secret|token|api[_-]?key|access[_-]?key|account[_-]?key|private[_-]?key|credential)[a-z0-9_.-]*"\s*:\s*"(?:[^"\\]|\\.){4,}"`), redactedSecret},
	// URL credentials: scheme://user:password@host.
	{regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^\s/:@]+:[^\s/@]+@`), "${1}" + redactedSecret + "@"},
	// Authorization headers of every scheme gh, git, and HTTP clients print.
	{regexp.MustCompile(`(?i)\b(?:proxy-)?authorization\s*:\s*(?:token|basic|bearer|digest)\s+[A-Za-z0-9._~+/=-]{6,}`), redactedSecret},
	// JSON Web Tokens: three base64url segments, the first a JSON header.
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), redactedSecret},
	// Azure storage and service bus connection string keys.
	{regexp.MustCompile(`(?i)\b(?:Account|SharedAccess)Key\s*=\s*[A-Za-z0-9+/=]{16,}`), redactedSecret},
	// Prefixed tokens: GitLab, GitHub (shorter forms too), sk- API keys,
	// Slack, and Stripe.
	{regexp.MustCompile(`\b(?:glpat-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{16,}|xox[abposr]-[A-Za-z0-9-]{10,}|[rs]k_(?:live|test)_[A-Za-z0-9]{16,})`), redactedSecret},
}

// Key marker parts shared by the block pattern and the orphan-marker rule:
// four or five dashes, an optional space, BEGIN or END, the key type, and
// PRIVATE KEY with PGP's optional BLOCK.
const (
	keyMarkerPrefix = `-{4,5} ?`
	keyMarkerSuffix = ` [A-Z0-9 _-]*PRIVATE KEY(?: BLOCK)? ?-{4,5}`
)

// redactBandSecrets applies bandSecretPatterns and reports whether any matched.
func redactBandSecrets(text string) (string, bool) {
	redacted := false
	for _, secret := range bandSecretPatterns {
		if secret.pattern.MatchString(text) {
			text, redacted = secret.pattern.ReplaceAllString(text, secret.replace), true
		}
	}
	return text, redacted
}
