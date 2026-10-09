// Package security tests for expanded secret scanner patterns (REQ-11).
package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecretScanner_GCPServiceAccountKey verifies detection of GCP service
// account JSON key format.
// RED: GCP service account pattern not in defaultPatterns yet.
func TestSecretScanner_GCPServiceAccountKey(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()

	// Given: a string containing a GCP service account key marker
	// ("private_key_id" is a distinctive field in GCP SA JSON)
	gcpInput := `"private_key_id": "abcdef1234567890abcdef1234567890abcdef12"`

	// When: we scan for secrets
	// Then: GCP service account key must be detected
	assert.True(t, s.ContainsSecret(gcpInput),
		"GCP service account key must be detected")
	assert.Contains(t, s.Scan(gcpInput), redactedPlaceholder,
		"GCP service account key must be redacted")
}

// TestSecretScanner_AzureClientSecret verifies detection of Azure client secret
// format.
// RED: Azure client secret pattern not in defaultPatterns yet.
func TestSecretScanner_AzureClientSecret(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()

	// Given: a string containing an Azure client secret
	// Azure client secrets are typically 32-34 char random strings
	azureInput := `AZURE_CLIENT_SECRET=Th1s1sAnAzureS3cret~ABCDEFGHIJ`

	// When: we scan for secrets
	// Then: Azure client secret must be detected
	assert.True(t, s.ContainsSecret(azureInput),
		"Azure client secret must be detected")
	assert.Contains(t, s.Scan(azureInput), redactedPlaceholder,
		"Azure client secret must be redacted")
}

// privateKeyHeader is the PEM header of a private key of kind, joined from
// fragments so no PEM armor is committed as one literal (secret scanners and
// GitHub push protection match the source text); the joined header is
// byte-identical to the literal it replaced.
func privateKeyHeader(kind string) string {
	return "-----BEGIN " + kind + " PRIVATE" + " KEY-----"
}

// TestSecretScanner_SSHPrivateKeyHeader verifies detection of SSH private key
// PEM header.
// RED: SSH private key header pattern not in defaultPatterns yet.
func TestSecretScanner_SSHPrivateKeyHeader(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()

	tests := []struct {
		name  string
		input string
	}{
		{"RSA private key", privateKeyHeader("RSA")},
		{"OpenSSH private key", privateKeyHeader("OPENSSH")},
		{"EC private key", privateKeyHeader("EC")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// When: we scan for secrets
			// Then: SSH private key header must be detected and redacted
			assert.True(t, s.ContainsSecret(tt.input),
				"SSH private key header must be detected: %s", tt.input)
			assert.Contains(t, s.Scan(tt.input), redactedPlaceholder,
				"SSH private key header must be redacted: %s", tt.input)
		})
	}
}

// TestSecretScanner_AutopusJWT verifies detection of Autopus JWT token format.
// RED: Autopus JWT pattern not in defaultPatterns yet.
func TestSecretScanner_AutopusJWT(t *testing.T) {
	t.Parallel()

	s := NewSecretScanner()

	// Given: a string containing an Autopus JWT
	// Autopus JWT prefix: "apjwt_" followed by base64url content
	autopusJWT := `apjwt_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ3b3JrZXItMTIzIn0.signature`

	// When: we scan for secrets
	// Then: Autopus JWT must be detected and redacted
	assert.True(t, s.ContainsSecret(autopusJWT),
		"Autopus JWT must be detected")
	assert.Contains(t, s.Scan(autopusJWT), redactedPlaceholder,
		"Autopus JWT must be redacted")
}
