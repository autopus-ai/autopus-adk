package omp

// Security review F1 of SPEC-MODELUX-003, recovery: the transaction that
// releases the ledger keeps its backup of .omp/config.yml, so the bytes a
// release replaced stay recoverable; only that newest release's copy stays.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claimConfigBackups recovers, from every .omp/config.yml rollback artifact
// left under .autopus/backup, the bytes it would restore over the current
// file.
func claimConfigBackups(t *testing.T, root string) []string {
	t.Helper()
	current, err := os.ReadFile(filepath.Join(root, configFile))
	require.NoError(t, err)
	var recovered []string
	err = filepath.WalkDir(filepath.Join(root, ".autopus", "backup"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(filepath.ToSlash(path), "/"+configFile+".rollback.json") {
			return err
		}
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		artifact, decodeErr := decodeOMPConfigRollbackArtifact(data)
		require.NoError(t, decodeErr)
		before, applyErr := applyOMPConfigRollbackArtifact(artifact, current)
		require.NoError(t, applyErr)
		recovered = append(recovered, string(before))
		return nil
	})
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	return recovered
}

func TestOMPRelease_KeepsTheConfigBackupOfTheNewestRelease(t *testing.T) {
	for _, operation := range []string{"generate", "update"} {
		t.Run(operation, func(t *testing.T) {
			p := newClaimProject(t)
			p.write(configFile, "theme: dark\napiKey: user-secret-shape\n")
			p.mustRun(operation, configForOMP())
			edited := "theme: dark\napiKey: user-secret-shape\ntask:\n  disabledAgents:\n  - scout\n  - security-reviewer\n  - sonic\n  - designer\n  - librarian\n"
			p.write(configFile, edited)

			p.mustRun(operation, nativeConfigForOMP())
			p.mustRun(operation, nativeConfigForOMP())

			assert.Equal(t, []string{edited}, claimConfigBackups(t, p.root), "the release's backup outlives the run")
			p.mustRun(operation, configForOMP())
			claimed, _ := p.read(configFile)
			p.mustRun(operation, nativeConfigForOMP())
			assert.Equal(t, []string{claimed}, claimConfigBackups(t, p.root), "a later release replaces it")
		})
	}
}
