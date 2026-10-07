//go:build unix

package healthband

import (
	"errors"
	"syscall"
)

// noFollowNonBlock makes an open fail on a symlink and return at once on a
// FIFO, whose type the descriptor check then refuses.
const noFollowNonBlock = syscall.O_NOFOLLOW | syscall.O_NONBLOCK

// isSymlinkLoop reports the error O_NOFOLLOW gives for a symlink.
func isSymlinkLoop(err error) bool { return errors.Is(err, syscall.ELOOP) }
