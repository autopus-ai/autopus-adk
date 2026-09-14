package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func updateVisibilityProject(t *testing.T, platforms ...string) string {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	cfg := config.DefaultFullConfig("update-visibility")
	cfg.Platforms = platforms
	cfg.Orchestra.Enabled = false
	require.NoError(t, config.Save(root, cfg))
	return root
}

func runVisibilityUpdate(root string) (string, error) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--local", "--yes", "--dir", root})
	err := cmd.Execute()
	return out.String(), err
}

func TestUpdateReportsOMPBeforeCompletionAlongsideOpenCode(t *testing.T) {
	root := updateVisibilityProject(t, "opencode", "omp")
	out, err := runVisibilityUpdate(root)
	require.NoError(t, err, out)
	target := strings.Index(out, "OMP (Oh My Pi)")
	complete := strings.Index(out, "omp updated")
	require.NotEqual(t, -1, target, "OMP must be named before waiting for its update")
	require.NotEqual(t, -1, complete)
	assert.Less(t, target, strings.Index(out, "opencode updated"), "all targets must be visible before any platform completes")
	assert.Less(t, target, complete)
	assert.FileExists(t, filepath.Join(root, ".autopus", "omp-manifest.json"))
	assert.FileExists(t, filepath.Join(root, "opencode.json"))
}

func TestUpdateNamesOMPWhenItsUpdateFails(t *testing.T) {
	root := updateVisibilityProject(t, "omp")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".omp"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".omp", "commands"), []byte("user file"), 0o600))
	out, err := runVisibilityUpdate(root)
	require.Error(t, err)
	assert.Contains(t, out, "OMP (Oh My Pi)")
	assert.NotContains(t, out, "omp updated", "a failed platform must not be reported as updated")
}

func TestWorkspacePreflightNamesEveryPlatformIncludingOMP(t *testing.T) {
	root := updateVisibilityProject(t, "opencode", "omp")
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := preflightWorkspaceUpdateTarget(context.Background(), cmd, workspaceUpdateTarget{
		Name: "sample", Path: "sample", AbsPath: root,
	}, "")
	require.NoError(t, err, out.String())
	assert.Contains(t, out.String(), "sample")
	assert.Contains(t, out.String(), "opencode")
	assert.Contains(t, out.String(), "OMP (Oh My Pi)")
	assert.NoDirExists(t, filepath.Join(root, ".omp"), "preflight must remain write-free")
}
