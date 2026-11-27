package harnessfb

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// The loop writes from a background process nobody watches, so every write
// goes through an os.Root and refuses a symlink or non-directory on its
// path: a link committed in place of a directory or file must not send the
// write outside the project.
var errUnsafePath = errors.New("refusing a symlink or non-directory on the write path")

func openRoot(root string) (*os.Root, error) { return os.OpenRoot(root) }

// ensureDirs creates each component of dir inside r, refusing one that is
// not a real directory.
func ensureDirs(r *os.Root, dir string) error {
	cur := ""
	for _, part := range strings.Split(dir, "/") {
		if part == "" || part == "." {
			continue
		}
		cur = path.Join(cur, part)
		info, err := r.Lstat(cur)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := r.Mkdir(cur, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
		case err != nil:
			return err
		case !info.IsDir():
			return fmt.Errorf("%w: %s", errUnsafePath, cur)
		}
	}
	return nil
}

// checkTarget refuses a target that exists and is not a regular file.
func checkTarget(r *os.Root, rel string) error {
	info, err := r.Lstat(rel)
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", errUnsafePath, rel)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// writeFile replaces rel inside root through a temporary file and a
// rename, so a reader never sees half of it.
func writeFile(root, rel string, data []byte) error {
	r, err := openRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := ensureDirs(r, path.Dir(rel)); err != nil {
		return err
	}
	if err := checkTarget(r, rel); err != nil {
		return err
	}
	tmp := path.Join(path.Dir(rel), "."+path.Base(rel)+"."+strconv.FormatInt(time.Now().UnixNano(), 36)+".tmp")
	f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	if err := errors.Join(werr, f.Close()); err != nil {
		_ = r.Remove(tmp)
		return err
	}
	if err := r.Rename(tmp, rel); err != nil {
		_ = r.Remove(tmp)
		return err
	}
	return nil
}

// appendFile appends data to rel inside root with the same checks.
func appendFile(root, rel string, data []byte) error {
	r, err := openRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := ensureDirs(r, path.Dir(rel)); err != nil {
		return err
	}
	if err := checkTarget(r, rel); err != nil {
		return err
	}
	f, err := r.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Close())
}

// readFile reads rel inside root, refusing anything but a regular file.
func readFile(root, rel string) ([]byte, error) {
	r, err := openRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	info, err := r.Lstat(rel)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", errUnsafePath, rel)
	}
	return r.ReadFile(rel)
}

// listFiles names the regular files in rel with suffix.
func listFiles(root, rel, suffix string) ([]string, error) {
	r, err := openRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	entries, err := fs.ReadDir(r.FS(), rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), suffix) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// removeFile removes rel inside root; a missing file is not an error.
func removeFile(root, rel string) error {
	r, err := openRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := checkTarget(r, rel); err != nil {
		return err
	}
	if err := r.Remove(rel); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// prepareDir makes rel's directories inside root safely, for a lock file
// that filelock then opens by absolute path.
func prepareDir(root, rel string) error {
	r, err := openRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	return ensureDirs(r, rel)
}
