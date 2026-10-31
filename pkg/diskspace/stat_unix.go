//go:build darwin || linux

package diskspace

import (
	"io/fs"
	"syscall"
)

// statOf reads the identity, owner, and allocated bytes of an Lstat result.
// Allocated bytes are st_blocks*512, the space a file occupies on disk and
// what deleting it gives back.
func statOf(info fs.FileInfo) fileStat {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileStat{alloc: info.Size(), uid: -1}
	}
	return fileStat{
		id:    FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)},
		uid:   int(st.Uid),
		alloc: int64(st.Blocks) * 512,
		known: true,
	}
}
