//go:build !unix

package healthband

// noFollowNonBlock is empty where O_NOFOLLOW and O_NONBLOCK do not exist
// (Windows); the Lstat and SameFile checks around the open still apply.
const noFollowNonBlock = 0

func isSymlinkLoop(error) bool { return false }
