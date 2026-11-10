package verifygate

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeToken builds a GitHub-token-shaped value at run time, so this file
// holds no literal a secret scanner would flag.
func fakeToken() string {
	return "gh" + "p_" + strings.Repeat("aZ3kP9qR2mX7", 3)
}

func TestAddedSecrets_FindsOnlyAddedLines(t *testing.T) {
	tok := fakeToken()
	base := map[string]string{
		"config.go":  "package demo\n\nconst name = \"demo\"\n",
		"legacy.txt": "old = " + tok + "\n",
		".gitignore": "local.env\n",
	}
	edit := map[string]string{
		// Line 4 is added; the committed legacy.txt secret is not this turn's.
		"config.go":                "package demo\n\nconst name = \"demo\"\nconst token = \"" + tok + "\"\n",
		"legacy.txt":               "old = " + tok + "\nnew = harmless\n",
		"notes/new.yaml":           "first: 1\nkey: " + tok + "\n",
		"fixture.go":               "var f = \"" + tok + "\" // autopus:allow-secret\n",
		"local.env":                "TOKEN=" + tok + "\n",
		".claude/settings.json":    "{\"t\": \"" + tok + "\"}\n",
		"bin/blob.dat":             "\x00\x01" + tok + "\n",
		"README.md":                "Use `gh auth token` for local runs.\n",
		"ignored/other_secret.txt": tok + "\n",
		".gitleaksignore":          "ignored/other_secret.txt:github-pat:1\n",
	}
	root := redRepo(t, base, edit)

	hits := AddedSecrets(context.Background(), root)
	var got []string
	for _, h := range hits {
		got = append(got, h.String())
	}
	assert.ElementsMatch(t, []string{"config.go:4 (github-pat)", "notes/new.yaml:2 (github-pat)"}, got)
	for _, h := range hits {
		assert.NotContains(t, h.String(), tok, "a hit never carries the value")
	}
}

func TestAddedSecrets_CleanTreeAndNonRepo(t *testing.T) {
	root := redRepo(t, map[string]string{"a.go": "package a\n"}, map[string]string{"a.go": "package a\n\nconst x = 1\n"})
	assert.Empty(t, AddedSecrets(context.Background(), root))
	assert.Empty(t, AddedSecrets(context.Background(), t.TempDir()))
}

func TestSecretHitsFingerprint_IsStableAndOrderFree(t *testing.T) {
	a := []SecretHit{{File: "a.go", Line: 1, Rule: "github-pat"}, {File: "b.go", Line: 2, Rule: "jwt"}}
	b := []SecretHit{a[1], a[0]}
	require.Equal(t, SecretHitsFingerprint(a), SecretHitsFingerprint(b))
	assert.NotEqual(t, SecretHitsFingerprint(a), SecretHitsFingerprint(a[:1]))
}
