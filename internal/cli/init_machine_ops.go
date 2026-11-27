package cli

import (
	"os"
	"path/filepath"
)

// initMachineOps is the machine_ops value init writes. A project that
// already has autopus.yaml, here or in a directory above (a monorepo
// package), keeps that file's value, so a re-init or a nested init does
// not turn the shared-machine helpers on or off behind the user's back. A
// file that exists but does not load (a key this binary does not know)
// keeps the unset default too. Only a new project starts with the
// verification core.
func initMachineOps(dir string) *bool {
	if existing := nearestConfig(dir); existing != nil {
		return existing.MachineOps
	}
	if hasAncestorConfig(dir) {
		return nil
	}
	off := false
	return &off
}

// hasAncestorConfig reports an autopus.yaml in dir or a directory above.
func hasAncestorConfig(dir string) bool {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "autopus.yaml")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
