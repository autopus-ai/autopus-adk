package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const coAuthoredCommit = "feat(x): add y\n\nConstraint: z\n\n" +
	"🐙 Autopus <noreply@autopus.co>\n" +
	"Co-Authored-By: Claude <noreply@anthropic.com>\n"

// writeCommitMessage mirrors the commit-msg hook layout: the message lives in
// .git/ and the config is read from the repository root above it.
func writeCommitMessage(t *testing.T, lore string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	if lore != "" {
		require.NoError(t, os.WriteFile(filepath.Join(root, "autopus.yaml"), []byte(
			"mode: full\nproject_name: probe\nplatforms:\n  - claude-code\nlore:\n"+lore,
		), 0o600))
	}
	path := filepath.Join(root, ".git", "COMMIT_EDITMSG")
	require.NoError(t, os.WriteFile(path, []byte(coAuthoredCommit), 0o644))
	return path
}

func runCommitGates(t *testing.T, msgPath string) (checkErr, validateErr error) {
	t.Helper()
	check := newTestRootCmd()
	check.SetOut(&bytes.Buffer{})
	check.SetErr(&bytes.Buffer{})
	check.SetArgs([]string{"check", "--lore", "--quiet", "--message", msgPath})
	checkErr = check.Execute()

	validate := newTestRootCmd()
	validate.SetArgs([]string{"lore", "validate", msgPath})
	validateErr = validate.Execute()
	return checkErr, validateErr
}

func TestCommitGates_RejectCoAuthoredByByDefault(t *testing.T) {
	t.Parallel()

	for name, lore := range map[string]string{
		"no config":   "",
		"key omitted": "  enabled: true\n  required_trailers: [Constraint]\n",
	} {
		checkErr, validateErr := runCommitGates(t, writeCommitMessage(t, lore))
		assert.Error(t, checkErr, "check --lore must reject the trailer (%s)", name)
		assert.Error(t, validateErr, "lore validate must reject the trailer (%s)", name)
	}
}

func TestCommitGates_AllowCoAuthoredByWhenListIsEmpty(t *testing.T) {
	t.Parallel()

	msgPath := writeCommitMessage(t, "  enabled: true\n  required_trailers: [Constraint]\n  forbidden_trailers: []\n")
	checkErr, validateErr := runCommitGates(t, msgPath)
	assert.NoError(t, checkErr)
	assert.NoError(t, validateErr)
}
