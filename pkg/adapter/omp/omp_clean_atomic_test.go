package omp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ompCleanFileState struct {
	data []byte
	mode os.FileMode
}

func TestOMPClean_LateFailureRollsBackEveryMutation(t *testing.T) {
	root := t.TempDir()
	original := []byte("# user\r\nsecret: ${TOKEN}\r\n")
	mustWriteOMPTestFile(t, filepath.Join(root, configFile), original, 0o640)
	a := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner())
	_, err := a.Generate(context.Background(), integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))
	require.NoError(t, err)

	changed := filepath.Join(root, ".omp", "commands", "auto.md")
	require.NoError(t, os.WriteFile(changed, []byte("user changed\n"), 0o620))
	workspace, err := openOMPRootedWorkspace(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, workspace.Close()) })
	plan, err := a.prepareCleanAt(workspace)
	require.NoError(t, err)
	require.NotEmpty(t, plan.steps)

	before := make(map[string]ompCleanFileState, len(plan.steps)+1)
	for _, step := range plan.steps {
		data, info, readErr := workspace.readFile(step.relPath, 4<<20)
		require.NoError(t, readErr)
		before[step.relPath] = ompCleanFileState{data: data, mode: info.Mode().Perm()}
	}
	manifest, info, err := workspace.readFile(plan.manifestRel, 4<<20)
	require.NoError(t, err)
	before[plan.manifestRel] = ompCleanFileState{data: manifest, mode: info.Mode().Perm()}
	backupBefore := snapshotOMPCleanTree(t, filepath.Join(root, ".autopus", "backup"))

	receipt, err := a.applyCleanAtWithHook(workspace, plan, func(path string) error {
		if path == plan.manifestRel {
			return errors.New("injected late clean failure")
		}
		return nil
	})
	require.ErrorContains(t, err, "injected late clean failure")
	assert.Empty(t, receipt.ChangedPaths)
	for path, want := range before {
		data, gotInfo, readErr := workspace.readFile(path, 4<<20)
		require.NoError(t, readErr, path)
		assert.Equal(t, want.data, data, path)
		assert.Equal(t, want.mode, gotInfo.Mode().Perm(), path)
	}
	assert.Equal(t, backupBefore, snapshotOMPCleanTree(t, filepath.Join(root, ".autopus", "backup")))
}

func snapshotOMPCleanTree(t *testing.T, root string) map[string]ompCleanFileState {
	t.Helper()
	result := make(map[string]ompCleanFileState)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		state := ompCleanFileState{mode: info.Mode()}
		if info.Mode().IsRegular() {
			state.data, err = os.ReadFile(path)
		}
		result[rel] = state
		return err
	})
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	return result
}
