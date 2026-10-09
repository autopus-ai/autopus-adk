package issue_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/issue"
)

func TestFormatMarkdown_ContainsExpectedSections(t *testing.T) {
	t.Parallel()

	report := issue.IssueReport{
		Title: "Test error report",
		Context: issue.IssueContext{
			ErrorMessage: "command not found: auto",
			Command:      "auto plan",
			ExitCode:     127,
			OS:           "linux/amd64",
			GoVersion:    "go1.23.0",
			AutoVersion:  "0.5.0",
			Platform:     "claude-code",
		},
		Hash:   "abc123def456",
		Labels: []string{"auto-report"},
		Repo:   "insajin/autopus-adk",
	}

	body, err := issue.FormatMarkdown(report)
	require.NoError(t, err)

	assert.Contains(t, body, "auto plan")
	assert.Contains(t, body, "127")
	assert.Contains(t, body, "command not found: auto")
	assert.Contains(t, body, "linux/amd64")
	assert.Contains(t, body, "go1.23.0")
	assert.Contains(t, body, "0.5.0")
	assert.Contains(t, body, "claude-code")
	assert.Contains(t, body, "abc123def456")
}

func TestFormatMarkdown_WithTelemetry(t *testing.T) {
	t.Parallel()

	report := issue.IssueReport{
		Title: "Test report with telemetry",
		Context: issue.IssueContext{
			ErrorMessage: "pipeline failed",
			Command:      "auto run",
			Telemetry:    `{"spec_id":"SPEC-001","status":"FAIL"}`,
		},
		Hash: "deadbeef",
	}

	body, err := issue.FormatMarkdown(report)
	require.NoError(t, err)

	assert.Contains(t, body, "Recent Telemetry")
	assert.Contains(t, body, "SPEC-001")
}

func TestFormatMarkdown_WithoutTelemetry(t *testing.T) {
	t.Parallel()

	report := issue.IssueReport{
		Title: "Report without telemetry",
		Context: issue.IssueContext{
			ErrorMessage: "some error",
			Command:      "auto init",
		},
		Hash: "cafebabe",
	}

	body, err := issue.FormatMarkdown(report)
	require.NoError(t, err)

	assert.NotContains(t, body, "Recent Telemetry")
}

// TestFormatMarkdown_SanitizesSecrets reproduces the leak where Command and
// ErrorMessage are rendered into the issue body without sanitization,
// exposing API keys, credential URLs, and home paths to a public repo.
func TestFormatMarkdown_SanitizesSecrets(t *testing.T) {
	t.Parallel()

	const (
		apiKey  = "sk-" + "live-ABCDEF0123456789" // joined so no provider-format literal is committed
		credURL = "https://user:supersecrettoken@host/x"
	)

	// Use the real home directory so this exercises the actual SanitizePath
	// behavior (it redacts os.UserHomeDir(), not an arbitrary literal path).
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	home := filepath.Join(homeDir, "private")

	report := issue.IssueReport{
		Title: "Test error report",
		Context: issue.IssueContext{
			ErrorMessage: "failed with token " + apiKey + " at " + home,
			Command:      "curl " + credURL,
			ExitCode:     1,
		},
		Hash: "secrettest",
	}

	body, err := issue.FormatMarkdown(report)
	require.NoError(t, err)

	assert.NotContains(t, body, apiKey, "API key must not appear in the rendered issue body")
	assert.NotContains(t, body, "supersecrettoken", "credential URL secret must not appear in the rendered issue body")
	assert.NotContains(t, body, home, "home directory path must not appear in the rendered issue body")
}

func TestFormatMarkdown_TruncatesLargeBody(t *testing.T) {
	t.Parallel()

	// Build an error message that will push the rendered body over 65536 chars.
	longMsg := strings.Repeat("x", 70000)
	report := issue.IssueReport{
		Title: "Large report",
		Context: issue.IssueContext{
			ErrorMessage: longMsg,
			Command:      "auto plan",
		},
		Hash: "truncated",
	}

	body, err := issue.FormatMarkdown(report)
	require.NoError(t, err)

	assert.LessOrEqual(t, len(body), 65536+len("\n... [truncated]"))
	assert.Contains(t, body, "[truncated]")
}
