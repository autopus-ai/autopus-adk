//go:build unix

package privatedir

import (
	"io/fs"
	"os"
	"syscall"
)

// noFollow makes an open fail on a symlink; O_NONBLOCK keeps a planted FIFO
// from blocking the open.
const noFollow = syscall.O_NOFOLLOW | syscall.O_NONBLOCK

func ownedByCurrentUser(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}
