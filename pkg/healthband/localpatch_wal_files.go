package healthband

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// File access below <lp> for the Cleanup Rules. Every name is relative to
// <lp> and reached through its os.Root; no symlink is followed.

// fileHash returns the SHA-256 of the regular file name; found is false when
// nothing is there. Anything but a regular file is an error.
func (d *LocalPatchDir) fileHash(name string) (sum string, found bool, err error) {
	info, err := d.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", true, err
	}
	if !info.Mode().IsRegular() {
		return "", true, errUnsafeStorePath
	}
	file, err := d.root.OpenFile(name, os.O_RDONLY|noFollowNonBlock, 0)
	if err != nil {
		return "", true, err
	}
	defer file.Close()
	if opened, err := file.Stat(); err != nil || !os.SameFile(opened, info) {
		return "", true, errors.Join(errUnsafeStorePath, err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", true, err
	}
	return hex.EncodeToString(hash.Sum(nil)), true, nil
}

// worktreeContents reports whether <key>/worktree is absent and whether it
// holds only its .git file (a worktree added without checkout).
func (d *LocalPatchDir) worktreeContents(key string) (absent, onlyGitFile bool, err error) {
	name := filepath.Join(key, "worktree")
	info, err := d.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return true, false, nil
	}
	if err != nil || !info.IsDir() {
		return false, false, errors.Join(errUnsafeStorePath, err)
	}
	entries, err := d.readDir(name)
	if err != nil {
		return false, false, err
	}
	return false, len(entries) == 1 && entries[0].Name() == ".git" && entries[0].Type().IsRegular(), nil
}

func (d *LocalPatchDir) readDir(name string) ([]os.DirEntry, error) {
	dir, err := d.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadDir(-1)
}

// gitlinkClear reports that a gitlink path of the claim's worktree is absent
// or an empty directory, checked with Lstat at every component so no link
// is followed (Cleanup Rule 3, rev 11).
func (d *LocalPatchDir) gitlinkClear(key, path string) bool {
	name := filepath.Join(key, "worktree")
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
		name = filepath.Join(name, part)
		info, err := d.root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		if err != nil || !info.IsDir() {
			return false
		}
		if i == len(parts)-1 {
			entries, err := d.readDir(name)
			return err == nil && len(entries) == 0
		}
	}
	return false
}

// removeFile deletes the file name below <lp>.
func (d *LocalPatchDir) removeFile(name string) error { return d.root.Remove(name) }

// removeEmptyKeyDir removes <key>/ when it is an empty directory, so a
// removed worktree takes no retention slot; anything else stays.
func (d *LocalPatchDir) removeEmptyKeyDir(key string) {
	if info, err := d.root.Lstat(key); err == nil && info.IsDir() {
		_ = d.root.Remove(key) // rmdir: a directory that holds anything stays
	}
}
