// Package verifygate runs the project's own build and tests on the files an
// agent session changed, so a session cannot end on code that does not build
// or pass its tests.
package verifygate

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
)

// ChangedFiles lists the paths that differ from HEAD in dir's work tree,
// including untracked files, relative to the repository root. ok is false
// when dir is not inside a git work tree.
func ChangedFiles(ctx context.Context, dir string) (root string, files []string, ok bool) {
	top, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, false
	}
	root = strings.TrimSpace(string(top))
	out, err := gitOutput(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return root, nil, false
	}
	return root, parsePorcelainZ(out), true
}

// parsePorcelainZ reads `git status --porcelain=v1 -z` output. A rename or
// copy entry is followed by its source path, which is skipped: only the new
// path exists in the work tree.
func parsePorcelainZ(out []byte) []string {
	var files []string
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		status, path := string(entry[:2]), string(entry[3:])
		if status[0] == 'R' || status[0] == 'C' {
			i++
		}
		if status[0] == 'D' || status[1] == 'D' {
			continue
		}
		files = append(files, path)
	}
	return files
}

func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd.Output()
}
