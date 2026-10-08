//go:build !windows

package intake

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
)

// writeSupported reports whether intake-area writes run on this platform.
func writeSupported() error { return nil }

// ensureDir creates every missing component of rel as a real directory
// through the Root, fsyncing the parent of each new one; an existing
// component must already be a real directory.
func (a *area) ensureDir(rel string) error {
	if !isCleanRel(rel) {
		return unsafef("%q is not a clean relative path", rel)
	}
	parts := strings.Split(rel, "/")
	for index := range parts {
		name := strings.Join(parts[:index+1], "/")
		info, err := a.root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			if err = a.root.Mkdir(name, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
			if err = a.syncDir(path.Dir(name)); err != nil {
				return err
			}
			info, err = a.root.Lstat(name)
		}
		if err != nil {
			return err
		}
		if info.Mode().Type() != fs.ModeDir {
			return unsafef("%s is not a real directory", name)
		}
	}
	return nil
}

// createExclusive publishes data at rel, which must not exist yet. The bytes
// go to a same-directory temp file .<name>.tmp-<16 hex> created with O_EXCL
// and fsynced; Root.Link then creates rel, failing with fs.ErrExist when rel
// already exists, so a reader never sees a partial file. The directory is
// fsynced after the link and again after the temp file is removed. Stale temp
// files of the same name are removed first.
func (a *area) createExclusive(rel string, data []byte) error {
	dir, name := path.Dir(rel), path.Base(rel)
	if _, err := a.lstatChain(dir, true); err != nil {
		return err
	}
	if err := a.removeStaleTemps(dir, name); err != nil {
		return err
	}
	temp, err := a.writeTemp(dir, name, data)
	if err != nil {
		return err
	}
	linkErr := a.root.Link(temp, rel)
	if linkErr == nil {
		linkErr = a.syncDir(dir)
	}
	removeErr := a.root.Remove(temp)
	if removeErr == nil {
		removeErr = a.syncDir(dir)
	}
	if linkErr != nil {
		return linkErr
	}
	return removeErr
}

// writeTemp writes data to a new fsynced temp file next to name.
func (a *area) writeTemp(dir, name string, data []byte) (string, error) {
	var suffix [8]byte
	// crypto/rand.Read never returns an error and always fills the buffer.
	_, _ = rand.Read(suffix[:])
	temp := path.Join(dir, "."+name+".tmp-"+hex.EncodeToString(suffix[:]))
	file, err := a.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = a.root.Remove(temp)
		return "", err
	}
	return temp, nil
}

// removeStaleTemps removes regular temp files an interrupted publication of
// name left in dir.
func (a *area) removeStaleTemps(dir, name string) error {
	entries, err := a.listDir(dir)
	if err != nil {
		return err
	}
	prefix := "." + name + ".tmp-"
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) || !entry.Type().IsRegular() {
			continue
		}
		if err := a.root.Remove(path.Join(dir, entry.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// removeFile deletes the regular file rel and fsyncs its directory.
func (a *area) removeFile(rel string) error {
	if _, err := a.lstatChain(rel, false); err != nil {
		return err
	}
	if err := a.root.Remove(rel); err != nil {
		return err
	}
	return a.syncDir(path.Dir(rel))
}

// syncDir fsyncs the directory rel so a created or removed entry is durable.
func (a *area) syncDir(rel string) error {
	dir, err := a.root.Open(rel)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
