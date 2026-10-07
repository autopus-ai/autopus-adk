package editguard

import (
	"os"
	"path/filepath"
	"testing"
)

// newProject returns a symlink-free temp project root that holds autopus.yaml.
func newProject(t *testing.T) string {
	t.Helper()
	root := realDir(t, t.TempDir())
	writeFile(t, root, projectMarker, "project:\n  name: editguard-test\n")
	return root
}

// realDir resolves the system prefix (macOS /var -> /private/var) so expected
// paths compare equal to resolved ones.
func realDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// volumeFoldsCase reports whether dir lives on a case-insensitive volume.
func volumeFoldsCase(t *testing.T, dir string) bool {
	t.Helper()
	probe := writeFile(t, dir, "case-probe", "x")
	_, err := os.Stat(filepath.Join(dir, "CASE-PROBE"))
	if removeErr := os.Remove(probe); removeErr != nil {
		t.Fatal(removeErr)
	}
	return err == nil
}
