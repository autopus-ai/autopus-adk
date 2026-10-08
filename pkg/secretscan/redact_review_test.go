package secretscan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// reviewFixtures pin the SPEC-HARNEVAL-002 review findings: text after a
// placeholder is not read as secret context (C2), a placeholder does not
// shield the raw value glued to it (S1), a value that runs over the next key
// does not hide that key (S3), and a private key header masks its key body
// (S4). Round 2 adds the header lines of an encrypted key without a footer and
// placeholders glued inside a fixed-alphabet token.
func reviewFixtures() []fixture {
	v12 := synth(12, synthAlphabet, 212)
	v20 := synth(20, synthAlphabet, 220)
	raw36 := synth(36, synthAlphabet, 236)
	up8 := synth(8, synthUpper, 208)
	body := synth(64, synthAlphabet, 264)
	tail := synth(24, synthAlphabet, 224)
	iv := synth(32, "0123456789ABCDEF", 232)
	sha := strings.Repeat("0123456789abcdef", 3)[:40]
	header, footer := "-----BEGIN RSA PRIVATE KEY-----", "-----END RSA PRIVATE KEY-----"
	procType, dekInfo := "Proc-Type: 4,ENCRYPTED", "DEK-Info: AES-128-CBC,"+iv
	const sec = PlaceholderSecret
	return []fixture{
		// C2: a commit SHA after a redacted value is not an AWS secret.
		{"sha after assignment", "token=abc fixed in " + sha, sec + " fixed in " + sha, true},
		{"sha after password", "password=hunter2 see commit " + sha, sec + " see commit " + sha, true},
		{"sha after bearer", "Bearer abc then " + sha, sec + " then " + sha, true},
		{"sha after placeholder", sec + " fixed in " + sha, sec + " fixed in " + sha, false},
		// S1: the raw value glued to a placeholder joins the redacted value.
		{"session shield", "session=" + sec + v20, "session=" + sec, true},
		{"auth shield", "auth=" + sec + v20, "auth=" + sec, true},
		{"cookie shield", "cookie: " + sec + v20, "cookie: " + sec, true},
		{"credential shield", "credential=" + sec + v20, "credential=" + sec, true},
		{"passwd shield", "DB_PASSWD=" + sec + v20, "DB_PASSWD=" + sec, true},
		{"pwd shield", "pwd=" + sec + v20, "pwd=" + sec, true},
		{"flag session shield", "--session " + sec + v20, "--session " + sec, true},
		{"flag pass shield", "--db-pass=" + sec + v20, "--db-pass=" + sec, true},
		{"user placeholder shield", "--cookie " + PlaceholderUser + v20, "--cookie " + sec, true},
		// S3: the key a value swallowed still has its own value redacted.
		{"bearer then two keys", "Bearer token: secret: hunter2xyz", sec, true},
		{"value swallows key", "token: secret: " + v12 + " end", sec + " end", true},
		{"key nested in value", "x_token=a:session: " + v12, "x_" + sec + " " + sec, true},
		// S4: the key body goes with its header.
		{"pem block", header + " " + body + " " + footer, sec, true},
		{"pem block in prose", "pem then " + header + " " + body + " " + footer + " rotated", "pem then " + sec + " rotated", true},
		{"pem body without footer", header + "\n" + body + "\n" + tail, sec, true},
		{"pem header then punctuation", header + " " + body + ".", sec + ".", true},
		// Round 2: the Proc-Type and DEK-Info lines go with the body they open.
		{"encrypted pem without footer", header + "\n" + procType + "\n" + dekInfo + "\n\n" + body + "\n", sec + "\n", true},
		{"flattened encrypted pem", header + " " + procType + " " + dekInfo + " " + body + ".", sec + ".", true},
		{"encrypted pem with footer", header + "\n" + procType + "\n" + dekInfo + "\n\n" + body + "\n" + footer + " done", sec + " done", true},
		// Round 2: a placeholder inside a fixed-alphabet token reads as letters
		// of it, so the raw characters glued to it join the token.
		{"bearer glued", "Bearer " + sec + raw36[:24], sec, true},
		{"bearer glued in header", "Authorization: Bearer " + sec + raw36[:24], "Authorization: " + sec, true},
		{"bearer split by placeholder", "Bearer " + raw36[:6] + sec + raw36[:24], sec, true},
		{"bearer keyword before placeholder", "Bearer " + sec, sec, true},
		{"sk glued", "sk-" + sec + raw36[:24], sec, true},
		{"ghp glued", "ghp_" + sec + raw36, sec, true},
		{"github_pat glued", "github" + "_pat_" + sec + raw36[:24], sec, true},
		{"akia glued", "AKIA" + sec + up8, sec, true},
		{"aws value split", "aws " + v20 + sec + v20 + " end", sec + " end", true},
		{"apjwt glued", "apjwt_" + sec + "." + v12 + "." + v12, sec, true},
	}
}

func TestRedact_ReviewFixtures_ProduceExactOutput(t *testing.T) {
	t.Parallel()
	for _, fx := range reviewFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			got, changed := Redact(fx.in)
			assert.Equal(t, fx.want, got)
			assert.Equal(t, fx.changed, changed)
		})
	}
}

// TestRedact_ReappliedToItsOutput_ChangesNothing is the idempotence property
// over every fixture and a random corpus with and without placeholders:
// Redact(Redact(x)) == Redact(x), so a value stored redacted reads the same
// when intake or promote checks it again.
func TestRedact_ReappliedToItsOutput_ChangesNothing(t *testing.T) {
	t.Parallel()
	inputs := make([]string, 0, 4096)
	for _, fx := range append(append(s11Fixtures(), extraFixtures()...), reviewFixtures()...) {
		inputs = append(inputs, fx.in)
	}
	for _, c := range randomCorpus(1500/corpusScale, 41, false) {
		inputs = append(inputs, c.in)
	}
	for _, c := range randomCorpus(1500/corpusScale, 43, true) {
		inputs = append(inputs, c.in)
	}
	failures := 0
	for _, in := range inputs {
		once, _ := Redact(in)
		twice, changed := Redact(once)
		if once != twice || changed {
			failures++
			if failures <= 5 {
				t.Errorf("not idempotent for %q:\n once  %q\n twice %q", in, once, twice)
			}
		}
	}
	assert.Zero(t, failures, "inputs whose redaction changes when applied again")
}

// TestRedact_DeeplyNestedKeys_RedactTheRestOfTheText covers the bound on the
// nested-match searches: thousands of keys nested in one value would make
// each search scan the rest of the text, so past the bound the rest of the
// text from where the searches stopped is one secret span.
func TestRedact_DeeplyNestedKeys_RedactTheRestOfTheText(t *testing.T) {
	t.Parallel()
	got, changed := Redact("note " + strings.Repeat("token=", 4000) + "end")
	assert.Equal(t, "note "+PlaceholderSecret, got)
	assert.True(t, changed)
}

// TestRedact_PassCapReached_RedactsTheWholeText covers the defensive cap on
// the fixpoint loop (review round 2): a text still changing when the passes
// run out is redacted whole instead of returned half done.
func TestRedact_PassCapReached_RedactsTheWholeText(t *testing.T) {
	t.Parallel()
	const in = "note token=abc end"

	got, changed := redactWithin(in, 2)
	assert.Equal(t, "note "+PlaceholderSecret+" end", got, "the second pass confirms the fixpoint")
	assert.True(t, changed)

	got, changed = redactWithin(in, 1)
	assert.Equal(t, PlaceholderSecret, got, "one pass changes the text and cannot confirm it")
	assert.True(t, changed)

	got, changed = redactWithin("router drops detail mapping", 1)
	assert.Equal(t, "router drops detail mapping", got, "clean text needs only the confirming pass")
	assert.False(t, changed)
}
