//go:build !unix

package learn

// noFollow is empty where O_NOFOLLOW and O_NONBLOCK do not exist (Windows);
// the Lstat and descriptor checks around the open still apply.
const noFollow = 0
