package secretscan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// localCase is one input built at run time and the value that must not
// survive Redact. Prefixes are joined from fragments and values come from
// synth, so no secret-shaped literal is committed.
type localCase struct{ name, in, secret string }

func localCases() []localCase {
	v := func(n int, seed uint32) string { return synth(n, synthAlphabet, seed) }
	b64 := v(24, 1) + "=="
	slack := "xo" + "xb-" + v(12, 2) + "-" + v(12, 3) + "-" + v(24, 4)
	stripe := "sk" + "_live_" + v(24, 5)
	restricted := "rk" + "_live_" + v(24, 6)
	google := "AI" + "za" + v(35, 7)
	gitlab := "gl" + "pat-" + v(20, 8)
	npm := "np" + "m_" + v(36, 9)
	jwt := "ey" + "J" + v(17, 10) + ".ey" + "J" + v(17, 11) + "." + v(22, 12)
	pgpBody := v(40, 13)
	pgpBegin := "-----BEGIN PGP " + "PRIVATE KEY BLOCK-----"
	pgpEnd := "-----END PGP " + "PRIVATE KEY BLOCK-----"
	cookie := v(20, 14)
	pass := v(14, 15)
	return []localCase{
		{"authorization basic", "Authorization: Basic " + b64, b64},
		{"authorization quoted header", `curl -H "Authorization: Digest ` + b64 + `" x`, b64},
		{"slack token", "notify via " + slack, slack},
		{"stripe live key", "charge failed for " + stripe, stripe},
		{"stripe restricted key", "use " + restricted + " now", restricted},
		{"google api key", "maps 403 for " + google, google},
		{"gitlab pat", "clone with " + gitlab + " failed", gitlab},
		{"npm token", "publish " + npm + " denied", npm},
		{"bare jwt", "session " + jwt + " rejected", jwt},
		{"pgp block", pgpBegin + "\n\n" + pgpBody + "\n" + pgpEnd, pgpBody},
		{"pgp block without footer", "x\n" + pgpBegin + "\n\n" + pgpBody + "\n", pgpBody},
		{"second cookie", "Cookie: theme=dark; sid=" + cookie, cookie},
		{"set-cookie", "Set-Cookie: sid=" + cookie + "; Path=/", cookie},
		{"curl user", "curl -u admin:" + pass + " https://api.example.com", pass},
		{"curl long user", "curl --user admin:" + pass + " https://x", pass},
		{"quoted value with spaces", `password: "correct horse ` + pass + `"`, pass},
		{"single quoted value", `api_key = 'a b ` + pass + `'`, pass},
	}
}

// TestRedact_LocalShapes_LeaveNoValue covers the shapes neither upstream
// redactor catches: each value is gone and the output is a fixpoint.
func TestRedact_LocalShapes_LeaveNoValue(t *testing.T) {
	t.Parallel()
	for _, c := range localCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, changed := Redact(c.in)
			assert.True(t, changed)
			assert.NotContains(t, out, c.secret)
			again, changedAgain := Redact(out)
			assert.Equal(t, out, again)
			assert.False(t, changedAgain)
		})
	}
}

// TestRedact_LocalShapes_KeepContext pins what stays readable around a
// redacted value.
func TestRedact_LocalShapes_KeepContext(t *testing.T) {
	t.Parallel()
	pass := synth(14, synthAlphabet, 21)
	got, _ := Redact(`db_passwd: "a b ` + pass + `"`)
	assert.Equal(t, `db_passwd: "`+PlaceholderSecret+`"`, got)

	got, _ = Redact("curl -u admin:" + pass + " https://example.com/x")
	assert.Equal(t, "curl -u "+PlaceholderSecret+" https://example.com/x", got)

	got, _ = Redact("Cookie: a=" + pass + "; b=" + pass + "\nnext line")
	assert.Equal(t, "Cookie: "+PlaceholderSecret+"\nnext line", got)
}

// TestRedact_LocalShapes_LeaveProseAlone keeps ordinary prose that shares a
// prefix or keyword with a local shape unchanged.
func TestRedact_LocalShapes_LeaveProseAlone(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"Basic authentication is disabled",
		"the npm_config cache was cleared",
		"run sort -u names.txt",
		"eyJ is the base64 of a JSON object prefix",
	} {
		got, changed := Redact(in)
		assert.False(t, changed, in)
		assert.Equal(t, in, got)
	}
}
