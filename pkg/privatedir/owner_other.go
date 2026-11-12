//go:build !unix

package privatedir

import "io/fs"

// noFollow is empty where O_NOFOLLOW does not exist (Windows); the Lstat and
// descriptor checks around the open still apply.
const noFollow = 0

// ownedByCurrentUser cannot read an owner uid here; the per-user cache
// directory is the isolation.
func ownedByCurrentUser(fs.FileInfo) bool { return true }
