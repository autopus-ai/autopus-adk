//go:build !windows

package intake

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// symlinkFixture creates an outside directory O holding one file and returns
// the project root and O.
func symlinkFixture(t *testing.T) (root, outside string) {
	t.Helper()
	root, outside = t.TempDir(), t.TempDir()
	writeFile(t, outside, "GTC-023e9302ff0b.json", "outside\n")
	return root, outside
}

func link(t *testing.T, target, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.Symlink(target, full))
}

func TestArea_CheckLayout_RejectsEachSymlinkedDirectory(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{"evals", "evals/harness", IntakeDir, PromotedDir, RejectedDir, "evals/harness/tasks", SurfaceTaskDir} {
		root, outside := symlinkFixture(t)
		link(t, outside, root, dir)
		before := treeDigest(t, outside)

		err := openTestArea(t, root).checkLayout()

		assert.ErrorIs(t, err, errPathUnsafe, "symlinked %s", dir)
		assert.Equal(t, before, treeDigest(t, outside))
	}
}

func TestArea_ReadAndList_RejectSymlinks(t *testing.T) {
	t.Parallel()
	root, outside := symlinkFixture(t)
	link(t, filepath.Join(outside, "GTC-023e9302ff0b.json"), root, IntakeDir+"/GTC-023e9302ff0b.json")
	link(t, outside, root, SurfaceTaskDir+"/nested")
	a := openTestArea(t, root)

	_, err := a.readFile(IntakeDir + "/GTC-023e9302ff0b.json")
	assert.ErrorIs(t, err, errPathUnsafe)
	_, err = a.listDir(SurfaceTaskDir + "/nested")
	assert.ErrorIs(t, err, errPathUnsafe)
	err = a.walkJSON(SurfaceTaskDir, func(string, []byte) error { return nil })
	assert.ErrorIs(t, err, errPathUnsafe)
}

func TestArea_EnsureDir_CreatesMissingRealDirectories(t *testing.T) {
	t.Parallel()
	root, outside := symlinkFixture(t)
	a := openTestArea(t, root)

	require.NoError(t, a.ensureDir(PromotedDir))
	exists, err := a.dirExists(PromotedDir)
	require.NoError(t, err)
	assert.True(t, exists)
	require.NoError(t, a.ensureDir(PromotedDir), "existing directories are accepted")

	link(t, outside, root, "evals/harness/tasks")
	assert.ErrorIs(t, a.ensureDir(SurfaceTaskDir), errPathUnsafe)
	assert.NoDirExists(t, filepath.Join(outside, "surface"))
}

func TestArea_CreateExclusive_PublishesOnceAndLeavesNoTemp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a := openTestArea(t, root)
	require.NoError(t, a.ensureDir(IntakeDir))
	rel := IntakeDir + "/GTC-023e9302ff0b.json"

	require.NoError(t, a.createExclusive(rel, []byte("first\n")))
	err := a.createExclusive(rel, []byte("second\n"))

	assert.ErrorIs(t, err, fs.ErrExist)
	assert.Equal(t, "first\n", readFile(t, root, rel))
	info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, statErr)
	assert.True(t, info.Mode().IsRegular())
	assert.Equal(t, []string{"GTC-023e9302ff0b.json"}, dirNames(t, root, IntakeDir))
}

func TestArea_CreateExclusive_RemovesStaleTempOfSameName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, IntakeDir+"/.GTC-023e9302ff0b.json.tmp-0011223344556677", "partial")
	writeFile(t, root, IntakeDir+"/.GTC-8e80c7a18029.json.tmp-0011223344556677", "other")
	a := openTestArea(t, root)

	require.NoError(t, a.createExclusive(IntakeDir+"/GTC-023e9302ff0b.json", []byte("x\n")))

	assert.Equal(t, []string{".GTC-8e80c7a18029.json.tmp-0011223344556677", "GTC-023e9302ff0b.json"}, dirNames(t, root, IntakeDir))
}

func TestArea_CreateExclusive_SymlinkedParentIsUnsafe(t *testing.T) {
	t.Parallel()
	root, outside := symlinkFixture(t)
	link(t, outside, root, IntakeDir)
	before := treeDigest(t, outside)

	err := openTestArea(t, root).createExclusive(IntakeDir+"/GTC-8e80c7a18029.json", []byte("x\n"))

	assert.ErrorIs(t, err, errPathUnsafe)
	assert.Equal(t, before, treeDigest(t, outside))
}

func TestArea_RemoveFile_RemovesOnlyRegularFiles(t *testing.T) {
	t.Parallel()
	root, outside := symlinkFixture(t)
	writeFile(t, root, IntakeDir+"/GTC-8e80c7a18029.json", "x\n")
	link(t, filepath.Join(outside, "GTC-023e9302ff0b.json"), root, IntakeDir+"/GTC-023e9302ff0b.json")
	a := openTestArea(t, root)

	require.NoError(t, a.removeFile(IntakeDir+"/GTC-8e80c7a18029.json"))
	assert.ErrorIs(t, a.removeFile(IntakeDir+"/GTC-023e9302ff0b.json"), errPathUnsafe)
	assert.ErrorIs(t, a.removeFile(IntakeDir+"/GTC-000000000000.json"), fs.ErrNotExist)

	assert.Equal(t, []string{"GTC-023e9302ff0b.json"}, dirNames(t, root, IntakeDir))
	assert.Equal(t, "outside\n", readFile(t, outside, "GTC-023e9302ff0b.json"))
}

func TestWriteSupported_OnUnix_ReturnsNil(t *testing.T) {
	t.Parallel()
	assert.NoError(t, writeSupported())
}
