package verifygate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func treeHash(t *testing.T, root string) string {
	t.Helper()
	return TreeHash(context.Background(), root, "3m0s")
}

// Review B1: everything git reports about the tree is part of its hash,
// including what the plan leaves out.
func TestTreeHash_ChangesWithEveryTreeChange(t *testing.T) {
	root := redRepo(t, map[string]string{"go.mod": baseModule, "add.go": baseAdd, "helper.go": "package basegate\n",
		"run.sh": "#!/bin/sh\n", ".autopus/specs/golden.txt": "3\n"}, map[string]string{"add.go": baseAddFix})
	seen := map[string]string{}
	step := func(name string, change func()) {
		change()
		h := treeHash(t, root)
		for prev, ph := range seen {
			require.NotEqual(t, ph, h, "%s hashes like %s", name, prev)
		}
		seen[name] = h
	}
	step("edit", func() {})
	require.NotEqual(t, seen["edit"], TreeHash(context.Background(), root, "5m0s"), "the budget is part of the run")
	step("delete a tracked file", func() { require.NoError(t, os.Remove(filepath.Join(root, "helper.go"))) })
	step("chmod", func() { require.NoError(t, os.Chmod(filepath.Join(root, "run.sh"), 0o755)) })
	step("harness file", func() { writeFiles(t, root, map[string]string{".autopus/specs/golden.txt": "4\n"}) })
	step("rename", func() {
		gitRun(t, root, "mv", "run.sh", "go.sh")
	})
	step("symlink", func() { require.NoError(t, os.Symlink("add.go", filepath.Join(root, "link.go"))) })
	step("relink", func() {
		require.NoError(t, os.Remove(filepath.Join(root, "link.go")))
		require.NoError(t, os.Symlink("go.mod", filepath.Join(root, "link.go")))
	})
	assert.Equal(t, treeHash(t, root), treeHash(t, root), "the same tree hashes the same")
}

// Review A5: an empty file, a file holding a NUL and an unreadable file are
// three different trees.
func TestTreeHash_ContentsDoNotCollide(t *testing.T) {
	root := redRepo(t, map[string]string{"go.mod": baseModule}, nil)
	path := filepath.Join(root, "x.txt")
	require.NoError(t, os.WriteFile(path, nil, 0o644))
	empty := treeHash(t, root)
	require.NoError(t, os.WriteFile(path, []byte{0}, 0o644))
	nul := treeHash(t, root)
	require.NoError(t, os.WriteFile(path, nil, 0o644))
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	unreadable := treeHash(t, root)
	assert.NotEqual(t, empty, nul)
	assert.NotEqual(t, empty, unreadable)
	assert.NotEqual(t, nul, unreadable)
}

// Review of #374 (M2): the same changes on another commit are another tree.
func TestTreeHash_HEADIsPartOfTheTree(t *testing.T) {
	root := redRepo(t, map[string]string{"go.mod": baseModule, "add.go": baseAdd}, map[string]string{"add.go": baseAddFix})
	before := treeHash(t, root)
	gitRun(t, root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "empty")
	assert.NotEqual(t, before, treeHash(t, root), "only HEAD moved")
}

// Review of #374: untracked harness state (QA journeys and scenarios, written
// at every stop and deliberately not ignored) does not make a new tree;
// tracked harness files still do.
func TestTreeHash_IgnoresUntrackedHarnessState(t *testing.T) {
	root := redRepo(t, map[string]string{"go.mod": baseModule, "add.go": baseAdd, ".autopus/qa/capture/fixture.txt": "a\n"},
		map[string]string{"add.go": baseAddFix})
	before := treeHash(t, root)
	writeFiles(t, root, map[string]string{".autopus/qa/journeys/j1.json": "{}\n", ".autopus/qa/scenarios/round.yaml": "id: r\n",
		".claude/state.json": "{}\n"})
	assert.Equal(t, before, treeHash(t, root), "untracked harness files are not part of the tree")
	writeFiles(t, root, map[string]string{".autopus/qa/capture/fixture.txt": "b\n"})
	assert.NotEqual(t, before, treeHash(t, root), "a tracked harness file is")
}
