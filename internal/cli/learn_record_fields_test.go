package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLearnRecord_S2_VerbatimFlags_RefusedWithoutWrite pins review S2:
// --severity must name a severity, and --files and --packages are stored as
// given, so a value redaction would change is refused before any write and
// the error never echoes it.
func TestLearnRecord_S2_VerbatimFlags_RefusedWithoutWrite(t *testing.T) {
	ghp := "ghp_" + strings.Repeat("BCDF2468", 4) + "BCDF"
	tests := []struct {
		name, flag, value, field, detail, secret string
	}{
		{"token in files", "--files", "pkg/a.go," + ghp, "files", "needs_redaction", ghp},
		{"password in files", "--files", "password=hunter2xyz", "files", "needs_redaction", "hunter2xyz"},
		{"token in packages", "--packages", synthGitHubToken(), "packages", "needs_redaction", synthGitHubToken()},
		{"unknown severity", "--severity", "urgent", "severity", "unknown_value", "urgent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupLearnDir(t)
			chdir(t, dir)
			path := writeLegacyStore(t, dir)
			before := fileSHA256(t, path)

			out, err := runLearn(t, "learn", "record", "--type", "fix_pattern", "--pattern", "p", tt.flag, tt.value)

			require.Error(t, err)
			assert.Equal(t, "learning_field_invalid: "+tt.field+": "+tt.detail, err.Error())
			assert.NotContains(t, out, tt.secret)
			assert.Equal(t, before, fileSHA256(t, path), "store bytes must not change")
		})
	}
}

// TestLearnRecord_S5_EchoEscapesControlCharacters pins review S5: the
// Recorded line prints the stored (redacted) pattern with its control
// characters escaped, so a pattern cannot drive the terminal.
func TestLearnRecord_S5_EchoEscapesControlCharacters(t *testing.T) {
	dir := setupLearnDir(t)
	chdir(t, dir)

	out, err := runLearn(t, "learn", "record", "--type", "fix_pattern",
		"--pattern", "hook \x1b[31mred\nnext "+synthGitHubToken())

	require.NoError(t, err)
	assert.Equal(t, `Recorded fix_pattern entry: hook \u001b[31mred\u000anext [REDACTED_SECRET]`+"\n", out)
}
