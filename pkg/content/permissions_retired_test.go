package content_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

func TestDetectPermissions_DefaultsAreNarrowPrefixes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644))

	perms := content.DetectPermissions(dir, config.PermissionsConf{})

	for _, value := range perms.Allow {
		assert.False(t, content.IsRetiredDefaultAllow(value), "retired default %q still emitted", value)
		if strings.HasPrefix(value, "Bash(") {
			assert.NotContains(t, value, " *)", "space-wildcard entry %q", value)
		}
	}
	for _, blanket := range []string{"Bash(auto:*)", "Bash(git:*)", "Bash(gh:*)", "Bash(make:*)", "Bash(find:*)",
		"Bash(go run:*)", "Bash(go install:*)", "Bash(auto disk lease:*)", "Bash(auto doctor:*)"} {
		assert.NotContains(t, perms.Allow, blanket)
	}
	assert.Contains(t, perms.Allow, "Bash(auto doctor)")
	assert.Contains(t, perms.Allow, "Bash(go test:*)")
}

func TestWithoutRetiredDefaultAllows_FingerprintedBlockRetires(t *testing.T) {
	t.Parallel()
	existing := []string{"Bash(auto *)", "Bash(auto:*)", "Bash(git *)", "Bash(git:*)", "Bash(make:*)",
		"Bash(find:*)", "Bash(gh:*)", "Bash(ls:*)", "UserAllow", "Bash(npx *)"}

	got := content.WithoutRetiredDefaultAllows(existing, []string{"Bash(ls:*)"}, nil)

	// The common block carries its fingerprint; the lone node entry does not.
	assert.Equal(t, []string{"Bash(ls:*)", "UserAllow", "Bash(npx *)"}, got)
}

func TestWithoutRetiredDefaultAllows_ClaimedAndDesired(t *testing.T) {
	t.Parallel()
	existing := []string{"Bash(git *)", "Bash(gh:*)", "Bash(go run:*)", "UserAllow"}

	got := content.WithoutRetiredDefaultAllows(existing,
		[]string{"Bash(go run:*)"}, // the user's ExtraAllow keeps it
		[]string{"Bash(gh:*)", "Bash(go run:*)", "UserAllow"})

	// Unclaimed partial block stays (hand-added); claimed retired entry goes;
	// a claimed non-retired entry is not this function's concern.
	assert.Equal(t, []string{"Bash(git *)", "Bash(go run:*)", "UserAllow"}, got)
}
