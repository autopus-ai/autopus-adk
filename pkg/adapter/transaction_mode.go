package adapter

import "os"

// transactionWriteMode returns the permission bits a transaction write gives
// its file. A new file takes the requested mode, 0644 when unset. A rewrite
// keeps the mode the file already has, so a private file (an opencode.json or
// settings file at 0600) is never widened to a generated default; it gains
// only the execute bits the write requests where the file already grants
// read, so a generated script stays runnable. existing is the Lstat result of
// the target, nil when the target does not exist.
func transactionWriteMode(existing os.FileInfo, requested os.FileMode) os.FileMode {
	if requested == 0 {
		requested = 0o644
	}
	if existing == nil || !existing.Mode().IsRegular() {
		return requested
	}
	current := existing.Mode().Perm()
	readable := (current & 0o444) >> 2 // the exec bit beside each read bit
	return current | (requested & 0o111 & readable)
}
