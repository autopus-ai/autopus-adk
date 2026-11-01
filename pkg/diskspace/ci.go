package diskspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// CI states as the trim header prints them.
const (
	CIRunning       = "running"
	CIIdle          = "idle"
	CIForcedRunning = "forced-running"
	CIForcedIdle    = "forced-idle"
	CIUnknown       = "unknown"
)

// processListTimeout bounds one process listing.
const processListTimeout = 10 * time.Second

// Where a CI verdict came from.
const (
	CISourceFlag      = "flag"
	CISourceProcesses = "processes"
	CISourceNoNames   = "no_process_names"
)

// CIStatus is the CI verdict a trim mode is selected from.
type CIStatus struct {
	State   string
	Running bool
	Source  string
	// Reason says why detection failed when State is CIUnknown.
	Reason string
}

// ProcessLister lists the executable of every running process, each as a
// path or a bare name.
type ProcessLister interface {
	Executables(ctx context.Context) ([]string, error)
}

// ListerFunc adapts a function to ProcessLister.
type ListerFunc func(ctx context.Context) ([]string, error)

// Executables calls f.
func (f ListerFunc) Executables(ctx context.Context) ([]string, error) { return f(ctx) }

// DetectCI reports whether a CI job runs (REQ-DISK-TRIM-08): at least one
// process whose executable basename equals a configured name exactly. A full
// command line is never matched, because probe A3 counted the detector's own
// shells that way. override "running" or "idle" replaces detection. A lister
// failure is unknown and counts as running, since the CI rule set is the
// conservative error; the floor still selects emergency below it.
func DetectCI(ctx context.Context, lister ProcessLister, names []string, override string) CIStatus {
	switch override {
	case CIRunning:
		return CIStatus{State: CIForcedRunning, Running: true, Source: CISourceFlag}
	case CIIdle:
		return CIStatus{State: CIForcedIdle, Source: CISourceFlag}
	}
	if len(names) == 0 {
		return CIStatus{State: CIIdle, Source: CISourceNoNames}
	}
	ctx, cancel := context.WithTimeout(ctx, processListTimeout)
	defer cancel()
	executables, err := lister.Executables(ctx)
	if err != nil {
		return CIStatus{State: CIUnknown, Running: true, Source: CISourceProcesses, Reason: err.Error()}
	}
	for _, executable := range executables {
		if slices.Contains(names, executableBase(executable)) {
			return CIStatus{State: CIRunning, Running: true, Source: CISourceProcesses}
		}
	}
	return CIStatus{State: CIIdle, Source: CISourceProcesses}
}

// executableBase is the basename of a lister row. ps prints some processes'
// names in parentheses, so one surrounding pair is stripped first.
func executableBase(executable string) string {
	executable = strings.TrimSpace(executable)
	if len(executable) >= 2 && executable[0] == '(' && executable[len(executable)-1] == ')' {
		executable = executable[1 : len(executable)-1]
	}
	if i := strings.LastIndex(executable, "/"); i >= 0 {
		return executable[i+1:]
	}
	return executable
}

// SystemProcessLister is the lister of this operating system: `ps -axo comm=`
// on darwin, which prints each executable path, and /proc on linux.
func SystemProcessLister(run Runner) ProcessLister {
	switch runtime.GOOS {
	case "darwin":
		return psLister{run: run}
	case "linux":
		return procLister{root: "/proc"}
	default:
		return ListerFunc(func(context.Context) ([]string, error) {
			return nil, fmt.Errorf("process listing is not supported on %s", runtime.GOOS)
		})
	}
}

type psLister struct{ run Runner }

func (p psLister) Executables(ctx context.Context) ([]string, error) {
	out, err := p.run(ctx, "", nil, "ps", "-axo", "comm=")
	if err != nil {
		return nil, fmt.Errorf("ps -axo comm=: %w", err)
	}
	var executables []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			executables = append(executables, line)
		}
	}
	return executables, nil
}

// procLister reads /proc/<pid>/comm and the /proc/<pid>/exe link of every
// process. comm is cut at 15 bytes and exe is unreadable for another user's
// process, so both are listed and either may match.
type procLister struct{ root string }

func (p procLister) Executables(context.Context) ([]string, error) {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		return nil, err
	}
	var executables []string
	for _, entry := range entries {
		if strings.TrimLeft(entry.Name(), "0123456789") != "" {
			continue
		}
		dir := filepath.Join(p.root, entry.Name())
		if comm, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
			executables = append(executables, strings.TrimSuffix(string(comm), "\n"))
		}
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
			executables = append(executables, exe)
		}
	}
	return executables, nil
}
