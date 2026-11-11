package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/diskspace"
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
	// No test walks the developer's package caches or VM disks, and process
	// working directories read as known and holding nothing under a fixture.
	diskConsumerHome = func() string { return "" }
	diskGoModCache = func() string { return "" }
	diskCwdLister = func() diskspace.CwdLister {
		return func(context.Context) ([]string, error) { return []string{"/"}, nil }
	}
	// A spawned reclaim would start the test binary itself.
	diskSpawnAutoReclaim = func(string) error { return nil }
}
