// Package security provides secret scanning and redaction for worker output.
package security

import (
	"regexp"
	"strings"
)

const redactedPlaceholder = "***REDACTED***"

// SecretScanner detects and redacts secrets in text output.
type SecretScanner struct {
	patterns []*regexp.Regexp
}

// DefaultPatternSources returns the source text of the built-in secret
// patterns in the order Scan applies them. Each call returns a fresh slice.
// pkg/secretscan keeps a verbatim copy and its tests compare it with this list.
func DefaultPatternSources() []string {
	return []string{
		// OpenAI / Stripe style API keys
		`sk-[a-zA-Z0-9]{20,}`,
		// AWS access key IDs
		`AKIA[A-Z0-9]{16}`,
		// GitHub personal access tokens
		`ghp_[a-zA-Z0-9]{36}`,
		// GitHub OAuth tokens
		`gho_[a-zA-Z0-9]{36}`,
		// Bearer tokens
		`Bearer [a-zA-Z0-9._\-]+`,
		// Generic secret assignments (password=, secret=, api_key=, etc.)
		`(?i)(password|secret|api_key|apikey|token)\s*[=:]\s*\S+`,
		// AWS secret keys (40-char base64 near "aws" or "secret")
		`(?i)(aws|secret).{0,20}[a-zA-Z0-9/+=]{40}`,
		// GCP service account JSON key
		`"private_key[_a-z]*"\s*:\s*"[^"]+`,
		// Azure client secrets
		`(?i)azure.{0,20}(client.?secret|tenant.?id)\s*[=:]\s*\S+`,
		// SSH private key headers
		`-----BEGIN[A-Z ]*PRIVATE KEY-----`,
		// Autopus JWT (apjwt_ prefix with base64url segments)
		`apjwt_[a-zA-Z0-9_\-]+\.[a-zA-Z0-9_\-]+\.[a-zA-Z0-9_\-]+`,
	}
}

// defaultPatterns returns the built-in secret detection patterns.
func defaultPatterns() []*regexp.Regexp {
	raw := DefaultPatternSources()
	patterns := make([]*regexp.Regexp, 0, len(raw))
	for _, r := range raw {
		patterns = append(patterns, regexp.MustCompile(r))
	}
	return patterns
}

// NewSecretScanner creates a scanner with default secret patterns.
func NewSecretScanner() *SecretScanner {
	return &SecretScanner{patterns: defaultPatterns()}
}

// NewSecretScannerWithPatterns creates a scanner with default patterns
// plus additional user-supplied regex patterns. Invalid patterns are skipped.
func NewSecretScannerWithPatterns(additional []string) *SecretScanner {
	s := &SecretScanner{patterns: defaultPatterns()}
	for _, p := range additional {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		s.patterns = append(s.patterns, re)
	}
	return s
}

// Scan returns the input with all detected secrets replaced by ***REDACTED***.
func (s *SecretScanner) Scan(input string) string {
	result := input
	for _, re := range s.patterns {
		result = re.ReplaceAllString(result, redactedPlaceholder)
	}
	return result
}

// ContainsSecret reports whether the input contains any detectable secret.
func (s *SecretScanner) ContainsSecret(input string) bool {
	for _, re := range s.patterns {
		if re.MatchString(input) {
			return true
		}
	}
	return false
}

// ScanLines scans each line independently and returns the redacted output.
func (s *SecretScanner) ScanLines(input string) string {
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		lines[i] = s.Scan(line)
	}
	return strings.Join(lines, "\n")
}
