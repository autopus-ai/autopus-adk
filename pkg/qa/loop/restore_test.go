package loop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// After a loop returns to the caller's branch its scenarios are only on the
// loop branch; a rerun gets them back as unstaged files, and a file already
// in the working tree is left as it is.
func TestRestoreLastSeed_BringsBackTheLoopBranchScenarios(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	repo.write(".autopus/qa/scenarios/login.yaml", "id: login\n")
	repo.write(".autopus/qa/scenarios/logout.yaml", "id: logout\n")
	report, err := repo.seededLoop(repo.deps(triage.ClassProductDefect, writing(repo, map[string]string{"fixed.txt": "ok\n"})))
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(repo.dir, ".autopus/qa/scenarios/login.yaml"))
	require.True(t, os.IsNotExist(statErr), "the caller's branch has no scenarios after the loop")

	repo.write(".autopus/qa/scenarios/logout.yaml", "id: logout-local\n")
	restored, err := RestoreLastSeed(repo.dir, seedPaths)

	require.NoError(t, err)
	assert.Equal(t, report.Branch, restored.Branch)
	assert.Equal(t, []string{".autopus/qa/scenarios/login.yaml"}, restored.Files)
	assert.Equal(t, "id: login\n", repo.read(".autopus/qa/scenarios/login.yaml"))
	assert.Equal(t, "id: logout-local\n", repo.read(".autopus/qa/scenarios/logout.yaml"))
	assert.Equal(t, "", repo.git("diff", "--cached", "--name-only"), "nothing is staged")
	assert.Equal(t, "broken\n", repo.read("fixed.txt"), "product fixes stay on the loop branch")
}

func TestRestoreLastSeed_NothingWithoutASeededBranch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	restored, err := RestoreLastSeed(repo.dir, seedPaths)
	require.NoError(t, err)
	assert.Empty(t, restored.Files)

	repo.write(".autopus/qa/scenarios/login.yaml", "id: login\n")
	report, err := repo.seededLoop(repo.deps(triage.ClassProductDefect, writing(repo, map[string]string{"fixed.txt": "ok\n"})))
	require.NoError(t, err)
	repo.git("branch", "-D", report.Branch)
	restored, err = RestoreLastSeed(repo.dir, seedPaths)
	require.NoError(t, err)
	assert.Empty(t, restored.Files, "a deleted branch is skipped")
}
