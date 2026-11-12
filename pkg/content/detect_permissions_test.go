// Package content_test verifies DetectPermissions behavior.
package content_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

func TestDetectPermissions_DefaultOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	perms := content.DetectPermissions(dir, config.PermissionsConf{})

	assert.NotNil(t, perms)
	assert.Contains(t, perms.Allow, "Bash(auto status:*)")
	assert.Contains(t, perms.Allow, "Bash(git status:*)")
	assert.Contains(t, perms.Allow, "WebSearch")
	assert.NotContains(t, perms.Allow, "Bash(go test:*)")
	assert.NotContains(t, perms.Allow, "Bash(npm *)")
}

func TestDetectPermissions_GoProject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644))

	perms := content.DetectPermissions(dir, config.PermissionsConf{})

	assert.Contains(t, perms.Allow, "Bash(go test:*)")
	assert.Contains(t, perms.Allow, "Bash(go build:*)")
	assert.Contains(t, perms.Allow, "Bash(golangci-lint:*)")
	assert.NotContains(t, perms.Allow, "Bash(npm *)")
}

func TestDetectPermissions_NodeProject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644))

	perms := content.DetectPermissions(dir, config.PermissionsConf{})

	assert.Contains(t, perms.Allow, "Bash(npm test:*)")
	assert.Contains(t, perms.Allow, "Bash(pnpm run build:*)")
	assert.NotContains(t, perms.Allow, "Bash(npx *)")
	assert.NotContains(t, perms.Allow, "Bash(go test:*)")
}

func TestDetectPermissions_ExtraPerms(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	extra := config.PermissionsConf{
		ExtraAllow: []string{"Bash(cargo build:*)"},
		ExtraDeny:  []string{"Bash(rm -rf:*)"},
	}

	perms := content.DetectPermissions(dir, extra)

	assert.Contains(t, perms.Allow, "Bash(cargo build:*)")
	assert.Contains(t, perms.Deny, "Bash(rm -rf:*)")
	assert.Contains(t, perms.Allow, "Bash(auto status:*)")
}
