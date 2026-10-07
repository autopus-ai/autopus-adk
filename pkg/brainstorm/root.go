// Package brainstorm writes the read-only diagnosis BS files of
// `auto react band` (SPEC-SIGMABAND-001 REQ-13): BS Root Resolution over the
// exported setup.DetectMultiRepo, the locked BS-BAND-NNN allocation, the
// content/skills/idea.md BS renderer, and its structural validator.
package brainstorm

import (
	"errors"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/setup"
)

// MaxScanDepth bounds the recursive component scan: DetectMultiRepo runs on
// the root and on components down to this many component edges below it
// (BS Root Resolution item 2).
const MaxScanDepth = 8

// ErrScopeTooDeep refuses a start more than MaxScanDepth component edges
// below the top of its component chain. The scan from the root would not
// reach that start, so another start in the same tree would never see the
// IDs allocated there; refusing is safer than risking an ID collision.
var ErrScopeTooDeep = errors.New("brainstorm: project is deeper than the BS-BAND scan limit")

// Scope is the BS root of a project and every directory of its allocation
// scope: the root first, then each level of components in DetectMultiRepo
// order, each absolute path once.
type Scope struct {
	Root string
	Dirs []string
}

// rootComponent is the Path DetectMultiRepo gives the scanned directory
// itself when that directory is a repository.
const rootComponent = "."

// ResolveScope applies BS Root Resolution items 1 and 2 to projectDir. The
// root is the top of the component chain: while the parent of the current
// directory is a repository whose DetectMultiRepo result lists it, the
// parent becomes current. The scope follows the same edges back down, so
// every start inside one component tree computes the same root and scope.
func ResolveScope(projectDir string) (Scope, error) {
	root, err := filepath.Abs(projectDir)
	if err != nil {
		return Scope{}, err
	}
	for depth := 0; ; depth++ {
		parent := filepath.Dir(root)
		if parent == root || !lists(setup.DetectMultiRepo(parent), parent, root) {
			break
		}
		if depth == MaxScanDepth {
			return Scope{}, ErrScopeTooDeep
		}
		root = parent
	}
	return Scope{Root: root, Dirs: componentTree(root)}, nil
}

// componentTree walks component edges breadth first from root, at most
// MaxScanDepth levels, visiting each absolute path once.
func componentTree(root string) []string {
	dirs := []string{root}
	visited := map[string]bool{root: true}
	level := []string{root}
	for depth := 0; depth < MaxScanDepth && len(level) > 0; depth++ {
		var next []string
		for _, dir := range level {
			for _, child := range children(setup.DetectMultiRepo(dir), dir) {
				if !visited[child] {
					visited[child] = true
					dirs = append(dirs, child)
					next = append(next, child)
				}
			}
		}
		level = next
	}
	return dirs
}

// children returns the component directories of a repository's
// DetectMultiRepo result. A directory that is not itself a repository (no
// root component) has no component edges, matching the chain rule.
func children(info *setup.MultiRepoInfo, dir string) []string {
	if info == nil {
		return nil
	}
	var out []string
	isRepository := false
	for _, component := range info.Components {
		if component.Path == rootComponent {
			isRepository = true
			continue
		}
		out = append(out, filepath.Join(dir, filepath.FromSlash(component.Path)))
	}
	if !isRepository {
		return nil
	}
	return out
}

// lists reports whether parent is a repository whose components include dir.
func lists(info *setup.MultiRepoInfo, parent, dir string) bool {
	for _, child := range children(info, parent) {
		if child == dir {
			return true
		}
	}
	return false
}
