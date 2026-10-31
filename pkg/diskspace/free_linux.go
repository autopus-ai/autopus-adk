package diskspace

import (
	"os"
	"path/filepath"
	"syscall"
)

// Free measures the file system that holds path as df does. Linux counts
// Bavail in f_frsize blocks, or f_bsize when f_frsize is 0 (statfs(2)); the
// mount is the longest mount point in /proc/self/mounts that holds the
// canonical path. The arithmetic mirrors lpStatfs of the local-patch flow.
func Free(path string) (Space, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Space{}, err
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(abs, &st); err != nil {
		return Space{}, err
	}
	mount := "/"
	if canonical, err := filepath.EvalSymlinks(abs); err == nil {
		if table, err := os.ReadFile("/proc/self/mounts"); err == nil {
			mount = longestMount(table, canonical)
		}
	}
	return statfsSpace(&st, mount), nil
}

// statfsSpace converts a linux statfs reading.
func statfsSpace(st *syscall.Statfs_t, mount string) Space {
	unit := linuxBlockUnit(int64(st.Frsize), int64(st.Bsize))
	return Space{AvailBytes: availBytes(st.Bavail, unit), BlockUnit: unit, Mount: mount}
}
