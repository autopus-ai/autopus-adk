//go:build !darwin && !linux

package diskspace

import "io/fs"

// statOf has no identity, owner, or block count here; the size stands in for
// the allocation, and a walk cannot tell a mount point apart.
func statOf(info fs.FileInfo) fileStat {
	return fileStat{alloc: info.Size(), uid: -1}
}
