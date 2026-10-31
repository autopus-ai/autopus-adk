package diskspace

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// Runner runs an external program in dir (the current directory when empty)
// and returns its standard output. Every program the package starts goes
// through a Runner, so a test can record them all: the package runs no go
// subprocess other than `go env GOCACHE`, and never `go clean`
// (REQ-DISK-TRIM-07).
type Runner func(ctx context.Context, dir, name string, args ...string) ([]byte, error)

// ExecRunner starts real processes. Standard error is discarded, as
// runSyncGit does in internal/cli: git and go can print absolute paths and
// credentials embedded in remotes there.
func ExecRunner(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), err
}

// exitCode is the exit status a Runner error carries, 0 for nil, and -1 when
// the program did not run to an exit status.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return -1
}
