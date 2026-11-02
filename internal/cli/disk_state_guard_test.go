package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// diskTestStateRoot holds the disk state dir of every test in this process;
// TestMain removes it.
var diskTestStateRoot = filepath.Join(os.TempDir(), fmt.Sprintf("autopus-disk-state-test-%d", os.Getpid()))

// init points the disk state dir at diskTestStateRoot before any test runs,
// so no test that forgets withDiskSeams writes to the real
// ~/.local/state/autopus/disk.
func init() {
	dir := filepath.Join(diskTestStateRoot, "autopus", "disk")
	diskStatePath = func() string { return dir }
}
