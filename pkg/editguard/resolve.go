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
	"runtime"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// projectMarker is the file whose directory is a project root.
const projectMarker = "autopus.yaml"

// maxSymlinkHops bounds how many symlinks one resolution follows.
const maxSymlinkHops = 40

var (
	// ErrNoProjectRoot reports a path with no autopus.yaml in any ancestor.
	ErrNoProjectRoot = errors.New("editguard: no project root")
	// ErrUnresolvable reports a path that cannot be resolved to a location.
	ErrUnresolvable = errors.New("editguard: path cannot be resolved")
)

// caseFolder is Unicode full case folding; the Caser is stateless and safe for
// concurrent use.
var caseFolder = cases.Fold()

// Target is one path resolved against a project root (REQ-EG-05).
type Target struct {
	// Root is the symlink-free absolute directory that holds autopus.yaml.
	Root string
	// Rel is the slash-separated path below Root, as resolved.
	Rel string
	// Key is FoldKey of Rel for Root's volume. Every comparison uses Key.
	Key string
	// CaseInsensitive reports the detected case behavior of Root's volume.
	CaseInsensitive bool
}

// Abs returns the target's absolute path.
func (t Target) Abs() string {
	return filepath.Join(t.Root, filepath.FromSlash(t.Rel))
}

// Resolve resolves raw against cwd (the process working directory when cwd is
// empty) the way the kernel walks it, and returns the path relative to the
// nearest ancestor directory that contains autopus.yaml.
func Resolve(cwd, raw string) (Target, error) {
	targets, err := ResolveAll(cwd, raw)
	if err != nil {
		return Target{}, err
	}
	return targets[0], nil
}

// ResolveAll resolves raw like Resolve and returns it relative to every
// enclosing project root, nearest first. A nested autopus.yaml, which any edit
// can create, must not hide what an outer root protects (M1).
func ResolveAll(cwd, raw string) ([]Target, error) {
	abs, err := checkedAbs(cwd, raw)
	if err != nil {
		return nil, err
	}
	return locate(abs)
}

// checkedAbs is absolutePath for a usable raw and cwd.
func checkedAbs(cwd, raw string) (string, error) {
	if raw == "" || strings.ContainsRune(raw, 0) || strings.ContainsRune(cwd, 0) {
		return "", ErrUnresolvable
	}
	abs, err := absolutePath(cwd, raw)
	if err != nil {
		return "", ErrUnresolvable
	}
	return abs, nil
}

// locate walks abs the way the kernel does and returns it relative to every
// enclosing project root, nearest first.
func locate(abs string) ([]Target, error) {
	resolved, err := resolveComponents(abs)
	if err != nil {
		return nil, ErrUnresolvable
	}
	var targets []Target
	for _, root := range enclosingRoots(filepath.Dir(resolved)) {
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, ErrUnresolvable
		}
		rel = filepath.ToSlash(rel)
		fold := volumeIsCaseInsensitive(root)
		targets = append(targets, Target{Root: root, Rel: rel, Key: FoldKey(rel, fold), CaseInsensitive: fold})
	}
	if len(targets) == 0 {
		return nil, ErrNoProjectRoot
	}
	return targets, nil
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
	if roots := enclosingRoots(resolved); len(roots) > 0 {
		return roots[0], nil
	}
	return "", ErrNoProjectRoot
}

// FoldKey returns the comparison key of a project-relative path. On a
// case-insensitive volume it is the canonical caseless form NFD(fold(NFD(p))),
// which names two spellings alike where APFS opens one file for both: case,
// full case folding (U+00DF and "ss", U+017F and "s"), canonical equivalence
// (NFC and NFD), and the Cherokee case pairs (oneCherokeeCase). Invalid UTF-8
// passes through unchanged.
func FoldKey(rel string, caseInsensitive bool) string {
	switch {
	case !caseInsensitive:
		return rel
	case isASCII(rel):
		return strings.ToLower(rel)
	}
	return norm.NFD.String(strings.Map(oneCherokeeCase, caseFolder.String(norm.NFD.String(rel))))
}

// oneCherokeeCase gives both letters of a Cherokee case pair the uppercase
// form Unicode case folding maps them to. The x/text fold instead swaps the
// case of each of the 86 pairs, which would leave two keys for one name.
func oneCherokeeCase(r rune) rune {
	if unicode.Is(unicode.Cherokee, r) {
		return unicode.ToUpper(r)
	}
	return r
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// absolutePath joins a relative raw onto cwd, or onto the working directory
// for an empty or relative cwd, without cleaning: a `..` climbs from wherever
// the symlinks before it lead, which only the component walk knows (L3).
// Windows applies `..` lexically before it follows a link, so it is cleaned.
func absolutePath(cwd, raw string) (string, error) {
	p := filepath.FromSlash(raw)
	if !filepath.IsAbs(p) {
		base := filepath.FromSlash(cwd)
		if !filepath.IsAbs(base) {
			wd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			base = joinUncleaned(wd, base)
		}
		p = joinUncleaned(base, p)
	}
	if runtime.GOOS == "windows" {
		return filepath.Clean(p), nil
	}
	return p, nil
}

func joinUncleaned(dir, name string) string {
	if name == "" {
		return dir
	}
	return dir + string(filepath.Separator) + name
}

// resolveComponents walks abs one component at a time, as the kernel does: a
// symlink is replaced by its target before the next component, and `..` leaves
// the directory reached so far. From the first missing component on, the rest
// is kept lexically, because no write passes through a missing directory.
func resolveComponents(abs string) (string, error) {
	volume := filepath.VolumeName(abs)
	dest := volume + string(filepath.Separator)
	rest := splitComponents(abs[len(volume):])
	for hops := 0; len(rest) > 0; {
		name := rest[0]
		rest = rest[1:]
		switch name {
		case ".":
			continue
		case "..":
			dest = filepath.Dir(dest)
			continue
		}
		next := filepath.Join(dest, name)
		info, err := os.Lstat(next)
		if err != nil {
			tail := filepath.Join(append([]string{name}, rest...)...)
			if first, _, _ := strings.Cut(tail, string(filepath.Separator)); first != name {
				rest = splitComponents(tail) // `..` climbed back out of the missing component
				continue
			}
			return filepath.Join(dest, tail), nil
		}
		if info.Mode()&os.ModeSymlink == 0 {
			dest = next
			continue
		}
		if hops++; hops > maxSymlinkHops {
			return "", ErrUnresolvable
		}
		link, err := os.Readlink(next)
		if err != nil {
			return "", err
		}
		if filepath.IsAbs(link) {
			volume = filepath.VolumeName(link)
			dest, link = volume+string(filepath.Separator), link[len(volume):]
		}
		rest = append(splitComponents(link), rest...)
	}
	return dest, nil
}

// splitComponents splits p at its separators, dropping empty components. The
// separators are ASCII, so splitting bytes never cuts a UTF-8 sequence.
func splitComponents(p string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || os.IsPathSeparator(p[i]) {
			if i > start {
				parts = append(parts, p[start:i])
			}
			start = i + 1
		}
	}
	return parts
}

// enclosingRoots lists every directory at or above dir, nearest first, that
// holds a regular autopus.yaml.
func enclosingRoots(dir string) []string {
	var roots []string
	for current := dir; ; {
		if info, err := os.Stat(filepath.Join(current, projectMarker)); err == nil && info.Mode().IsRegular() {
			roots = append(roots, current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return roots
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
