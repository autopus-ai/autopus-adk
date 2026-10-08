package cli

import (
	"os"
	"syscall"
)

// lpNoFollowFlag makes an open below <lp> fail on a symlink.
const lpNoFollowFlag = syscall.O_NOFOLLOW

// lpStatfs is the free space of the file system that holds path.
func lpStatfs(path string) (lpDiskSpace, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return lpDiskSpace{}, err
	}
	return lpDiskSpace{avail: st.Bavail, unit: lpStatfsUnit(&st)}, nil
}

// lpStatfsUnit is the block unit in which Linux counts Bavail: f_frsize,
// or f_bsize when f_frsize is 0 (statfs(2)).
func lpStatfsUnit(st *syscall.Statfs_t) uint64 {
	if st.Frsize > 0 {
		return uint64(st.Frsize)
	}
	return uint64(st.Bsize)
}

// lpOwnedByCurrentUser reports that info belongs to the running user.
func lpOwnedByCurrentUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Getuid())
}
