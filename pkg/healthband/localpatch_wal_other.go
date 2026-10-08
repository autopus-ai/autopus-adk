//go:build !unix

package healthband

import "os"

// privateDir accepts every directory where POSIX ownership and permission
// bits do not exist (Windows); the os.Root walk still refuses symlinks.
func privateDir(info os.FileInfo) bool { return info.IsDir() }
