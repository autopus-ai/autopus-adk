package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		// The lore-commit rule documents merge(<scope>) for required merge commits.
		"merge(release): integrate rc branch",
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
		{"empty scope", "revert(): no scope"},
		{"blank scope", "revert(  ): no scope"},
		{"unclosed scope", "revert(api: missing closing parenthesis"},
		{"missing separator", "revert(api) missing colon"},
		{"empty subject", "revert(api): "},
		{"blank subject", "revert(api):   "},
		{"nested scope", "revert(a(b)): invalid scope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, hasValidLoreType(tt.msg), "expected invalid Lore type for: %q", tt.msg)
		})
	}
}

func TestLoreHookAndValidatorAgreeOnSubjects(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("a POSIX shell is required to run the hook")
	}
	hook, err := filepath.Abs(filepath.Join("..", "..", "content", "hooks", "pre-commit-lore.sh"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		msg   string
		valid bool
	}{
		{"feat(cli): add a feature", true},
		{"fix(cli): correct a bug", true},
		{"refactor(cli): simplify code", true},
		{"test(cli): cover behavior", true},
		{"docs(cli): clarify usage", true},
		{"chore(cli): update config", true},
		{"perf(cli): improve speed", true},
		{"merge(release): integrate branch", true},
		{"revert(cli): undo a change", true},
		{"revert(cli): undo (a change)\n\nConstraint: preserve behavior", true},
		{"ci(cli): run checks", false},
		{"feat: missing scope", false},
		{"revert(): empty scope", false},
		{"revert(  ): blank scope", false},
		{"revert(cli: unclosed scope", false},
		{"revert(cli) missing separator", false},
		{"revert(cli):   ", false},
		{"revert(a(b)): nested scope", false},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			messageFile := filepath.Join(t.TempDir(), "commit-msg")
			if err := os.WriteFile(messageFile, []byte(tt.msg), 0o600); err != nil {
				t.Fatal(err)
			}
			output, hookErr := exec.Command(sh, hook, messageFile).CombinedOutput()
			assert.Equal(t, tt.valid, hasValidLoreType(tt.msg), "validator result")
			assert.Equal(t, tt.valid, hookErr == nil, "hook output: %s", output)
		})
	}
}

func TestLoreTypeTablesMatchValidator(t *testing.T) {
	files := []struct {
		path    string
		heading string
	}{
		{"content/rules/lore-commit.md", "## Types"},
		{"content/skills/lore-commit.md", "### 타입 분류"},
		{"templates/codex/skills/lore-commit.md.tmpl", "### 타입 분류"},
		{"templates/gemini/skills/lore-commit/SKILL.md.tmpl", "### 타입 분류"},
	}
	want := make([]string, 0, len(loreValidTypes))
	for _, prefix := range loreValidTypes {
		want = append(want, strings.TrimSuffix(prefix, "("))
	}
	for _, file := range files {
		t.Run(file.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", file.path))
			if err != nil {
				t.Fatal(err)
			}
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			_, table, found := strings.Cut(text, file.heading+"\n")
			if !found {
				t.Fatalf("missing heading %q", file.heading)
			}
			table, _, _ = strings.Cut(table, "\n## ")
			var got []string
			for _, line := range strings.Split(table, "\n") {
				cells := strings.Split(line, "|")
				if len(cells) < 4 {
					continue
				}
				typeName := strings.Trim(strings.TrimSpace(cells[1]), "`")
				if typeName != "" && strings.Trim(typeName, "abcdefghijklmnopqrstuvwxyz") == "" {
					got = append(got, typeName)
				}
			}
			assert.ElementsMatch(t, want, got)
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
