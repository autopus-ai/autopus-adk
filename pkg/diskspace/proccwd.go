package diskspace

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CwdLister lists the working directories of the running processes. An
// error means the list is unknown, and a reclaim rule that needs it keeps
// the worktree.
type CwdLister func(ctx context.Context) ([]string, error)

var errNoCwds = errors.New("process working directories are unavailable")

// ProcessCwdLister reads /proc/<pid>/cwd where /proc exists, and otherwise
// asks `lsof -d cwd`. lsof exits 1 when it could not read every process;
// what it printed is still used, but an empty answer is unknown.
func ProcessCwdLister(run Runner) CwdLister {
	return func(ctx context.Context) ([]string, error) {
		if cwds, ok := procCwds(); ok {
			return cwds, nil
		}
		out, err := run(ctx, "", nil, "lsof", "-w", "-n", "-P", "-d", "cwd", "-Fn")
		cwds := parseLsofNames(out)
		if len(cwds) == 0 {
			if err == nil {
				err = errNoCwds
			}
			return nil, err
		}
		return cwds, nil
	}
}

func procCwds() ([]string, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	var cwds []string
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		if cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd")); err == nil {
			cwds = append(cwds, cwd)
		}
	}
	return cwds, len(cwds) > 0
}

// parseLsofNames reads the n<path> lines of lsof -F output.
func parseLsofNames(out []byte) []string {
	var names []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if name, ok := strings.CutPrefix(scanner.Text(), "n"); ok && filepath.IsAbs(name) {
			names = append(names, name)
		}
	}
	return names
}

// canonicalCwds resolves each listed cwd once, so a worktree reached through
// a symlinked parent still matches.
func canonicalCwds(cwds []string) []string {
	out := make([]string, 0, len(cwds))
	seen := map[string]bool{}
	for _, cwd := range cwds {
		if !seen[cwd] {
			seen[cwd] = true
			out = append(out, canonicalOrSelf(cwd))
		}
	}
	return out
}

// holdsCwd reports whether any canonical cwd is path or below it.
func holdsCwd(path string, cwds []string) bool {
	for _, cwd := range cwds {
		if cwd == path || strings.HasPrefix(cwd, path+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
