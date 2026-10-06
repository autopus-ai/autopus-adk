// Package editguard decides whether a file-editing tool call may proceed and
// keeps the `/auto fix` reproduction-test locks (SPEC-EDITGUARD-001).
//
// Every decision is fail-open: a fault never makes a decision stricter than the
// guard without the faulty part.
package editguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// projectMarker is the file whose directory is a project root.
const projectMarker = "autopus.yaml"

// maxSymlinkHops bounds how many dangling links resolution follows by hand.
const maxSymlinkHops = 40

var (
	// ErrNoProjectRoot reports a path with no autopus.yaml in any ancestor.
	ErrNoProjectRoot = errors.New("editguard: no project root")
	// ErrUnresolvable reports a path that cannot be resolved to a location.
	ErrUnresolvable = errors.New("editguard: path cannot be resolved")
)

// Target is one path resolved against its nearest project root (REQ-EG-05).
type Target struct {
	// Root is the symlink-free absolute directory that holds autopus.yaml.
	Root string
	// Rel is the slash-separated path below Root, as resolved.
	Rel string
	// Key is Rel folded to lower case when Root's volume is case-insensitive,
	// and Rel otherwise. Every comparison uses Key.
	Key string
	// CaseInsensitive reports the detected case behavior of Root's volume.
	CaseInsensitive bool
}

// Abs returns the target's absolute path.
func (t Target) Abs() string {
	return filepath.Join(t.Root, filepath.FromSlash(t.Rel))
}

// Resolve resolves raw against cwd (the process working directory when cwd is
// empty), cleans `.` and `..` segments and duplicate separators, resolves the
// symlinks of the deepest existing ancestor, and returns the path relative to
// the nearest ancestor directory that contains autopus.yaml.
func Resolve(cwd, raw string) (Target, error) {
	if raw == "" || strings.ContainsRune(raw, 0) || strings.ContainsRune(cwd, 0) {
		return Target{}, ErrUnresolvable
	}
	abs, err := absolutePath(cwd, raw)
	if err != nil {
		return Target{}, ErrUnresolvable
	}
	resolved, err := resolveExisting(abs)
	if err != nil {
		return Target{}, ErrUnresolvable
	}
	root, err := findRoot(filepath.Dir(resolved))
	if err != nil {
		return Target{}, err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Target{}, ErrUnresolvable
	}
	rel = filepath.ToSlash(rel)
	fold := volumeIsCaseInsensitive(root)
	return Target{Root: root, Rel: rel, Key: FoldKey(rel, fold), CaseInsensitive: fold}, nil
}

// FindProjectRoot returns the nearest directory at or above dir that contains
// autopus.yaml, with dir's symlinks resolved first.
func FindProjectRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", ErrNoProjectRoot
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", ErrNoProjectRoot
	}
	return findRoot(resolved)
}

// FoldKey returns the comparison key of a project-relative path.
func FoldKey(rel string, caseInsensitive bool) string {
	if caseInsensitive {
		return strings.ToLower(rel)
	}
	return rel
}

func absolutePath(cwd, raw string) (string, error) {
	p := filepath.FromSlash(raw)
	if !filepath.IsAbs(p) {
		base := cwd
		if base == "" {
			wd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			base = wd
		}
		p = filepath.Join(base, p)
	}
	return filepath.Abs(p)
}

// resolveExisting resolves the symlinks of the deepest existing ancestor of abs
// and keeps the non-existent tail. A dangling link at that ancestor is followed
// by hand, because a write through it lands at the link target.
func resolveExisting(abs string) (string, error) {
	for hop := 0; hop <= maxSymlinkHops; hop++ {
		existing, tail := deepestExisting(abs)
		resolved, err := filepath.EvalSymlinks(existing)
		if err == nil {
			return filepath.Join(resolved, tail), nil
		}
		link, linkErr := os.Readlink(existing)
		if linkErr != nil {
			return "", err
		}
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(existing))
		if parentErr != nil {
			return "", parentErr
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(parent, link)
		}
		abs = filepath.Join(filepath.Clean(link), tail)
	}
	return "", ErrUnresolvable
}

func deepestExisting(abs string) (existing, tail string) {
	existing = abs
	for {
		if _, err := os.Lstat(existing); err == nil {
			return existing, tail
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return existing, tail
		}
		tail = filepath.Join(filepath.Base(existing), tail)
		existing = parent
	}
}

func findRoot(dir string) (string, error) {
	for current := dir; ; {
		info, err := os.Stat(filepath.Join(current, projectMarker))
		if err == nil && info.Mode().IsRegular() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", ErrNoProjectRoot
		}
		current = parent
	}
}

// volumeIsCaseInsensitive probes root's volume by naming its own marker file in
// another case. Only a lookup that reaches the very same file counts, so a
// second, differently cased file on a case-sensitive volume does not.
func volumeIsCaseInsensitive(root string) bool {
	exact, err := os.Stat(filepath.Join(root, projectMarker))
	if err != nil {
		return false
	}
	folded, err := os.Stat(filepath.Join(root, strings.ToUpper(projectMarker)))
	return err == nil && os.SameFile(exact, folded)
}
