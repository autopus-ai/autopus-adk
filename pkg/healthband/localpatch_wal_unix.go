//go:build unix

package healthband

import (
	"os"
	"syscall"
)

// privateDir reports a directory of the current user without group or other
// permission bits (REQ-14).
func privateDir(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && info.Mode().Perm()&0o077 == 0
}
