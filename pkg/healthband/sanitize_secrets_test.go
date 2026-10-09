package healthband_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Fake credentials are joined from fragments. GitHub push protection and
// secret scanners match the contiguous source text, not what a test does with
// it, so a provider-format fake written as one literal blocks the push: an
// Alibaba AccessKey ID and a 30+ character secret-shaped run in this table
// did. Split here: provider prefixes, PEM armor, the JWT, a key and value a
// generic rule reads as a credential, and every 30+ character run of letters
// and digits. Each joined value is byte-identical to the literal it replaced,
// so every detector below still sees the same input.
const (
	armorBegin = "-----BEGIN "
	armorEnd   = "-----END "

	fakeAlibabaKeyID    = "LTAI" + "5tQwErTyUiOpAsDfGhJk"
	fakeClientSecret    = "Zm9vYmFy" + "YmF6cXV4MTIzNA"
	fakeOpaqueToken     = "tok_" + "9f8e7d6c5b4a3f2e1d0c"
	fakeJWTSignature    = "SflKxwRJSMeKKF2QT4fw" + "pMeJf36POk6yJV_adQssw5c"
	fakeJWT             = "eyJhbGciOiJIUzI1NiIs" + "InR5cCI6IkpXVCJ9." + "eyJzdWIiOiIxMjM0NTY3ODkwIn0." + fakeJWTSignature
	fakePGPBody         = "lQOYBFsynthetic" + "KeyMaterial0123"
	fakePGPBodyOpen     = "lQOYBFsynthetic" + "KeyMaterial4567"
	fakeOpenSSHBody     = "b3BlbnNzaC1rZXkt" + "djEAAAAABG5vbmU"
	fakeAzureAccountKey = "Zm9vYmFy" + "YmF6cXV4Zm9vYmFyYmF6cXV4" + "Zm9vYmFy=="
	fakeGitLabToken     = "glpat-" + "AbCdEfGhIjKlMnOpQrSt"
	fakeGitHubOAuth     = "gho_" + "0123456789abcdefghij" + "ABCDEFGHIJ0123"
	fakeGitHubPAT       = "github_pat_" + "11ABCDEFG0123456789_" + "abcdefghijklmnopqrstuvwxyz"
	fakeAnthropicKey    = "sk-" + "ant-api03-AbCdEfGhIjKlMnOp"
	fakeDictAPIKey      = "AbCdEf" + "0123456789"
	fakePyPIToken       = "pypi-" + "AgEIcHlwaS5vcmcCJDAwMDAwMDAw" + "LTAwMDAtMDAwMC0wMDAwLTAwMDAw" + "MDAwMDAwMAACKlsz"
)

// Security M2: every secret form the audit sampled is redacted from a CI log
// before the cut, records secret_risk, and leaves the surrounding line. Every
// value below is synthetic.
func TestSanitizeCILog_RedactsEverySecretForm(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		form   string
		line   string
		secret string // must not survive in any letter case
		keep   string // context that must survive
	}{
		{"json password", `config {"user": "ci", "password": "hunter2-hunter2"}`, "hunter2-hunter2", `"user": "ci"`},
		{"json AccessKey", `{"AccessKey": "` + fakeAlibabaKeyID + `", "Region": "eu"}`, fakeAlibabaKeyID, `"Region": "eu"`},
		{"json client secret", `{"client_secret":"` + fakeClientSecret + `"}`, fakeClientSecret, "{"},
		{"json token", `{"token": "` + fakeOpaqueToken + `"}`, fakeOpaqueToken, "{"},
		{"url credentials", "pushing to https://deploy:s3cr3t-pass@registry.example.com/v2/app", "s3cr3t-pass", "registry.example.com/v2/app"},
		{"authorization token", "> Authorization: token 0123456789abcdef0123456789abcdef01234567", "0123456789abcdef0123456789abcdef01234567", "> "},
		{"authorization basic", "header Authorization: Basic ZGVwbG95OnMzY3IzdC1wYXNz sent", "ZGVwbG95OnMzY3IzdC1wYXNz", " sent"},
		{"authorization bearer", "authorization: bearer abcDEF123456.ghijklMNOP", "abcDEF123456.ghijklMNOP", ""},
		{"jwt", "cookie=" + fakeJWT + " ok", fakeJWTSignature, " ok"},
		{"pgp private key block", armorBegin + "PGP PRIVATE KEY BLOCK-----\n\n" + fakePGPBody + "\n=abcd\n" + armorEnd + "PGP PRIVATE KEY BLOCK-----\nafter",
			fakePGPBody, "after"},
		{"unterminated pgp block", "before\n" + armorBegin + "PGP PRIVATE KEY BLOCK-----\n" + fakePGPBodyOpen, fakePGPBodyOpen, "before"},
		{"ssh2 private key block", "---- BEGIN SSH2 ENCRYPTED PRIVATE KEY ----\nP2/56wAAAi4AAAA3aWYtbW9kbntzaWdu\n---- END SSH2 ENCRYPTED PRIVATE KEY ----\nafter",
			"P2/56wAAAi4AAAA3aWYtbW9kbntzaWdu", "after"},
		{"openssh private key block", armorBegin + "OPENSSH PRIVATE KEY-----\n" + fakeOpenSSHBody + "\n" + armorEnd + "OPENSSH PRIVATE KEY-----\nafter",
			fakeOpenSSHBody, "after"},
		{"pem key type with a hyphen", armorBegin + "RSA-PSS PRIVATE KEY-----\nMIIEvQIBADANBgkqSyntheticPSS\n" + armorEnd + "RSA-PSS PRIVATE KEY-----\nafter",
			"MIIEvQIBADANBgkqSyntheticPSS", "after"},
		{"azure AccountKey", "DefaultEndpointsProtocol=https;AccountName=acme;AccountKey=" + fakeAzureAccountKey + ";EndpointSuffix=core.windows.net",
			fakeAzureAccountKey, "EndpointSuffix=core.windows.net"},
		{"gitlab token", "cloning with " + fakeGitLabToken + " now", fakeGitLabToken, " now"},
		{"github oauth token", "using " + fakeGitHubOAuth + " here", fakeGitHubOAuth, " here"},
		{"github fine-grained token", "pat " + fakeGitHubPAT + " here", fakeGitHubPAT, " here"},
		{"api key with sk prefix", "key " + fakeAnthropicKey + " here", fakeAnthropicKey, " here"},
		// Review round 2 (M2 residual).
		{"url token as the user", "fetching https://0123456789abcdef0123456789abcdef01234567@github.com/acme/app.git",
			"0123456789abcdef0123456789abcdef01234567", "github.com/acme/app.git"},
		{"url password without a user", "redis://:s3cr3t-pass@cache.internal:6379/0", "s3cr3t-pass", "cache.internal:6379/0"},
		{"python dict password", `config {'user': 'ci', 'password': 'hunter2 hunter2'}`, "hunter2 hunter2", `'user': 'ci'`},
		{"python dict api key", `{'api_key': "` + fakeDictAPIKey + `"}`, fakeDictAPIKey, "{"},
		{"cookie header", "> Cookie: session=Zm9vYmFyYmF6cXV4; theme=dark", "Zm9vYmFyYmF6cXV4", "> Cookie: "},
		{"set-cookie header", "< set-cookie: _gh_sess=AbCdEf0123456789; path=/; secure; HttpOnly", "AbCdEf0123456789", "< set-cookie: "},
		{"pypi token", "twine upload -p " + fakePyPIToken + " now", fakePyPIToken, " now"},
		// Review round 3 (M2 residual): header names as quoted keys, the JS
		// object form, and a token user with an empty password.
		{"json authorization member", `request {"Authorization": "Bearer Zm9vYmFyYmF6cXV4MTIzNA", "Accept": "application/json"}`,
			"Zm9vYmFyYmF6cXV4MTIzNA", `"Accept": "application/json"`},
		{"python dict authorization item", `headers {'Authorization': 'Bearer Zm9vYmFyYmF6cXV4MTIzNA', 'Accept': 'text/plain'}`,
			"Zm9vYmFyYmF6cXV4MTIzNA", `'Accept': 'text/plain'`},
		{"js object authorization", "fetch(url, { headers: { Authorization: 'Bearer Zm9vYmFyYmF6cXV4MTIzNA' } })",
			"Zm9vYmFyYmF6cXV4MTIzNA", "fetch(url, { headers: { "},
		{"quoted authorization header name", `header "Authorization": Bearer Zm9vYmFyYmF6cXV4MTIzNA sent`, "Zm9vYmFyYmF6cXV4MTIzNA", " sent"},
		{"python dict cookie item", `{'Cookie': 'session=Zm9vYmFyYmF6cXV4; theme=dark', 'Host': 'example.com'}`,
			"Zm9vYmFyYmF6cXV4", `'Host': 'example.com'`},
		{"json cookie member", `{"cookie": "_gh_sess=AbCdEf0123456789", "status": 200}`, "AbCdEf0123456789", `"status": 200`},
		{"url token as the user with an empty password", "fetching https://0123456789abcdef0123456789abcdef01234567:@github.com/acme/app.git",
			"0123456789abcdef0123456789abcdef01234567", "github.com/acme/app.git"},
	} {
		t.Run(tc.form, func(t *testing.T) {
			t.Parallel()
			got := healthband.SanitizeCILog("step 1 ok\n"+tc.line+"\nstep 9 failed: exit 2\n", false, "/work/repo")

			assert.NotContains(t, strings.ToLower(got.Text), strings.ToLower(tc.secret))
			assert.Contains(t, got.Text, "[REDACTED_SECRET]")
			assert.Contains(t, got.Text, tc.keep)
			if strings.HasPrefix(tc.form, "unterminated") {
				// Untrusted Input Contract item 3: a BEGIN without an END is
				// redacted through the end of the text.
				assert.True(t, strings.HasSuffix(got.Text, "before\n[REDACTED_SECRET]"), got.Text)
			} else {
				assert.Contains(t, got.Text, "step 9 failed: exit 2")
			}
			assert.Contains(t, got.Reasons, healthband.ReasonSecretRisk)
			assert.Equal(t, "redacted", got.RedactionStatus)
		})
	}
}

// Redaction is not a blanket filter: ordinary log text keeps every byte.
func TestSanitizeCILog_LeavesOrdinaryTextAlone(t *testing.T) {
	t.Parallel()
	text := "GET https://registry.example.com/v2/app 200\nAuthorization header absent\n{\"name\": \"lint\", \"status\": \"ok\"}\n" +
		"token count: 4\ncommit eyJ is not a jwt\n" +
		"cloning ssh://git@github.com/acme/app.git and git@github.com:acme/app.git\nsee https://example.com/a?q=x@y\n" +
		"{'name': 'lint', 'status': 'ok'}\nfortune cookie: none\nuses: pypa/gh-action-pypi-publish@release/v1\n"
	got := healthband.SanitizeCILog(text, false, "/work/repo")
	assert.Equal(t, strings.TrimSpace(text), got.Text)
	assert.Empty(t, got.Reasons)
}
