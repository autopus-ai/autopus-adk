//go:build unix

package healthband

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Security L4: a path swapped for a FIFO or a symlink between the check and
// the open is refused at once; the open never blocks waiting for a writer
// and never follows the link. Not parallel: it replaces beforeStoreOpen.
func TestOpenRegular_RefusesAPathSwappedAfterTheCheck(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside the project\n"), 0o600))
	for name, swap := range map[string]func(string) error{
		"fifo":    func(path string) error { return syscall.Mkfifo(path, 0o600) },
		"symlink": func(path string) error { return os.Symlink(outside, path) },
	} {
		path := filepath.Join(dir, name+".md")
		require.NoError(t, os.WriteFile(path, []byte("report\n"), 0o600))
		beforeStoreOpen = func(target string) {
			require.NoError(t, os.Remove(target))
			require.NoError(t, swap(target))
		}
		done := make(chan error, 1)
		go func() {
			file, err := OpenRegular(path)
			if file != nil {
				_ = file.Close()
			}
			done <- err
		}()
		select {
		case err := <-done:
			assert.ErrorIs(t, err, errUnsafeStorePath, name)
		case <-time.After(2 * time.Second):
			// Release the blocked reader before failing.
			if writer, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
				_ = writer.Close()
			}
			t.Fatalf("%s: the open blocked", name)
		}
		beforeStoreOpen = func(string) {}
	}
}
