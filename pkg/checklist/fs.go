package checklist

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

// Project-relative locations. Hits live under the runtime directory because
// they are machine state, not reviewed content.
const (
	ChecksDir     = ".autopus/checklist"
	CandidatesDir = ".autopus/checklist/candidates"
	HitsFile      = ".autopus/runtime/checklist-hits.jsonl"
)

var errUnsafePath = errors.New("refusing a symlink or non-directory on the write path")

// writeFile writes data at rel inside root. Every component is checked with
// Lstat through an os.Root, so a symlink a change committed in place of a
// directory or the file is refused instead of followed.
func writeFile(root, rel string, data []byte) error {
	return withSafeFile(root, rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	})
}

// appendFile appends data to rel inside root with the same checks.
func appendFile(root, rel string, data []byte) error {
	return withSafeFile(root, rel, os.O_WRONLY|os.O_CREATE|os.O_APPEND, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	})
}

func withSafeFile(root, rel string, flag int, use func(*os.File) error) error {
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := ensureDirs(r, path.Dir(rel)); err != nil {
		return err
	}
	if info, err := r.Lstat(rel); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", errUnsafePath, rel)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := r.OpenFile(rel, flag, 0o644)
	if err != nil {
		return err
	}
	if err := use(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

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

// readFile reads rel inside root, refusing a symlink at the file itself.
func readFile(root, rel string) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	info, err := r.Lstat(rel)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", errUnsafePath, rel)
	}
	return r.ReadFile(rel)
}

// listDir names the regular files in rel with the given suffix, sorted.
func listDir(root, rel, suffix string) ([]string, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
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
