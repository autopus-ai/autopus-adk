package learn

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// errStoreNotRegular reports a store path that is a symlink or another
// non-regular file.
var errStoreNotRegular = errors.New("learning store is not a regular file")

// openStoreFile opens the store for writing with flag only when the path is a
// regular file or missing. A committed symlink (pipeline.jsonl -> ../../.env)
// would otherwise let an append or a rewrite write through it. noFollow makes
// the open refuse a symlink swapped in after the check, where the platform
// has O_NOFOLLOW, and the opened descriptor must still be a regular file.
func openStoreFile(path string, flag int) (*os.File, error) {
	info, err := os.Lstat(path)
	switch {
	case err == nil && !info.Mode().IsRegular():
		return nil, fmt.Errorf("%s: %w", path, errStoreNotRegular)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	f, err := os.OpenFile(path, flag|noFollow, 0o644)
	if err != nil {
		return nil, err
	}
	if info, err = f.Stat(); err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		if err == nil {
			err = errStoreNotRegular
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}
