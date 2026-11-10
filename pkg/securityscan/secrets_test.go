package securityscan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mixed returns n characters of a fixed alphanumeric cycle. Secret-shaped
// test values are assembled from it at run time, so no file in this
// repository holds a literal that a secret scanner would flag.
func mixed(n int) string {
	const cycle = "aZ3kP9qR2mX7vB4nL8wC5tY1hJ6gD0fSeU"
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(cycle[i%len(cycle)])
	}
	return b.String()
}

func upper(n int) string { return strings.ToUpper(strings.Map(dropDigitsOutsideAWS, mixed(n))) }

// dropDigitsOutsideAWS keeps only the base32 alphabet AWS key ids use.
func dropDigitsOutsideAWS(r rune) rune {
	if r == '0' || r == '1' || r == '8' || r == '9' {
		return 'Q'
	}
	return r
}

func TestMatchSecrets_TruePositives(t *testing.T) {
	jwt := "ey" + "J" + mixed(20) + ".ey" + "J" + mixed(24) + "." + mixed(30)
	for name, row := range map[string]struct {
		line string
		rule string
	}{
		"rsa key header":   {"-----BEGIN RSA " + "PRIVATE KEY-----", "private-key"},
		"plain key header": {`key := "-----BEGIN ` + `PRIVATE KEY-----\nMIIE"`, "private-key"},
		"openssh header":   {"-----BEGIN OPENSSH " + "PRIVATE KEY-----", "private-key"},
		"aws key id":       {`aws_access_key_id = "AK` + "IA" + upper(16) + `"`, "aws-access-token"},
		"github pat":       {"GITHUB_TOKEN=gh" + "p_" + mixed(36), "github-pat"},
		"github oauth":     {"token: gh" + "o_" + mixed(36), "github-oauth"},
		"github app":       {"gh" + "s_" + mixed(36), "github-app-token"},
		"github fine":      {"github" + "_pat_" + mixed(82), "github-fine-grained-pat"},
		"slack bot":        {"SLACK=xo" + "xb-" + "123456789012-" + "123456789012-" + mixed(24), "slack-token"},
		"stripe live":      {"stripe.Key = \"sk" + "_live_" + mixed(24) + "\"", "stripe-access-token"},
		"google api key":   {"apiKey: 'AI" + "za" + mixed(35) + "'", "gcp-api-key"},
		"jwt assignment":   {"const authToken = \"" + jwt + "\"", "jwt"},
		"bearer header":    {"Authorization: Bearer " + jwt, "jwt"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, []string{row.rule}, MatchSecrets(row.line))
		})
	}
}

func TestMatchSecrets_NearMissesDoNotMatch(t *testing.T) {
	jwt := "ey" + "J" + mixed(20) + ".ey" + "J" + mixed(24) + "." + mixed(30)
	for name, line := range map[string]string{
		"public key":         "-----BEGIN PUBLIC " + "KEY-----",
		"certificate":        "-----BEGIN " + "CERTIFICATE-----",
		"aws docs example":   "AK" + "IAIOSFODNN7" + "EXAMPLE",
		"aws too short":      "AK" + "IA" + upper(12),
		"github short":       "gh" + "p_" + mixed(30),
		"github placeholder": "gh" + "p_" + strings.Repeat("x", 36),
		"github in a word":   "nogh" + "p_" + mixed(36),
		"slack placeholder":  "xo" + "xb-your-bot-token-here",
		"stripe test key":    "sk" + "_test_" + mixed(24),
		"google key short":   "AI" + "za" + mixed(20),
		"bare jwt":           "fixtures: " + jwt,
		"regex source":       "regexp.MustCompile(`gh" + "p_[0-9a-zA-Z]{36}`)",
		"prose":              "Rotate the private key and the GitHub token after the incident.",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, MatchSecrets(line))
		})
	}
}

func TestMatchSecrets_AllowMarker(t *testing.T) {
	token := "gh" + "p_" + mixed(36)
	assert.NotEmpty(t, MatchSecrets("fixture := \""+token+"\""))
	assert.Empty(t, MatchSecrets("fixture := \""+token+"\" // autopus:allow-secret"))
	assert.Empty(t, MatchSecrets("fixture := \""+token+"\" # gitleaks:allow"))
}
