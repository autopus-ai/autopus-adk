package omp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMPLifecycle_AncestorSwapStaysOnOpenedWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("generate", func(t *testing.T) {
		root, moved, outside, swap := ompAncestorSwapFixture(t)
		a := NewWithRoot(root)
		a.rootedWorkspaceHook = swap
		_, err := a.Generate(context.Background(), configForOMP())
		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(moved, ".autopus", "omp-manifest.json"))
		assert.Equal(t, []byte("external:"+configFile), mustReadOMPReviewFile(t, filepath.Join(outside, configFile)))
		assert.NoFileExists(t, filepath.Join(outside, ".autopus", "omp-manifest.json"))
	})

	t.Run("update", func(t *testing.T) {
		root, moved, outside, swap := ompAncestorSwapFixture(t)
		mustWriteOMPTestFile(t, filepath.Join(root, configFile), []byte("user: keep\n"), 0o640)
		a := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner())
		cfg := integrationHarnessConfig(config.RoleModelConfigModeProjectManaged)
		_, err := a.Generate(context.Background(), cfg)
		require.NoError(t, err)
		a.rootedWorkspaceHook = swap
		_, err = a.Update(context.Background(), cfg)
		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(moved, ".autopus", "omp-manifest.json"))
		assert.FileExists(t, filepath.Join(moved, OMPModelProjectOwnershipRelativePath))
		assert.FileExists(t, filepath.Join(moved, OMPModelReceiptRelativePath))
		assert.Equal(t, []byte("external:"+configFile), mustReadOMPReviewFile(t, filepath.Join(outside, configFile)))
		assert.Equal(t, []byte("external:"+OMPModelProjectOwnershipRelativePath), mustReadOMPReviewFile(t, filepath.Join(outside, OMPModelProjectOwnershipRelativePath)))
		assert.Equal(t, []byte("external:"+OMPModelReceiptRelativePath), mustReadOMPReviewFile(t, filepath.Join(outside, OMPModelReceiptRelativePath)))
		assert.NoFileExists(t, filepath.Join(outside, ".autopus", "omp-manifest.json"))
	})
}

func TestOMPStandaloneModelWriters_AncestorSwapStaysOnOpenedWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("overlay", func(t *testing.T) {
		root, moved, outside, swap := ompAncestorSwapFixture(t)
		workspace, err := openOMPRootedWorkspace(root)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, workspace.Close()) })
		swap()
		_, err = writeOMPModelOverlayAt(workspace, OMPModelOverlayWriteInput{
			Projection: OMPModelOverlayProjection{AgentModelOverrides: map[string]string{"task": "p/m"}},
		})
		require.NoError(t, err)
		assert.Contains(t, string(mustReadOMPReviewFile(t, filepath.Join(moved, DefaultOMPModelOverlayPath))), "task: p/m")
		assert.Equal(t, []byte("external:"+DefaultOMPModelOverlayPath), mustReadOMPReviewFile(t, filepath.Join(outside, DefaultOMPModelOverlayPath)))
	})

	t.Run("receipt", func(t *testing.T) {
		root, moved, outside, swap := ompAncestorSwapFixture(t)
		workspace, err := openOMPRootedWorkspace(root)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, workspace.Close()) })
		swap()
		_, err = writeOMPModelResolutionReceiptAt(workspace, OMPModelReceiptWriteInput{
			Receipt: modelReceiptFixture(time.Now().UTC()),
		})
		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(moved, OMPModelReceiptRelativePath))
		assert.Equal(t, []byte("external:"+OMPModelReceiptRelativePath), mustReadOMPReviewFile(t, filepath.Join(outside, OMPModelReceiptRelativePath)))
	})
}

func TestOMPRootedTransaction_AncestorSwapDoesNotRedirectWritesOrRollback(t *testing.T) {
	t.Parallel()

	t.Run("model files and manifest", func(t *testing.T) {
		root, moved, outside, swap := ompAncestorSwapFixture(t)
		workspace, err := openOMPRootedWorkspace(root)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, workspace.Close()) })
		swap()
		paths := []string{configFile, DefaultOMPModelOverlayPath, OMPModelProjectOwnershipRelativePath, OMPModelReceiptRelativePath}
		writes := make([]adapter.TransactionWrite, 0, len(paths))
		manifest := adapter.NewManifest(adapterName)
		for _, path := range paths {
			writes = append(writes, adapter.TransactionWrite{Path: path, Content: []byte("owned:" + path), Perm: 0o600})
			manifest.Files[path] = adapter.ManifestFile{Checksum: adapter.Checksum("owned:" + path), Policy: adapter.OverwriteAlways}
		}
		_, err = applyOMPTransactionAt(workspace, adapterName, adapter.TransactionPlan{Writes: writes, Manifest: manifest})
		require.NoError(t, err)
		for _, path := range paths {
			assert.Equal(t, []byte("owned:"+path), mustReadOMPReviewFile(t, filepath.Join(moved, path)))
			assert.Equal(t, []byte("external:"+path), mustReadOMPReviewFile(t, filepath.Join(outside, path)))
		}
		assert.FileExists(t, filepath.Join(moved, ".autopus", "omp-manifest.json"))
		assert.NoFileExists(t, filepath.Join(outside, ".autopus", "omp-manifest.json"))
	})

	t.Run("late failure rollback", func(t *testing.T) {
		root, moved, outside, swap := ompAncestorSwapFixture(t)
		original := []byte("user: original\n")
		mustWriteOMPTestFile(t, filepath.Join(root, configFile), original, 0o640)
		mustWriteOMPTestFile(t, filepath.Join(root, "blocker"), []byte("blocker"), 0o600)
		workspace, err := openOMPRootedWorkspace(root)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, workspace.Close()) })
		swap()
		_, err = applyOMPTransactionAt(workspace, adapterName, adapter.TransactionPlan{Writes: []adapter.TransactionWrite{
			{Path: configFile, Content: []byte("managed\n"), Perm: 0o640},
			{Path: "blocker/late.yml", Content: []byte("fail\n"), Perm: 0o600},
		}})
		require.Error(t, err)
		assert.Equal(t, original, mustReadOMPReviewFile(t, filepath.Join(moved, configFile)))
		assert.Equal(t, []byte("external:"+configFile), mustReadOMPReviewFile(t, filepath.Join(outside, configFile)))
	})
}

func TestOMPProjectClean_AncestorSwapDoesNotRedirectRestoreOrDelete(t *testing.T) {
	t.Parallel()
	container := t.TempDir()
	root := filepath.Join(container, "workspace")
	require.NoError(t, os.Mkdir(root, 0o700))
	original := []byte("# exact\r\nuser: ${TOKEN}\r\n")
	mustWriteOMPTestFile(t, filepath.Join(root, configFile), original, 0o640)
	a := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner())
	_, err := a.Generate(context.Background(), integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))
	require.NoError(t, err)
	workspace, err := openOMPRootedWorkspace(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, workspace.Close()) })
	plan, err := a.prepareCleanAt(workspace)
	require.NoError(t, err)

	moved := filepath.Join(container, "moved")
	require.NoError(t, os.Rename(root, moved))
	outside := t.TempDir()
	external := make(map[string][]byte)
	for _, step := range plan.steps {
		body := []byte("external:" + step.relPath)
		external[step.relPath] = body
		mustWriteOMPTestFile(t, filepath.Join(outside, step.relPath), body, 0o600)
	}
	manifestBody := []byte("external-manifest")
	mustWriteOMPTestFile(t, filepath.Join(outside, plan.manifestRel), manifestBody, 0o600)
	require.NoError(t, os.Symlink(outside, root))

	receipt, err := a.applyCleanAt(workspace, plan)
	require.NoError(t, err)
	require.NotEmpty(t, receipt.ChangedPaths)
	assert.Equal(t, original, mustReadOMPReviewFile(t, filepath.Join(moved, configFile)))
	assert.NoFileExists(t, filepath.Join(moved, OMPModelProjectOwnershipRelativePath))
	assert.NoFileExists(t, filepath.Join(moved, OMPModelReceiptRelativePath))
	assert.NoFileExists(t, filepath.Join(moved, plan.manifestRel))
	for path, body := range external {
		assert.Equal(t, body, mustReadOMPReviewFile(t, filepath.Join(outside, path)))
	}
	assert.Equal(t, manifestBody, mustReadOMPReviewFile(t, filepath.Join(outside, plan.manifestRel)))
}

func ompAncestorSwapFixture(t *testing.T) (string, string, string, func()) {
	t.Helper()
	container := t.TempDir()
	root := filepath.Join(container, "workspace")
	require.NoError(t, os.Mkdir(root, 0o700))
	moved := filepath.Join(container, "moved")
	outside := t.TempDir()
	for _, path := range []string{configFile, DefaultOMPModelOverlayPath, OMPModelProjectOwnershipRelativePath, OMPModelReceiptRelativePath} {
		mustWriteOMPTestFile(t, filepath.Join(outside, path), []byte("external:"+path), 0o600)
	}
	return root, moved, outside, func() {
		require.NoError(t, os.Rename(root, moved))
		require.NoError(t, os.Symlink(outside, root))
	}
}

func mustWriteOMPTestFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, mode))
}
