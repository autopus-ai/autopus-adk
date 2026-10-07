package healthband_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/healthband"
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
		{"json AccessKey", `{"AccessKey": "LTAI5tQwErTyUiOpAsDfGhJk", "Region": "eu"}`, "LTAI5tQwErTyUiOpAsDfGhJk", `"Region": "eu"`},
		{"json client secret", `{"client_secret":"Zm9vYmFyYmF6cXV4MTIzNA"}`, "Zm9vYmFyYmF6cXV4MTIzNA", "{"},
		{"json token", `{"token": "tok_9f8e7d6c5b4a3f2e1d0c"}`, "tok_9f8e7d6c5b4a3f2e1d0c", "{"},
		{"url credentials", "pushing to https://deploy:s3cr3t-pass@registry.example.com/v2/app", "s3cr3t-pass", "registry.example.com/v2/app"},
		{"authorization token", "> Authorization: token 0123456789abcdef0123456789abcdef01234567", "0123456789abcdef0123456789abcdef01234567", "> "},
		{"authorization basic", "header Authorization: Basic ZGVwbG95OnMzY3IzdC1wYXNz sent", "ZGVwbG95OnMzY3IzdC1wYXNz", " sent"},
		{"authorization bearer", "authorization: bearer abcDEF123456.ghijklMNOP", "abcDEF123456.ghijklMNOP", ""},
		{"jwt", "cookie=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c ok",
			"SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", " ok"},
		{"pgp private key block", "-----BEGIN PGP PRIVATE KEY BLOCK-----\n\nlQOYBFsyntheticKeyMaterial0123\n=abcd\n-----END PGP PRIVATE KEY BLOCK-----\nafter",
			"lQOYBFsyntheticKeyMaterial0123", "after"},
		{"unterminated pgp block", "before\n-----BEGIN PGP PRIVATE KEY BLOCK-----\nlQOYBFsyntheticKeyMaterial4567", "lQOYBFsyntheticKeyMaterial4567", "before"},
		{"ssh2 private key block", "---- BEGIN SSH2 ENCRYPTED PRIVATE KEY ----\nP2/56wAAAi4AAAA3aWYtbW9kbntzaWdu\n---- END SSH2 ENCRYPTED PRIVATE KEY ----\nafter",
			"P2/56wAAAi4AAAA3aWYtbW9kbntzaWdu", "after"},
		{"openssh private key block", "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmU\n-----END OPENSSH PRIVATE KEY-----\nafter",
			"b3BlbnNzaC1rZXktdjEAAAAABG5vbmU", "after"},
		{"pem key type with a hyphen", "-----BEGIN RSA-PSS PRIVATE KEY-----\nMIIEvQIBADANBgkqSyntheticPSS\n-----END RSA-PSS PRIVATE KEY-----\nafter",
			"MIIEvQIBADANBgkqSyntheticPSS", "after"},
		{"azure AccountKey", "DefaultEndpointsProtocol=https;AccountName=acme;AccountKey=Zm9vYmFyYmF6cXV4Zm9vYmFyYmF6cXV4Zm9vYmFy==;EndpointSuffix=core.windows.net",
			"Zm9vYmFyYmF6cXV4Zm9vYmFyYmF6cXV4Zm9vYmFy==", "EndpointSuffix=core.windows.net"},
		{"gitlab token", "cloning with glpat-AbCdEfGhIjKlMnOpQrSt now", "glpat-AbCdEfGhIjKlMnOpQrSt", " now"},
		{"github oauth token", "using gho_0123456789abcdefghijABCDEFGHIJ0123 here", "gho_0123456789abcdefghijABCDEFGHIJ0123", " here"},
		{"github fine-grained token", "pat github_pat_11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz here", "github_pat_11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz", " here"},
		{"api key with sk prefix", "key sk-ant-api03-AbCdEfGhIjKlMnOp here", "sk-ant-api03-AbCdEfGhIjKlMnOp", " here"},
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
		"token count: 4\ncommit eyJ is not a jwt\n"
	got := healthband.SanitizeCILog(text, false, "/work/repo")
	assert.Equal(t, strings.TrimSpace(text), got.Text)
	assert.Empty(t, got.Reasons)
}
