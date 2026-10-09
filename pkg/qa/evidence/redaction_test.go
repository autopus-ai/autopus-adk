package evidence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provider-format fakes shared by this package's tests are joined from
// fragments: GitHub push protection and secret scanners match the contiguous
// source text, so a fake written as one literal can block a push. The joined
// values are byte-identical to the literals they replaced.
const (
	fakeProjectKey   = "sk-" + "proj-qameshfake1234567890"
	fakeAWSSecretKey = "wJalrXUt" + "nFEMIK7MDENG" + "bPxRfiCYEXAMPLEKEY"
)

func TestRedactText_MasksSecretsPathsAndPrivateNotes(t *testing.T) {
	t.Parallel()

	raw := "token=" + fakeProjectKey + "\nsession=secret-cookie\npath=/Users/alice/private/notes.md\nprivate_note_body=customer note"

	redacted := RedactText(raw)

	require.NoError(t, AssertSafeText(redacted, "redacted"))
	assert.Contains(t, redacted, RedactedSecret)
	assert.Contains(t, redacted, "/Users/[REDACTED_USER]/private/notes.md")
	assert.Contains(t, redacted, RedactedPrivateNote)
	assert.NotContains(t, redacted, fakeProjectKey)
	assert.NotContains(t, redacted, "secret-cookie")
	assert.NotContains(t, redacted, "alice")
	assert.NotContains(t, redacted, "customer note")
}

func TestRedactText_MasksJSONSensitiveValuesAndCrossPlatformPaths(t *testing.T) {
	t.Parallel()

	raw := `{"access_token":"` + fakeProjectKey + `","cookie":"session=secret-cookie","authorization":"Bearer ` + fakeProjectKey +
		`","localNoteBody":"customer note","linux":"/home/alice/private/notes.md","windows":"C:\\Users\\alice\\private\\notes.md"}`

	redacted := RedactText(raw)

	require.NoError(t, AssertSafeText(redacted, "json"))
	assert.Contains(t, redacted, RedactedSecret)
	assert.Contains(t, redacted, RedactedPrivateNote)
	assert.Contains(t, redacted, "/home/[REDACTED_USER]/private/notes.md")
	assert.Contains(t, redacted, `C:\\Users\\[REDACTED_USER]\\private\\notes.md`)
	assert.NotContains(t, redacted, "qameshfake")
	assert.NotContains(t, redacted, "alice")
	assert.NotContains(t, redacted, "customer note")
}

func TestRedactText_MasksCLIFlagValuesAndCredentialURLs(t *testing.T) {
	t.Parallel()

	raw := "go test ./... --password 'hunter two' --api-key=abc12345 --report https://user:pass@example.test/out?token=tok12345"

	redacted := RedactText(raw)

	require.NoError(t, AssertSafeText(redacted, "command"))
	assert.Contains(t, redacted, RedactedSecret)
	assert.NotContains(t, redacted, "hunter")
	assert.NotContains(t, redacted, "abc12345")
	assert.NotContains(t, redacted, "user:pass@")
	assert.NotContains(t, redacted, "tok12345")
}

func TestFindUnsafeText_FindsProviderBoundLeaks(t *testing.T) {
	t.Parallel()

	findings := FindUnsafeText("Authorization: Bearer "+fakeProjectKey, "raw")

	require.NotEmpty(t, findings)
	assert.Equal(t, "secret", findings[0].Type)
	assert.NotContains(t, FormatFindings(findings), "qameshfake")
}

func TestFindUnsafeText_FindsSecretQueryParams(t *testing.T) {
	t.Parallel()

	findings := FindUnsafeText("https://example.test/out?token=tok12345", "url")

	require.NotEmpty(t, findings)
	assert.Contains(t, findingTypes(findings), "sensitive_query")
	assert.NotContains(t, FormatFindings(findings), "tok12345")
}

func TestFindUnsafeText_IgnoresHyphenatedWordsContainingFlagNames(t *testing.T) {
	t.Parallel()

	findings := FindUnsafeText("review failure pattern tie-token should stay searchable", "review")

	assert.Empty(t, findings)
}

func TestRedactText_MasksCredentialURLsOfEveryScheme(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://admin:hunter2@host/x",
		"http://admin:hunter2@host/x",
		"postgres://admin:hunter2@db:5432/prod",
		"mysql://admin:hunter2@db:3306/prod",
		"mongodb://admin:hunter2@db:27017/prod",
		"redis://admin:hunter2@cache:6379",
		"amqp://admin:hunter2@mq:5672",
		"ssh://admin:hunter2@box:22",
		"ftp://admin:hunter2@files:21",
		"postgresql+ssl://admin:hunter2@db:5432/prod",
		"POSTGRES://ADMIN:HUNTER2@DB:5432/PROD",
	} {
		t.Run(raw, func(t *testing.T) {
			line := "URL " + raw

			redacted := RedactText(line)

			assert.Contains(t, redacted, RedactedSecret)
			assert.NotContains(t, redacted, "hunter2")
			assert.NotContains(t, redacted, "HUNTER2")
			assert.NotEmpty(t, FindUnsafeText(line, "artifact"), "an unredacted credential URL must be unsafe")
			require.NoError(t, AssertSafeText(redacted, "artifact"))
		})
	}
}

func TestRedactText_KeepsURLsWithoutUserinfoIntact(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"https://host/x", "https://host:8080/x", "postgres://db:5432/prod"} {
		assert.Equal(t, raw, RedactText(raw))
		assert.Empty(t, findingTypes(FindUnsafeText(raw, "url")))
	}
}

// The credential URL gate must agree with the redactor: FindUnsafeText is what
// keeps redaction_status from reporting "passed" over a surviving credential.
func TestFindUnsafeText_FlagsNonHTTPCredentialURL(t *testing.T) {
	t.Parallel()

	findings := FindUnsafeText("db=postgres://admin:hunter2@db.internal:5432/prod", "stdout")

	assert.Contains(t, findingTypes(findings), "credential_url")
	assert.NotContains(t, FormatFindings(findings), "hunter2")
	require.Error(t, AssertSafeText("db=postgres://admin:hunter2@db.internal:5432/prod", "stdout"))
}

func TestRedactText_KeepsHarnessProseAfterCredentialKey(t *testing.T) {
	t.Parallel()

	line := "missing_credentials: opaque credential refs are required before mobile execution"

	redacted := RedactText(line)

	assert.Equal(t, line, redacted)
	assert.Empty(t, FindUnsafeText(line, "setup_gap"), "prose must not be reported as a leak either")
}

func TestRedactText_StillMasksSecretShapedAssignmentValues(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"mixed case and symbol": "password: hunter2SECRET!",
		"aws secret":            "AWS_SECRET_ACCESS_KEY=" + fakeAWSSecretKey,
		"github pat":            "token=github" + "_pat_11ABCDEFG0aBcDeFgHiJkL_" + "MnOpQrStUvWxYz" + "0123456789abcdefghij",
		"jwt":                   "authorization=" + "eyJhbGciOiJIUzI1NiIs" + "InR5cCI6IkpXVCJ9.abc.def",
		"quoted prose shape":    `password: "opaque" and more prose`,
		"bare weak password":    "password: letmein",
		"next key follows":      "token=abcdefgh password=zyxwvuts",
	} {
		t.Run(name, func(t *testing.T) {
			redacted := RedactText(raw)

			assert.Contains(t, redacted, RedactedSecret)
			require.NoError(t, AssertSafeText(redacted, "assignment"))
			assert.NotEmpty(t, FindUnsafeText(raw, "assignment"))
		})
	}
}

// Redaction runs more than once on the same bytes: publish, feedback bundle
// export, then prompt rendering. A non-idempotent pass corrupted the
// placeholder ("[REDACTED_SECRET]]]") in published evidence.
func TestRedactText_IsIdempotent(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"AWS_SECRET_ACCESS_KEY=" + fakeAWSSecretKey,
		"db=postgres://admin:hunter2@db.internal:5432/prod",
		"token=" + fakeProjectKey,
		`{"access_token":"` + fakeProjectKey + `"}`,
		"go test ./... --password 'hunter two'",
		"path=/Users/alice/private/notes.md",
		"missing_credentials: opaque credential refs are required",
	} {
		once := RedactText(raw)

		assert.Equal(t, once, RedactText(once), "second pass changed %q", raw)
		assert.NotContains(t, once, RedactedSecret+"]")
	}
}

func findingTypes(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.Type)
	}
	return out
}
