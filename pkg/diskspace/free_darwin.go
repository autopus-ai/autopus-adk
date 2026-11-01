package diskspace

import (
	"path/filepath"
	"syscall"
)

// Free measures the file system that holds path as df does. macOS counts
// Bavail in f_bsize blocks, and f_mntonname names the mount, which for a path
// under the firmlinked /Users is the Data volume, /System/Volumes/Data. The
// arithmetic mirrors lpStatfs of the local-patch flow in internal/cli.
func Free(path string) (Space, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Space{}, err
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(abs, &st); err != nil {
		return Space{}, err
	}
	return statfsSpace(&st), nil
}

// statfsSpace converts a darwin statfs reading.
func statfsSpace(st *syscall.Statfs_t) Space {
	unit := int64(st.Bsize)
	return Space{AvailBytes: availBytes(st.Bavail, unit), BlockUnit: unit, Mount: cString(st.Mntonname[:])}
}

// cString reads a NUL-terminated C char array.
func cString(raw []int8) string {
	buf := make([]byte, 0, len(raw))
	for _, c := range raw {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return string(buf)
}
