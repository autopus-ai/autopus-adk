package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasValidLoreType_AllKnownTypes(t *testing.T) {
	t.Parallel()

	validPrefixes := []string{
		"feat(cli): add something",
		"fix(config): correct typo",
		"refactor(pkg): clean up logic",
		"test(api): add unit tests",
		"docs(readme): update guide",
		"chore(deps): bump version",
		"perf(cache): reduce allocations",
		"merge(release): integrate release branch",
		"revert(api): restore compatibility",
	}

	for _, msg := range validPrefixes {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			assert.True(t, hasValidLoreType(msg), "expected valid Lore type for: %q", msg)
		})
	}
}

func TestHasValidLoreType_InvalidMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  string
	}{
		{"empty string", ""},
		{"plain english", "Update README"},
		{"lowercase type without parens", "feat: missing parens"},
		{"wrong type", "update(cli): something"},
		{"typo in type", "fixt(api): typo"},
		{"mixed case", "Feat(scope): capitalized"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, hasValidLoreType(tt.msg), "expected invalid Lore type for: %q", tt.msg)
		})
	}
}

func TestCheckLore_ValidLoreCommit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initTestGitRepo(t, dir)
	writeTestFile(t, dir, "dummy.go", "package dummy\n")

	runGitCommand(t, dir, "add", "dummy.go")
	runGitCommand(
		t,
		dir,
		"commit",
		"-m",
		"feat(cli): add feature\n\nConstraint: keep cli contract stable\n\n🐙 Autopus <noreply@autopus.co>",
	)

	var buf bytes.Buffer
	result := checkLore(dir, &buf, false)
	assert.True(t, result, "checkLore must return true for a valid Lore commit")
	assert.Contains(t, buf.String(), "OK", "output must indicate success")
}

func TestCheckLore_InvalidLoreCommit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initTestGitRepo(t, dir)
	writeTestFile(t, dir, "dummy.go", "package dummy\n")

	runGitCommand(t, dir, "add", "dummy.go")
	runGitCommand(t, dir, "commit", "-m", "Update something without lore format")

	var buf bytes.Buffer
	result := checkLore(dir, &buf, false)
	assert.False(t, result, "checkLore must return false for an invalid commit")
}

func TestCheckLore_MissingSignOffOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initTestGitRepo(t, dir)
	writeTestFile(t, dir, "dummy.go", "package dummy\n")

	runGitCommand(t, dir, "add", "dummy.go")
	runGitCommand(
		t,
		dir,
		"commit",
		"-m",
		"fix(api): correct logic\n\nConstraint: preserve api contract",
	)

	var buf bytes.Buffer
	result := checkLore(dir, &buf, false)
	assert.False(t, result, "checkLore must return false when sign-off is missing")
	assert.Contains(t, buf.String(), "sign-off", "output must mention missing sign-off")
}

func TestCheckLore_QuietModeNoOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	var buf bytes.Buffer
	result := checkLore(dir, &buf, true)
	assert.True(t, result, "checkLore must return true when no commits exist")
	assert.Empty(t, buf.String(), "quiet mode must produce no output for the no-commit case")
}

func TestCheckLoreFromFile_ValidMessage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	msgPath := writeTestFile(
		t,
		dir,
		"COMMIT_EDITMSG",
		"feat(cli): add feature\n\nConstraint: keep cli stable\n\n🐙 Autopus <noreply@autopus.co>",
	)

	var buf bytes.Buffer
	result := checkLoreFromFile(msgPath, &buf, false)
	assert.True(t, result, "valid Lore message file should pass")
}

func TestCheckLoreFromFile_MissingRequiredTrailer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	msgPath := writeTestFile(
		t,
		dir,
		"COMMIT_EDITMSG",
		"feat(cli): add feature\n\n🐙 Autopus <noreply@autopus.co>",
	)

	var buf bytes.Buffer
	result := checkLoreFromFile(msgPath, &buf, false)
	assert.False(t, result, "message file missing required trailers should fail")
	assert.Contains(t, buf.String(), "Constraint")
}

func TestCheckLoreFromFile_InvalidMessage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	msgPath := writeTestFile(t, dir, "COMMIT_EDITMSG", "Update something")

	var buf bytes.Buffer
	result := checkLoreFromFile(msgPath, &buf, false)
	assert.False(t, result, "invalid message file should fail")
}
