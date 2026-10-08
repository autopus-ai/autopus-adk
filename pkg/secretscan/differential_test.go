package secretscan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/qa/evidence"
	"github.com/insajin/autopus-adk/pkg/worker/security"
)

// corpusCase is one random input and the unique secret value it ends with.
type corpusCase struct {
	in, secret string
}

// Pieces of the random corpus: the key, separator, prefix, and suffix shapes
// the security review combined to find leaks. A filler value is drawn from
// synthAlphabet, so only the pieces decide which detector fires.
var (
	corpusKeys = []string{"password", "session", "auth", "token", "api_key", "secret", "cookie", "credential",
		"pwd", "DB_PASSWD", "private_key", "access-key", "aws_secret_access_key", "client_secret",
		"Authorization", "refresh_token", "azure client_secret", "note_body", "vault note", "localNote",
		"Bearer", "key", "pass", "id", "x"}
	corpusSeps = []string{"=", ": ", " = ", " ", "\":\"", "\": \"", "=\"", "='", "%3D", "\t", "?", "&", "--", "-", ":"}
	corpusPres = []string{"", " ", "\"", "'", "{", "[", "(", "--", "-", "?", "&", "export ", "https://user:",
		"https://u:", "curl -H \"", "/Users/dev/", "/home/ci/", "C:\\Users\\", "Bearer ", "sk-", "ghp_", "AKIA",
		"sk-ant-", "github_pat_", "apjwt_", "aws "}
	corpusPosts        = []string{"", " ", "\"", "'", "}", "]", ",", "@host", "/x", ".a.b", " end", "\n", ";", "&x=1"}
	corpusPlaceholders = []string{PlaceholderSecret, PlaceholderUser, PlaceholderPrivateNote}
)

// randomCorpus returns n deterministic inputs. Each joins one to three
// prefix-key-separator groups, some followed by a filler value, then a unique
// secret Q...Z and a suffix. withPlaceholders mixes canonical placeholders into
// the prefixes and suffixes.
func randomCorpus(n int, seed uint32, withPlaceholders bool) []corpusCase {
	x := seed*2654435761 + 7
	pick := func(k int) int {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		return int(x % uint32(k))
	}
	pres, posts := corpusPres, corpusPosts
	if withPlaceholders {
		pres = append(append([]string{}, pres...), corpusPlaceholders...)
		posts = append(append([]string{}, posts...), corpusPlaceholders...)
	}
	cases := make([]corpusCase, 0, n)
	for i := range n {
		var b strings.Builder
		for group := 1 + pick(3); group > 0; group-- {
			b.WriteString(pres[pick(len(pres))] + corpusKeys[pick(len(corpusKeys))] + corpusSeps[pick(len(corpusSeps))])
			if pick(3) == 0 {
				b.WriteString(synth(4+pick(8), synthAlphabet, uint32(i*7+group)) + posts[pick(len(posts))])
			}
		}
		if pick(3) == 0 {
			b.WriteString(pres[pick(len(pres))])
		}
		secret := "Q" + synth(10+pick(40), synthAlphabet, seed+uint32(i)) + "Z"
		b.WriteString(secret + posts[pick(len(posts))])
		cases = append(cases, corpusCase{in: b.String(), secret: secret})
	}
	return cases
}

// leaks reports whether text still holds an 8-byte window of secret.
func leaks(text, secret string) bool {
	for i := 0; i+8 <= len(secret); i++ {
		if strings.Contains(text, secret[i:i+8]) {
			return true
		}
	}
	return false
}

// TestRedact_RandomCorpus_CoversQAAndWorkerRedactions is the differential
// superset check on placeholder-free random input: whenever
// evidence.RedactText or the worker SecretScanner leaves no 8-byte window of
// the secret, Redact leaves none either. Both upstream redactors rewrite text
// step by step, so a later step can match across an earlier placeholder; the
// corpus is built to reach those shapes. Input that already holds a
// placeholder is left out on purpose: the worker reads the SECRET of
// [REDACTED_SECRET] as AWS context and masks the text after it, which Redact
// deliberately does not (the stored value must stay stable on a second pass).
func TestRedact_RandomCorpus_CoversQAAndWorkerRedactions(t *testing.T) {
	t.Parallel()
	worker := security.NewSecretScanner()
	failures := 0
	for _, c := range append(randomCorpus(3000/corpusScale, 11, false), randomCorpus(3000/corpusScale, 13, false)...) {
		out, _ := Redact(c.in)
		if !leaks(out, c.secret) {
			continue
		}
		for name, upstream := range map[string]string{"qa evidence": evidence.RedactText(c.in), "worker scanner": worker.Scan(c.in)} {
			if !leaks(upstream, c.secret) {
				failures++
				if failures <= 5 {
					t.Errorf("%s removes the secret of %q but Redact keeps it:\n redact   %q\n upstream %q", name, c.in, out, upstream)
				}
			}
		}
	}
	assert.Zero(t, failures, "random inputs where Redact keeps what an upstream redactor removes")
}
