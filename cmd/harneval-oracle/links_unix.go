//go:build unix

package main

import (
	"os"
	"syscall"
)

// openFlags never follows a final symlink and never blocks on a FIFO.
const openFlags = os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK

// linkCount is the hard link count of an opened file.
func linkCount(info os.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Nlink), true
}
