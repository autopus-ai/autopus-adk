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
