package editguard

import (
	"os"
	"path/filepath"
	"strings"
)

// displacedMarker reports whether a call that removes or replaces raw (an
// apply_patch Delete File, or either end of a Move to) would remove or swap
// the autopus.yaml of a project root, which would leave that root's tree
// without the locks and manifests that protect it. It returns the marker's
// path below the outermost root that encloses it. Unlinking and renaming act
// on the directory entry itself, so the last component is not followed; the
// directory before it is walked the kernel's and, after a `..`, the lexical
// way, like every other target (resolveWrites).
func displacedMarker(cwd, raw string) (string, bool) {
	abs, err := checkedAbs(cwd, raw)
	if err != nil {
		return "", false
	}
	walks := []string{abs}
	if hasDotDot(abs) {
		walks = append(walks, filepath.Clean(abs))
	}
	for _, walk := range walks {
		cut := strings.LastIndexByte(walk, filepath.Separator)
		dir, err := resolveComponents(walk[:cut+1])
		if err != nil || !namesRootMarker(dir, walk[cut+1:]) {
			continue
		}
		roots := enclosingRoots(dir)
		if rel, err := filepath.Rel(roots[len(roots)-1], filepath.Join(dir, walk[cut+1:])); err == nil {
			return filepath.ToSlash(rel), true
		}
	}
	return "", false
}

// namesRootMarker reports whether the entry base of dir is the regular
// autopus.yaml that makes dir a project root, under whatever spelling the
// volume opens it.
func namesRootMarker(dir, base string) bool {
	if base == "" || base == "." || base == ".." {
		return false
	}
	marker := filepath.Join(dir, projectMarker)
	if info, err := os.Stat(marker); err != nil || !info.Mode().IsRegular() {
		return false
	}
	entry, err := os.Lstat(filepath.Join(dir, base))
	if err != nil {
		return false
	}
	link, err := os.Lstat(marker)
	return err == nil && os.SameFile(entry, link)
}
