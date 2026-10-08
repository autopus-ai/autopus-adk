package secretscan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// synthAlphabet has no vowels and none of p, s, w, k, or y, so a synthetic
// value can never spell a detector keyword (token, secret, pwd, key, aws, ...)
// and only the fixture's own prefix decides which detector fires.
const (
	synthAlphabet = "BCDFGHJLMNQRTVXZbcdfghjlmnqrtvxz0123456789"
	synthUpper    = "BCDFGHJLMNQRTVXZ0123456789"
)

// synth returns a deterministic n-byte value generated at run time, so no
// secret-shaped literal is committed to the repository.
func synth(n int, alphabet string, seed uint32) string {
	out := make([]byte, n)
	x := seed*2654435761 + 1
	for i := range out {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		out[i] = alphabet[x%uint32(len(alphabet))]
	}
	return string(out)
}

type fixture struct {
	name    string
	in      string
	want    string
	changed bool
}

// s11Fixtures is the acceptance S11 fixture set: 19 format fixtures, 6
// worker-only shapes, 2 overlaps, 6 placeholder-mixed matches, the adjacent
// match, the lone placeholder, and the accepted false positive and clean text.
func s11Fixtures() []fixture {
	v10 := synth(10, synthAlphabet, 10)
	v12 := synth(12, synthAlphabet, 12)
	v24 := synth(24, synthAlphabet, 24)
	v36 := synth(36, synthAlphabet, 36)
	v40 := synth(40, synthAlphabet, 40)
	up16 := synth(16, synthUpper, 16)
	const sec = PlaceholderSecret
	return []fixture{
		// Format fixtures.
		{"bearer", "Bearer " + v24, sec, true},
		{"sk", "sk-" + v24, sec, true},
		{"sk-proj", "sk-proj-" + v24, sec, true},
		{"sk-ant", "sk-ant-" + v24, sec, true},
		{"ghp", "ghp_" + v36, sec, true},
		{"gho", "gho_" + v36, sec, true},
		{"ghu", "ghu_" + v36, sec, true},
		{"ghs", "ghs_" + v36, sec, true},
		{"ghr", "ghr_" + v36, sec, true},
		{"github_pat", "github" + "_pat_" + v40, sec, true},
		{"assignment", "API_KEY=" + v12, sec, true},
		{"flag", "--token " + v12, "--token " + sec, true},
		{"json key", `"api_key": "` + v12 + `"`, `"api_key": "` + sec + `"`, true},
		{"credential url", "https://user:" + v10 + "@example.com", "https://" + sec + "@example.com", true},
		{"query", "?token=" + v12, "?" + sec, true},
		{"user path", "/Users/alice/proj", "/Users/" + PlaceholderUser + "/proj", true},
		{"aws access key", "AKIA" + up16, sec, true},
		{"pem header", "-----BEGIN RSA PRIVATE KEY-----", sec, true},
		{"apjwt", "apjwt_a.b.c", sec, true},
		// Shapes only the worker patterns catch.
		{"short password", "password=ab", sec, true},
		{"azure", "azure client_secret = ab", sec, true},
		{"prose password", "password=letmein was rejected", sec + " was rejected", true},
		{"aws context", "aws secret " + v40, sec, true},
		{"short bearer", "Bearer abc", sec, true},
		{"private_key json", `{"private_key_id": "k1"}`, "{" + sec + `"}`, true},
		// A user-path span overlapping a worker span merges into one secret.
		{"overlap user is aws", "see /Users/aws/" + v40 + " end", "see /Users/" + sec + " end", true},
		{"overlap user pushes window", "aws /Users/bob/" + v40 + " end", sec + " end", true},
		// A placeholder inside a match does not shield the raw value next to it.
		{"mixed assignment", "set password=" + sec + "hunter2xyz end", "set " + sec + " end", true},
		{"mixed aws before", "aws " + sec + " " + v40 + " end", sec + " end", true},
		{"mixed aws after", sec + "; aws " + v40 + " end", sec + " end", true},
		{"mixed password", "password=" + sec + v24 + " end", sec + " end", true},
		{"mixed private note", "vault note: " + PlaceholderPrivateNote + " plus diary text here", "vault note: " + PlaceholderPrivateNote, true},
		{"mixed json value", `{"api_key": "` + sec + ` rawvalue123"}`, `{"api_key": "` + sec + `"}`, true},
		{"adjacent matches", "API_KEY=" + sec + " token=" + v12 + " end", sec + " " + sec + " end", true},
		{"lone placeholder", sec, sec, false},
		// Accepted false positive and text that must stay untouched.
		{"bearer prose", "Bearer token header was dropped by the hook", sec + " header was dropped by the hook", true},
		{"clean prose", "router drops detail mapping for plan", "router drops detail mapping for plan", false},
	}
}

// extraFixtures exercise the remaining span selectors and the merge kind rule.
func extraFixtures() []fixture {
	v12 := synth(12, synthAlphabet, 112)
	v24 := synth(24, synthAlphabet, 124)
	const sec = PlaceholderSecret
	return []fixture{
		{"bearer tab keeps nothing", "Bearer\t" + v24, sec, true},
		{"json unquoted value", `{"token": abc123}`, `{"token": ` + sec + `}`, true},
		{"windows user path", `C:\Users\alice\proj`, `C:\Users\` + PlaceholderUser + `\proj`, true},
		{"json private note", `{"noteBody": "dear diary"}`, `{"noteBody": "` + PlaceholderPrivateNote + `"}`, true},
		{"user span over secret placeholder", "/Users/" + sec + "x/proj", "/Users/" + sec + "/proj", true},
		{"two secrets", "retry with token=" + v12 + " and Bearer " + v24 + " failed", "retry with " + sec + " and " + sec + " failed", true},
		{"multi line", "line one\npassword=hunter2xyz\nline three", "line one\n" + sec + "\nline three", true},
	}
}

func TestRedact_S11Fixtures_ProduceExactOutput(t *testing.T) {
	t.Parallel()
	for _, fx := range s11Fixtures() {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			got, changed := Redact(fx.in)
			assert.Equal(t, fx.want, got)
			assert.Equal(t, fx.changed, changed)
		})
	}
}

func TestRedact_ExtraShapes_ProduceExactOutput(t *testing.T) {
	t.Parallel()
	for _, fx := range extraFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			got, changed := Redact(fx.in)
			assert.Equal(t, fx.want, got)
			assert.Equal(t, fx.changed, changed)
		})
	}
}

func TestRedact_EmptyInput_ReturnsEmptyUnchanged(t *testing.T) {
	t.Parallel()
	got, changed := Redact("")
	assert.Empty(t, got)
	assert.False(t, changed)
}

// TestRedact_Properties_HoldForEveryFixture asserts the three S11 properties:
// re-applying Redact is a no-op, the spans a pass collects lie inside a
// placeholder in the output, and no 8-byte fragment of a raw secret span
// (placeholder bytes excluded) survives. A pass collects its spans on the
// placeholder-neutral copy, so text after a placeholder is not secret context.
func TestRedact_Properties_HoldForEveryFixture(t *testing.T) {
	t.Parallel()
	for _, fx := range append(append(s11Fixtures(), extraFixtures()...), reviewFixtures()...) {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			out, _ := Redact(fx.in)

			again, changed := Redact(out)
			assert.Equal(t, out, again, "Redact must be idempotent")
			assert.False(t, changed, "second pass must not replace anything")

			inPlaceholder := placeholderMask(out)
			for _, sp := range spansOf(out, placeholderTokens(out)) {
				for i := sp.start; i < sp.end; i++ {
					if !inPlaceholder[i] {
						t.Fatalf("span [%d,%d) of output %q lies outside a placeholder", sp.start, sp.end, out)
					}
				}
			}

			for _, fragment := range secretFragments(fx.in, 8) {
				assert.NotContains(t, out, fragment, "raw secret fragment survived")
			}
		})
	}
}

// placeholderMask marks every byte of s that belongs to a canonical placeholder.
func placeholderMask(s string) []bool {
	mask := make([]bool, len(s))
	for _, tok := range placeholderTokens(s) {
		for i := tok.start; i < tok.end; i++ {
			mask[i] = true
		}
	}
	return mask
}

// secretFragments returns every n-byte window of the spans a pass collects in
// s, skipping placeholder bytes, which are not raw secret material.
func secretFragments(s string, n int) []string {
	inPlaceholder := placeholderMask(s)
	var fragments []string
	for _, sp := range spansOf(s, placeholderTokens(s)) {
		var run strings.Builder
		flush := func() {
			r := run.String()
			for i := 0; i+n <= len(r); i++ {
				fragments = append(fragments, r[i:i+n])
			}
			run.Reset()
		}
		for i := sp.start; i < sp.end; i++ {
			if inPlaceholder[i] {
				flush()
				continue
			}
			run.WriteByte(s[i])
		}
		flush()
	}
	return fragments
}
