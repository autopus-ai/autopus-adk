//go:build darwin || linux

package diskspace

import "golang.org/x/sys/unix"

// leaseNoFollow makes opening a shared lease lock fail on a planted symlink.
const leaseNoFollow = unix.O_NOFOLLOW
