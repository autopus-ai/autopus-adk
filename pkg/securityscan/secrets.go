package securityscan

import (
	"regexp"
	"strings"
)

// AllowSecretMarkers on a line tell the built-in matcher that the value is
// intentional, such as a test fixture. gitleaks:allow is honored too, so one
// marker satisfies both checks.
var AllowSecretMarkers = []string{"autopus:allow-secret", "gitleaks:allow"}

// secretRule is one high-precision pattern. The first capture group, when
// present, is the token body that must look random to count.
type secretRule struct {
	id string
	re *regexp.Regexp
}

// secretRules favor precision over recall: each needs a vendor prefix or a
// PEM header, so a hit is almost always a real credential. Rule ids follow
// gitleaks so a .gitleaksignore entry reads the same for both.
var secretRules = []secretRule{
	{"private-key", regexp.MustCompile(`-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----`)},
	{"aws-access-token", regexp.MustCompile(`\b(?:A3T[A-Z0-9]|AKIA|ASIA|ABIA|ACCA)([A-Z2-7]{16})\b`)},
	{"github-pat", regexp.MustCompile(`\bghp_([0-9a-zA-Z]{36})\b`)},
	{"github-oauth", regexp.MustCompile(`\bgho_([0-9a-zA-Z]{36})\b`)},
	{"github-app-token", regexp.MustCompile(`\b(?:ghu|ghs)_([0-9a-zA-Z]{36})\b`)},
	{"github-refresh-token", regexp.MustCompile(`\bghr_([0-9a-zA-Z]{36})\b`)},
	{"github-fine-grained-pat", regexp.MustCompile(`\bgithub_pat_([0-9a-zA-Z_]{82})\b`)},
	{"slack-token", regexp.MustCompile(`\bxox[abposr]-([0-9]{8,}-[0-9A-Za-z-]{8,})`)},
	{"slack-webhook-url", regexp.MustCompile(`hooks\.slack\.com/services/(T[A-Z0-9]{8,}/B[A-Z0-9]{8,}/[A-Za-z0-9]{20,})`)},
	{"stripe-access-token", regexp.MustCompile(`\b(?:sk|rk)_live_([0-9a-zA-Z]{20,})\b`)},
	{"gcp-api-key", regexp.MustCompile(`\bAIza([0-9A-Za-z_-]{35})(?:[^0-9A-Za-z_-]|$)`)},
	// A JWT alone is often a harmless fixture; it counts only when it is
	// assigned to a credential-named key or sent as a bearer token.
	{"jwt", regexp.MustCompile(`(?i)(?:(?:token|jwt|secret|session|auth)[\w.-]*["']?\s*[:=]\s*["'` + "`" +
		`]?|bearer\s+)(eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{20,})`)},
}

// MatchSecrets returns the rule ids whose pattern a line matches, in rule
// order, or nil. A line carrying an allow marker never matches.
func MatchSecrets(line string) []string {
	for _, marker := range AllowSecretMarkers {
		if strings.Contains(line, marker) {
			return nil
		}
	}
	var ids []string
	for _, rule := range secretRules {
		for _, m := range rule.re.FindAllStringSubmatch(line, -1) {
			if len(m) < 2 || plausibleToken(m[1]) {
				ids = append(ids, rule.id)
				break
			}
		}
	}
	return ids
}

// plausibleToken rejects placeholders: documentation examples and bodies
// with too few distinct characters to be a generated credential.
func plausibleToken(body string) bool {
	if strings.Contains(strings.ToUpper(body), "EXAMPLE") {
		return false
	}
	seen := map[rune]bool{}
	for _, r := range body {
		seen[r] = true
	}
	return len(seen) >= 8
}
