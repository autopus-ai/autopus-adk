package editguard

import (
	"fmt"
	"os"
	"path/filepath"
)

// storeTarget resolves raw relative to the store root, whatever autopus.yaml
// lies between them, so a nested root planted after a lock cannot detach the
// lock from its path (M1). nested is the store-relative nearest root of raw
// when that root is a project nested in the store's; it is "" otherwise.
func (s *Store) storeTarget(raw string) (target Target, nested string, ok bool) {
	targets, err := ResolveAll(s.cwd, raw)
	if err != nil {
		return Target{}, "", false
	}
	for _, candidate := range targets {
		if !sameDir(candidate.Root, s.root) {
			continue
		}
		if !sameDir(targets[0].Root, s.root) {
			if rel, err := filepath.Rel(candidate.Root, targets[0].Root); err == nil {
				nested = filepath.ToSlash(rel)
			}
		}
		return candidate, nested, true
	}
	return Target{}, "", false
}

// sameDir reports two spellings of one directory, so a root spelled in
// another case on a case-insensitive volume is still the store root.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ai, aErr := os.Stat(a)
	bi, bErr := os.Stat(b)
	return aErr == nil && bErr == nil && os.SameFile(ai, bi)
}

// nestedHint tells where to run a command for a path of a nested project.
func nestedHint(nested, command string) string {
	return fmt.Sprintf(", which is in the nested project %s (the nearest directory with %s); run %s from there",
		displayPath(nested), projectMarker, command)
}
