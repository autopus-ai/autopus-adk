package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestUpdateCmd_WorkspaceApplyRollsBackCommittedTargetWhenLaterWriteFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	child := filepath.Join(root, "autopus-desktop")

	makeWorkspaceRepo(t, root)
	makeWorkspaceRepo(t, child)
	writeWorkspaceHarnessConfig(t, root, "meta-workspace")
	writeWorkspaceHarnessConfig(t, child, "autopus-desktop")
	rootCfg, err := config.LoadPreview(root)
	require.NoError(t, err)
	rootCfg.Quality.SupervisorModelPolicy = ""
	rootCfg.Orchestra.Providers["codex"] = config.ProviderEntry{
		Binary:      "codex",
		Args:        []string{"exec", "--sandbox", "workspace-write", "-m", config.CodexLegacyModel},
		ModelPolicy: config.ProviderModelPolicyPinned,
		Subprocess: config.SubprocessProvConf{
			SchemaFlag: "--output-schema",
			Timeout:    config.CodexOrchestraTimeoutSeconds,
		},
	}
	require.NoError(t, config.Save(root, rootCfg))
	childCfg, err := config.LoadPreview(child)
	require.NoError(t, err)
	childCfg.Quality.SupervisorModelPolicy = ""
	childCfg.Orchestra.Providers["codex"] = rootCfg.Orchestra.Providers["codex"]
	require.NoError(t, config.Save(child, childCfg))
	rootConfigBefore, err := os.ReadFile(filepath.Join(root, "autopus.yaml"))
	require.NoError(t, err)
	childConfigBefore, err := os.ReadFile(filepath.Join(child, "autopus.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(child, ".codex", "config.toml"), 0755))

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--dir", root, "--workspace", "--yes"})
	err = cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "autopus-desktop 업데이트 실패")
	assert.NoFileExists(t, filepath.Join(root, "AGENTS.md"))
	assert.NoFileExists(t, filepath.Join(root, ".codex", "config.toml"))
	assert.DirExists(t, filepath.Join(child, ".codex", "config.toml"))
	rootConfigAfter, readErr := os.ReadFile(filepath.Join(root, "autopus.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, rootConfigBefore, rootConfigAfter)
	childConfigAfter, readErr := os.ReadFile(filepath.Join(child, "autopus.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, childConfigBefore, childConfigAfter)
}
