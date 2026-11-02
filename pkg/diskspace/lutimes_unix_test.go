//go:build darwin || linux

package diskspace

import (
	"time"

	"golang.org/x/sys/unix"
)

// lutimes sets a symlink's own times without following it.
func lutimes(path string, at time.Time) error {
	tv := unix.NsecToTimeval(at.UnixNano())
	return unix.Lutimes(path, []unix.Timeval{tv, tv})
}
