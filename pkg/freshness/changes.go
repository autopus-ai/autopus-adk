package freshness

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// maxChangedManifests bounds the work one stop does on a sweeping change.
const maxChangedManifests = 200

// Change is a pin the work tree adds or moves relative to HEAD. Previous is
// the spec at HEAD, "" for a new pin.
type Change struct {
	Item     Item   `json:"item"`
	Previous string `json:"previous,omitempty"`
}

// Changed lists the pins that the work tree adds or changes against HEAD,
// in tracked manifests and in untracked ones that are not ignored. Pins that
// are the same as at HEAD are never returned, wherever their line moved.
// ok is false when dir is not in a git work tree.
func Changed(ctx context.Context, dir string) (root string, changes []Change, ok bool) {
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, false
	}
	root = strings.TrimSpace(string(top))
	_, headErr := git(ctx, root, "rev-parse", "--verify", "-q", "HEAD")
	hasHead := headErr == nil
	var paths []string
	if hasHead {
		out, err := git(ctx, root, "-c", "core.quotePath=false", "diff", "--name-only", "--no-renames", "-z", "HEAD")
		if err != nil {
			return root, nil, false
		}
		paths = splitZ(out)
	} else if out, err := git(ctx, root, "ls-files", "-z"); err == nil {
		paths = splitZ(out)
	}
	if out, err := git(ctx, root, "ls-files", "--others", "--exclude-standard", "-z"); err == nil {
		paths = append(paths, splitZ(out)...)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	n := 0
	for _, rel := range paths {
		if !IsManifest(rel) || skippedPath(rel) {
			continue
		}
		if n++; n > maxChangedManifests {
			break
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue // deleted
		}
		previous := map[string]string{}
		if hasHead {
			if old, err := git(ctx, root, "show", "HEAD:"+rel); err == nil {
				for _, it := range ParseFile(rel, old) {
					previous[it.Key()] = it.Spec
				}
			}
		}
		for _, it := range withRegistry(root, ParseFile(rel, data)) {
			if spec, had := previous[it.Key()]; !had || spec != it.Spec {
				changes = append(changes, Change{Item: it, Previous: spec})
			}
		}
	}
	return root, changes, true
}

// skippedPath reports a path inside a directory Scan never walks: vendored
// dependencies, build output, fixtures, harness state.
func skippedPath(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, p := range parts[:len(parts)-1] {
		if slices.Contains(skipDirs, p) {
			return true
		}
	}
	return false
}

func splitZ(out []byte) []string {
	var paths []string
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}
	return paths
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd.Output()
}
