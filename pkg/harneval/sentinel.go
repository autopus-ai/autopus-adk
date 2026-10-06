package harneval

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// sentinelBinaries are the host CLIs a generation path could probe. While
// generation runs, PATH holds only a recording stand-in for each of them.
var sentinelBinaries = []string{"codex", "opencode", "claude", "agy", "gemini", "omp", "git"}

// hostEnvMu serializes the process-wide PATH and HOME swap.
var hostEnvMu sync.Mutex

// sentinelScript records "name args..." with shell builtins only, because PATH
// holds nothing else, and fails so the probe sees an unusable CLI.
const sentinelScript = `#!/bin/sh
line="${0##*/}"
for arg in "$@"; do line="$line $arg"; done
printf '%%s\n' "$line" >> '%s'
exit 127
`

// withSentinel runs fn with PATH set to a directory of recording sentinels and
// HOME set to an empty directory, restores both, and returns every recorded
// invocation in call order. PATH and HOME are process-wide: callers in this
// package are serialized, and nothing else in the process should start a
// subprocess while fn runs. The stand-ins are POSIX shell scripts.
func withSentinel(fn func() error) ([]string, error) {
	hostEnvMu.Lock()
	defer hostEnvMu.Unlock()
	dir, err := os.MkdirTemp("", "harneval-sentinel-")
	if err != nil {
		return nil, fmt.Errorf("sentinel directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	binDir, home, logPath := filepath.Join(dir, "bin"), filepath.Join(dir, "home"), filepath.Join(dir, "sentinel.log")
	if strings.ContainsAny(logPath, "'\n") {
		return nil, fmt.Errorf("sentinel log path %q cannot be quoted", logPath)
	}
	for _, sub := range []string{binDir, home} {
		if err := os.Mkdir(sub, 0o700); err != nil {
			return nil, fmt.Errorf("sentinel directory: %w", err)
		}
	}
	script := fmt.Sprintf(sentinelScript, logPath)
	for _, name := range sentinelBinaries {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o700); err != nil {
			return nil, fmt.Errorf("sentinel %s: %w", name, err)
		}
	}
	restore := swapEnv(map[string]string{"PATH": binDir, "HOME": home})
	defer restore()
	runErr := fn()
	invocations, readErr := readSentinelLog(logPath)
	if readErr != nil {
		return nil, errors.Join(runErr, readErr)
	}
	return invocations, runErr
}

func swapEnv(values map[string]string) func() {
	type saved struct {
		value string
		set   bool
	}
	previous := make(map[string]saved, len(values))
	for key, value := range values {
		old, set := os.LookupEnv(key)
		previous[key] = saved{value: old, set: set}
		_ = os.Setenv(key, value)
	}
	return func() {
		for key, old := range previous {
			if old.set {
				_ = os.Setenv(key, old.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}
}

func readSentinelLog(logPath string) ([]string, error) {
	data, err := os.ReadFile(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sentinel log: %w", err)
	}
	var invocations []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			invocations = append(invocations, line)
		}
	}
	return invocations, nil
}

// invokedBinaries returns the sorted unique binary names in the invocations.
func invokedBinaries(invocations []string) []string {
	seen := map[string]bool{}
	var names []string
	for _, line := range invocations {
		name, _, _ := strings.Cut(line, " ")
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// UnpinnedProbeError reports that generation reached a host CLI: a probe that
// no manifest pin covers would make the surface depend on the host.
type UnpinnedProbeError struct {
	Binaries    []string
	Invocations []string
}

func (e *UnpinnedProbeError) Error() string {
	return ReasonHostProbeUnpinned + ": " + strings.Join(e.Binaries, ", ")
}
