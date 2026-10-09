package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Fake credentials are joined from fragments: GitHub push protection and
// secret scanners match the contiguous source text, so a provider-format fake
// written as one literal would be reported. Each joined value is
// byte-identical to the literal it replaced, so the scanner sees the same
// input.
const (
	fakeOpenAIKey     = "sk-" + "abcdefghijklmnopqr" + "stuvwxyz1234567890"
	fakeAWSKeyID      = "AKIA" + "IOSFODNN7EXAMPLE"
	fakeGitHubPAT     = "ghp_" + "ABCDEFGHIJKLMNOPQR" + "STUVWXYZabcdefghij"
	fakeGitHubOAuth   = "gho_" + "ABCDEFGHIJKLMNOPQR" + "STUVWXYZabcdefghij"
	fakeAWSSecretPair = "aws_secret=" + "ABCDEFGHIJKLMNOPQRST" + "UVWXYZ0123456789abcd"
)

func TestSecretScanner_DefaultPatterns(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()

	tests := []struct {
		name     string
		input    string
		contains bool
	}{
		{"OpenAI key", "key: " + fakeOpenAIKey, true},
		{"AWS access key", fakeAWSKeyID, true},
		{"GitHub PAT", fakeGitHubPAT, true},
		{"GitHub OAuth", fakeGitHubOAuth, true},
		{"Bearer token", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.test", true},
		{"password assignment", "password=supersecret123", true},
		{"secret assignment", "secret: my-secret-value", true},
		{"api_key assignment", "api_key=" + "abcdef12345", true},
		{"AWS secret key", fakeAWSSecretPair, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.contains, s.ContainsSecret(tt.input),
				"ContainsSecret mismatch for %q", tt.input)

			scanned := s.Scan(tt.input)
			if tt.contains {
				assert.Contains(t, scanned, redactedPlaceholder,
					"Scan should redact %q", tt.input)
			}
		})
	}
}

func TestSecretScanner_NoFalsePositives(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()

	safe := []string{
		"Hello, this is a normal log message.",
		"The quick brown fox jumps over the lazy dog.",
		"func main() { fmt.Println(42) }",
		"version: 1.2.3",
		"path: /usr/local/bin/app",
		"user_id=12345",
		"status_code=200",
		"sk-short",
		"AKIA",
		"ghp_short",
	}

	for _, input := range safe {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			assert.False(t, s.ContainsSecret(input),
				"should NOT detect secret in %q", input)
			assert.Equal(t, input, s.Scan(input),
				"Scan should not modify safe input %q", input)
		})
	}
}

func TestSecretScanner_CustomPatterns(t *testing.T) {
	t.Parallel()

	s := NewSecretScannerWithPatterns([]string{
		`CUSTOM-[A-Z]{10}`,
	})

	// Custom pattern should match.
	assert.True(t, s.ContainsSecret("key: CUSTOM-ABCDEFGHIJ"))
	assert.Contains(t, s.Scan("key: CUSTOM-ABCDEFGHIJ"), redactedPlaceholder)

	// Default patterns should still work.
	assert.True(t, s.ContainsSecret(fakeOpenAIKey))
}

func TestSecretScanner_CustomPatterns_InvalidSkipped(t *testing.T) {
	t.Parallel()

	// Invalid regex should not cause panic.
	s := NewSecretScannerWithPatterns([]string{
		`[invalid`,
		`VALID-[A-Z]+`,
	})

	// The valid custom pattern should work.
	assert.True(t, s.ContainsSecret("VALID-ABC"))
	// Default patterns should still work.
	assert.True(t, s.ContainsSecret(fakeOpenAIKey))
}

func TestSecretScanner_ContainsSecret(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()
	assert.True(t, s.ContainsSecret(fakeGitHubPAT))
	assert.False(t, s.ContainsSecret("this is just normal text"))
}

func TestSecretScanner_ScanLines(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()
	input := "line1: safe\nline2: " + fakeOpenAIKey + "\nline3: safe"
	result := s.ScanLines(input)

	assert.Contains(t, result, "line1: safe")
	assert.Contains(t, result, "line3: safe")
	assert.NotContains(t, result, fakeOpenAIKey)
	assert.Contains(t, result, redactedPlaceholder)
}

func TestSecretScanner_ScanPreservesNonSecretContent(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()
	input := "Build completed successfully in 3.2s"
	assert.Equal(t, input, s.Scan(input))
}
