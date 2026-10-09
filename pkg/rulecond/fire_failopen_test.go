package rulecond_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFire_BenignAbsenceFailsOpen is the S4 oracle for REQ-CONDRULE-FIRE-03.
// Every row must produce empty stdout, empty stderr, and no error.
func TestFire_BenignAbsenceFailsOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, f *fixture)
		stdin string
	}{
		{
			name:  "manifest file absent",
			setup: func(t *testing.T, f *fixture) { f.removeManifest() },
			stdin: bashPayload("git commit -m x"),
		},
		{
			name: "manifest is not json",
			setup: func(t *testing.T, f *fixture) {
				f.writeRawManifest("{not json")
			},
			stdin: bashPayload("git commit -m x"),
		},
		{
			name: "manifest rule regex does not compile",
			setup: func(t *testing.T, f *fixture) {
				f.writeBody("lore-commit.md", "# Lore Commit\n")
				f.writeManifest(bashRule("lore-commit", "a(", "lore-commit.md"))
			},
			stdin: bashPayload("git commit -m x"),
		},
		{
			name: "stdin is empty",
			setup: func(t *testing.T, f *fixture) {
				f.writeBody("lore-commit.md", "# Lore Commit\n")
				f.writeManifest(bashRule("lore-commit", condLoreCommit, "lore-commit.md"))
			},
			stdin: "",
		},
		{
			name: "stdin is not json",
			setup: func(t *testing.T, f *fixture) {
				f.writeBody("lore-commit.md", "# Lore Commit\n")
				f.writeManifest(bashRule("lore-commit", condLoreCommit, "lore-commit.md"))
			},
			stdin: "not json",
		},
		{
			name: "named body file does not exist",
			setup: func(t *testing.T, f *fixture) {
				f.writeManifest(bashRule("lore-commit", condLoreCommit, "lore-commit.md"))
			},
			stdin: bashPayload("git commit -m x"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			tt.setup(t, f)

			stdout, stderr := f.fire(tt.stdin)

			assert.Empty(t, stdout, "benign absence must produce empty stdout")
			assert.Empty(t, stderr, "benign absence must not write a diagnostic")
			assert.NotContains(t, stdout, "permissionDecision")
			assert.NotContains(t, stdout, "continue")
		})
	}
}

// TestFire_ToolInputNeverReentersContext is the S5 oracle for
// REQ-CONDRULE-FIRE-04.
func TestFire_ToolInputNeverReentersContext(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.writeBody("lore-commit.md", sourceRuleBody(t, "lore-commit"))
	f.writeManifest(bashRule("lore-commit", condLoreCommit, "lore-commit.md"))

	// The token is joined from fragments so no provider-format literal is
	// committed; the bytes are unchanged.
	const token = "ghp_" + "EXAMPLEFAKETOKEN"
	command := "export GH_TOKEN=" + token + " && git commit -m x"
	stdout, stderr := f.fire(bashPayload(command))

	ctx := injectedContext(t, stdout)
	require.Contains(t, ctx, loreCommitSignature)

	assert.NotContains(t, ctx, token)
	assert.NotContains(t, ctx, "export GH_TOKEN")
	assert.NotContains(t, ctx, "git commit -m x")
	assert.NotContains(t, stdout, token)
	assert.NotContains(t, stderr, token)
}
