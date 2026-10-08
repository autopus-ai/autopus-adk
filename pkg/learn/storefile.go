package learn

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// errStoreNotRegular reports a store path that is a symlink or another
// non-regular file.
var errStoreNotRegular = errors.New("learning store is not a regular file")

// errStoreDirUnsafe reports a store directory, .autopus or .autopus/learnings
// under the project, that is a symlink or not a directory.
var errStoreDirUnsafe = errors.New("learning store directory is a symlink or not a directory")

// openStoreFile opens the store with flag, for reading as for writing, only
// when .autopus and .autopus/learnings are real directories and the store is
// a regular file or missing. A committed symlink (pipeline.jsonl ->
// ../../.env, or .autopus/learnings -> elsewhere) would otherwise let a read
// or a write go through it, and a FIFO or a device (pipeline.jsonl ->
// /dev/urandom) would block a read or never end it. noFollow makes the open
// refuse a symlink swapped in after the check, where the platform has
// O_NOFOLLOW, and the opened descriptor must still be a regular file. A
// directory swapped for a link between the check and the open stays a
// residual risk of the operator's local machine.
func openStoreFile(path string, flag int) (*os.File, error) {
	learnings := filepath.Dir(path)
	for _, dir := range []string{filepath.Dir(learnings), learnings} {
		if err := realDir(dir); err != nil {
			return nil, err
		}
	}
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

// makeStoreDirs creates .autopus/learnings under dir one level at a time and
// checks each level before creating the next, so no directory is created
// through a symlink.
func makeStoreDirs(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, level := range []string{filepath.Join(dir, ".autopus"), filepath.Join(dir, ".autopus", "learnings")} {
		if err := os.Mkdir(level, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		if err := realDir(level); err != nil {
			return err
		}
	}
	return nil
}

// realDir refuses dir when it is a symlink or not a directory.
func realDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: %w", dir, errStoreDirUnsafe)
	}
	return nil
}
