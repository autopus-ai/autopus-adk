package editguard

import (
	"path/filepath"
	"slices"
)

// resolveWrites returns the targets of every location a host may write raw
// to, each location once and the kernel walk of ResolveAll first. Hosts that
// clean `..` lexically before the kernel follows any symlink (Gemini CLI's
// path.resolve, OpenCode's path.join) write to another file than the kernel
// walk reaches when a `..` follows a symlink, so a path with a `..` also gets
// the walk of its lexical clean, and a deny of either location denies the
// call. A location that cannot be walked drops out while the other decides.
func resolveWrites(cwd, raw string) ([]Target, error) {
	abs, err := checkedAbs(cwd, raw)
	if err != nil {
		return nil, err
	}
	targets, err := locate(abs)
	if !hasDotDot(abs) {
		return targets, err
	}
	lexical, lexicalErr := locate(filepath.Clean(abs))
	switch {
	case lexicalErr != nil:
		return targets, err
	case err != nil:
		return lexical, nil
	}
	for _, target := range lexical {
		if !slices.ContainsFunc(targets, func(known Target) bool {
			return known.Root == target.Root && known.Key == target.Key
		}) {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

// hasDotDot reports a `..` component, the only one whose lexical clean and
// kernel walk can disagree.
func hasDotDot(abs string) bool {
	return slices.Contains(splitComponents(abs[len(filepath.VolumeName(abs)):]), "..")
}
