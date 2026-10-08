package cli

import "syscall"

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

// lpStatfsUnit is the block unit in which macOS counts Bavail: f_bsize.
func lpStatfsUnit(st *syscall.Statfs_t) uint64 { return uint64(st.Bsize) }
