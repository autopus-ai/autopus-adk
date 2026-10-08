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

// RemoveFile deletes the file name below <lp> (an empty directory too).
func (d *LocalPatchDir) RemoveFile(name string) error { return d.root.Remove(name) }

// RemoveEmptyKeyDir removes <key>/ when it is an empty directory, so a
// removed worktree takes no retention slot; anything else stays.
func (d *LocalPatchDir) RemoveEmptyKeyDir(key string) {
	if info, err := d.root.Lstat(key); err == nil && info.IsDir() {
		_ = d.root.Remove(key) // rmdir: a directory that holds anything stays
	}
}

// The live flow's writes below <lp> (REQ-14): every file is new, mode 0600,
// and created through O_CREAT, O_EXCL, and O_NOFOLLOW below the os.Root, so
// no write follows a link or replaces an entry that appeared meanwhile.

// MakeKeyDir creates <key>/ with mode 0700; anything already there fails.
func (d *LocalPatchDir) MakeKeyDir(key string) error {
	if !localPatchKeyPattern.MatchString(key) {
		return errLocalPatchKey
	}
	return d.root.Mkdir(key, 0o700)
}

// CreateFile writes data to the new file name below <lp> and syncs it; a
// partial file is removed.
func (d *LocalPatchDir) CreateFile(name string, data []byte) error {
	file, err := d.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|noFollowNonBlock, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		_ = d.root.Remove(name)
	}
	return err
}

// RenameFile moves the file from to the name to, both below <lp>.
func (d *LocalPatchDir) RenameFile(from, to string) error { return d.root.Rename(from, to) }

// Exists reports an entry of any type at name below <lp> without following
// a link; a fault that cannot prove absence is an error.
func (d *LocalPatchDir) Exists(name string) (bool, error) {
	_, err := d.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
