package cli_test

// SPEC-PANERM-001 T3 (S6): the config notice is exact, single, and
// quiet-aware. A built auto binary runs `auto check --arch` on C2 and on C2'
// (C2 without its group K lines) with stderr on a pseudo-terminal, then with
// --quiet, then with stderr on a pipe. Only the first C2 run may differ from
// its C2' control, by exactly one notice line on stderr. Red at B: no notice
// exists. T12 un-skips it; the "two loads through one notifier" half of S6 is
// a unit test of the T12 notifier API.

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const panermNoticeSuffix = `; the orchestra pane backend was retired (SPEC-PANERM-001); run "auto update" or delete the keys`

type panermRun struct {
	stdout, stderr string
	exit           int
}

// buildPanermAuto builds cmd/auto from this module into a temp dir.
func buildPanermAuto(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	bin := filepath.Join(t.TempDir(), "auto")
	build := exec.Command("go", "build", "-o", bin, "./cmd/auto")
	build.Dir = filepath.Join(filepath.Dir(file), "..", "..")
	var stderr bytes.Buffer
	build.Stderr = &stderr
	require.NoError(t, build.Run(), stderr.String())
	return bin
}

// runPanermAuto runs the binary in dir with a scratch HOME and a fixed PATH;
// with tty set, stderr is the slave side of a fresh pseudo-terminal.
func runPanermAuto(t *testing.T, bin, dir string, tty bool, args ...string) panermRun {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "TMPDIR=" + t.TempDir()}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	var err error
	if !tty {
		cmd.Stderr = &stderr
		err = cmd.Run()
	} else {
		master, slave, ptyErr := openPanermPTY()
		if ptyErr != nil {
			t.Skipf("no pseudo-terminal: %v", ptyErr)
		}
		defer master.Close()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = io.Copy(&stderr, master) // ends with EOF or EIO once the child exits
		}()
		cmd.Stderr = slave
		startErr := cmd.Start()
		_ = slave.Close()
		require.NoError(t, startErr)
		err = cmd.Wait()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("pseudo-terminal output did not drain")
		}
	}
	exit := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exit = exitErr.ExitCode()
	} else {
		require.NoError(t, err)
	}
	// The terminal line discipline turns "\n" into "\r\n".
	return panermRun{stdout: stdout.String(), stderr: strings.ReplaceAll(stderr.String(), "\r\n", "\n"), exit: exit}
}

func panermConfigWorkspace(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), readLegacyPaneConfig(t, fixture), 0o644))
	return dir
}

// withoutLine removes every line equal to line and reports how many it removed.
func withoutLine(text, line string) (string, int) {
	var kept []string
	removed := 0
	for _, item := range strings.SplitAfter(text, "\n") {
		if strings.TrimSuffix(item, "\n") == line {
			removed++
			continue
		}
		kept = append(kept, item)
	}
	return strings.Join(kept, ""), removed
}

func TestPanermS6_ConfigNoticeIsExactSingleAndQuietAware(t *testing.T) {
	skipUntilPanermTask(t, "T12 (W3)", "B writes no config notice when a load ignores group K keys")
	bin := buildPanermAuto(t)
	c2, c2Prime := panermConfigWorkspace(t, "c2.yaml"), panermConfigWorkspace(t, "c2-prime.yaml")
	notice := "auto: warning: ignored removed autopus.yaml keys: " + strings.Join(panermP2, ", ") + panermNoticeSuffix

	for _, tc := range []struct {
		name    string
		tty     bool
		args    []string
		notices int
	}{
		{name: "terminal", tty: true, args: []string{"check", "--arch"}, notices: 1},
		{name: "quiet", tty: true, args: []string{"check", "--arch", "--quiet"}, notices: 0},
		{name: "pipe", tty: false, args: []string{"check", "--arch"}, notices: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runPanermAuto(t, bin, c2, tc.tty, tc.args...)
			control := runPanermAuto(t, bin, c2Prime, tc.tty, tc.args...)

			rest, notices := withoutLine(got.stderr, notice)
			assert.Equal(t, tc.notices, notices, "notice lines on stderr:\n%s", got.stderr)
			assert.Equal(t, control.stderr, rest, "stderr apart from the notice equals the C2' run")
			assert.Equal(t, control.stdout, got.stdout, "stdout equals the C2' run")
			assert.NotContains(t, got.stdout, "ignored removed autopus.yaml keys", "the notice never reaches stdout")
			assert.Equal(t, control.exit, got.exit, "exit code equals the C2' run")
		})
	}
}

// The S6 terminal runs are real terminals: a shell sees fd 2 as a tty.
func TestPanermS6_PseudoTerminalStderrIsATerminal(t *testing.T) {
	got := runPanermAuto(t, "/bin/sh", t.TempDir(), true, "-c", "if [ -t 2 ]; then echo tty >&2; else echo notty >&2; fi")
	assert.Equal(t, "tty\n", got.stderr)
	assert.Equal(t, 0, got.exit)
	piped := runPanermAuto(t, "/bin/sh", t.TempDir(), false, "-c", "if [ -t 2 ]; then echo tty >&2; else echo notty >&2; fi")
	assert.Equal(t, "notty\n", piped.stderr)
}
