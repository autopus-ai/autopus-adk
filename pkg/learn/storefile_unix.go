//go:build unix

package learn

import "syscall"

// noFollow makes a store open fail on a symlink and return at once on a FIFO,
// whose type the descriptor check then refuses.
const noFollow = syscall.O_NOFOLLOW | syscall.O_NONBLOCK
