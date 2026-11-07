package opencode

import (
	"path/filepath"
)

// opencodePruneRoots are the trees where a manifest-recorded file the
// generator no longer emits is deleted on update. Agents, commands, and rules
// are included so routes, roles, and rules a release drops do not outlive it;
// a file the user wrote there is never recorded and so never pruned.
func opencodePruneRoots() []string {
	return []string{
		filepath.ToSlash(filepath.Join(".agents", "skills")),
		filepath.ToSlash(filepath.Join(".opencode", "skills")),
		filepath.ToSlash(filepath.Join(".opencode", "agents")),
		filepath.ToSlash(filepath.Join(".opencode", "commands")),
		filepath.ToSlash(filepath.Join(".opencode", "rules", "autopus")),
	}
}
