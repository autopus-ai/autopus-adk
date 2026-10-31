// Package diskspace reads the disk budget of a shared development machine
// (SPEC-DISK-001): free space as df reports it, the consumers that fill the
// volume (the Go build cache, go build work directories, git worktrees, and
// agent scratchpads), and the contained go-build trim, planned as a pure
// function of those readings and applied only through os.Root.
package diskspace

import (
	"errors"
	"fmt"
)

// Space is the free space of the file system that holds a path.
type Space struct {
	// AvailBytes is statfs Bavail times BlockUnit: the space an unprivileged
	// process can still allocate, the Available column of df.
	AvailBytes int64
	// BlockUnit is the unit Bavail counts in.
	BlockUnit int64
	// Mount is the mount point of the file system.
	Mount string
}

// ErrUnsupported marks an operating system without a free-space reading.
var ErrUnsupported = errors.New("free space measurement is not supported")

// UnsupportedOn is the ErrUnsupported error Free returns on goos.
func UnsupportedOn(goos string) error {
	return fmt.Errorf("%w on %s", ErrUnsupported, goos)
}

// availBytes is the statfs reading df -k derives its Available column from:
// Bavail blocks of unit bytes.
func availBytes(bavail uint64, unit int64) int64 {
	return int64(bavail) * unit
}

// linuxBlockUnit is the unit Linux counts Bavail in: f_frsize, or f_bsize
// when f_frsize is 0 (statfs(2)). It is OS-neutral so a darwin host tests it.
func linuxBlockUnit(frsize, bsize int64) int64 {
	if frsize > 0 {
		return frsize
	}
	return bsize
}
