package issue_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/issue"
)

func TestSanitizePath(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir:", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "replaces home dir prefix",
			input: home + "/Documents/project/autopus.yaml",
			want:  "$HOME/Documents/project/autopus.yaml",
		},
		{
			name:  "no home dir in string",
			input: "/etc/hosts",
			want:  "/etc/hosts",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "home dir alone",
			input: home,
			want:  "$HOME",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := issue.SanitizePath(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Fake credentials in this file are joined from fragments: secret scanners and
// GitHub push protection match the contiguous source text, so a
// provider-format fake written as one literal would be reported. Each joined
// value is byte-identical to the literal it replaced.
func TestSanitizeSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "GitHub PAT token",
			input: "token: ghp_" + "abc123XYZdefGHI456jkl",
			want:  "token: [REDACTED]",
		},
		{
			name:  "OpenAI sk- key",
			input: "key: sk-" + "proj-abcdefghij1234567890",
			want:  "key: [REDACTED]",
		},
		{
			name:  "GitHub oauth token",
			input: "auth: gho_" + "someOAuthToken123",
			want:  "auth: [REDACTED]",
		},
		{
			name:  "AWS AKIA key",
			input: "aws_key=AKIA" + "IOSFODNN7EXAMPLE",
			want:  "aws_key=[REDACTED]",
		},
		{
			name:  "env var TOKEN",
			input: "GITHUB_TOKEN=abc123secret",
			want:  "GITHUB_TOKEN=[REDACTED]",
		},
		{
			name:  "env var SECRET",
			input: "MY_SECRET=topsecretvalue",
			want:  "MY_SECRET=[REDACTED]",
		},
		{
			name:  "env var KEY",
			input: "API_KEY=someapikey",
			want:  "API_KEY=[REDACTED]",
		},
		{
			name:  "no secrets",
			input: "just normal text without secrets",
			want:  "just normal text without secrets",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := issue.SanitizeSecrets(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSanitizeGitURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips username and password",
			input: "https://user:pass@github.com/org/repo.git",
			want:  "https://github.com/org/repo.git",
		},
		{
			name:  "strips token in URL",
			input: "https://ghp_" + "token123@github.com/org/repo.git",
			want:  "https://github.com/org/repo.git",
		},
		{
			name:  "no credentials in URL",
			input: "https://github.com/org/repo.git",
			want:  "https://github.com/org/repo.git",
		},
		{
			name:  "ssh URL unchanged",
			input: "git@github.com:org/repo.git",
			want:  "git@github.com:org/repo.git",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := issue.SanitizeGitURL(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSanitize(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir:", err)
	}

	input := home + "/project GITHUB_TOKEN=secret https://user:pass@github.com/org/repo.git"
	got := issue.Sanitize(input)

	assert.Contains(t, got, "$HOME/project")
	assert.Contains(t, got, "GITHUB_TOKEN=[REDACTED]")
	assert.NotContains(t, got, "user:pass@")
	assert.NotContains(t, got, "secret")
}
