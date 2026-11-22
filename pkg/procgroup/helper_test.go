//go:build unix || windows

package procgroup

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The test binary doubles as the processes under test, chosen by helperEnv:
//
//	parent  starts an "escaper" detached from its group (its own session on
//	        Unix, its own console process group on Windows), writes the
//	        escaper's PID to pidEnv, and waits forever
//	escaper waits forever
//	trap    on SIGTERM writes "cleaned" next to pidEnv and exits
//	stubborn ignores SIGTERM and waits forever
const (
	helperEnv = "PROCGROUP_TEST_HELPER"
	pidEnv    = "PROCGROUP_TEST_PIDFILE"
)

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "parent":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), helperEnv+"=escaper")
		detach(child)
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		pid := strconv.Itoa(child.Process.Pid)
		if os.WriteFile(os.Getenv(pidEnv), []byte(pid), 0o600) != nil {
			os.Exit(4)
		}
	case "trap":
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM)
		_ = os.WriteFile(os.Getenv(pidEnv), []byte(strconv.Itoa(os.Getpid())), 0o600)
		<-sig
		_ = os.WriteFile(filepath.Join(filepath.Dir(os.Getenv(pidEnv)), "cleaned"), nil, 0o600)
		os.Exit(0)
	case "stubborn":
		signal.Ignore(syscall.SIGTERM)
		_ = os.WriteFile(os.Getenv(pidEnv), []byte(strconv.Itoa(os.Getpid())), 0o600)
	}
	time.Sleep(time.Hour)
	os.Exit(0)
}

// helperCommand returns a command running the test binary as helper under
// ctx, and the file it writes a PID to.
func helperCommand(t *testing.T, ctx context.Context, helper string) (*exec.Cmd, string) {
	t.Helper()
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.CommandContext(ctx, os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"="+helper, pidEnv+"="+pidFile)
	return cmd, pidFile
}

// readPID waits for a helper to write its PID file.
func readPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			if err != nil {
				t.Fatalf("pid file %s: %v", path, err)
			}
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("helper never wrote %s", path)
	return 0
}
