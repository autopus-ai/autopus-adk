package diskspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rows(executables ...string) ProcessLister {
	return ListerFunc(func(context.Context) ([]string, error) { return executables, nil })
}

// S6: detection matches executable basenames only, and a lister failure is
// unknown and counts as running, with its reason.
func TestDetectCI_MatchesBasenamesOnly(t *testing.T) {
	t.Parallel()
	names := []string{"Runner.Worker"}
	failing := ListerFunc(func(context.Context) ([]string, error) { return nil, errors.New("ps: not permitted") })

	for _, tc := range []struct {
		name   string
		lister ProcessLister
		want   CIStatus
	}{
		{"worker running", rows("/opt/actions-runner/bin/Runner.Listener", "/opt/actions-runner/bin/Runner.Worker", "/bin/zsh"),
			CIStatus{State: CIRunning, Running: true, Source: CISourceProcesses}},
		{"listener only", rows("/opt/actions-runner/bin/Runner.Listener", "/bin/zsh", "/usr/bin/grep"), CIStatus{State: CIIdle, Source: CISourceProcesses}},
		{"suffixed name", rows("/opt/actions-runner/bin/Runner.Worker.old", "/bin/zsh"), CIStatus{State: CIIdle, Source: CISourceProcesses}},
		{"command line mention", rows("/bin/zsh -c ps | grep Runner.Worker"), CIStatus{State: CIIdle, Source: CISourceProcesses}},
		{"bare comm", rows("Runner.Worker"), CIStatus{State: CIRunning, Running: true, Source: CISourceProcesses}},
		{"lister error", failing, CIStatus{State: CIUnknown, Running: true, Source: CISourceProcesses, Reason: "ps: not permitted"}},
	} {
		assert.Equal(t, tc.want, DetectCI(context.Background(), tc.lister, names, ""), tc.name)
	}
}

// S6: --ci overrides detection both ways without consulting the lister, and
// an empty name list detects nothing.
func TestDetectCI_OverridesAreForced(t *testing.T) {
	t.Parallel()
	names := []string{"Runner.Worker"}
	unused := ListerFunc(func(context.Context) ([]string, error) {
		t.Error("an override must not list processes")
		return nil, nil
	})
	assert.Equal(t, CIStatus{State: CIForcedIdle, Source: CISourceFlag}, DetectCI(context.Background(), unused, names, "idle"))
	assert.Equal(t, CIStatus{State: CIForcedRunning, Running: true, Source: CISourceFlag}, DetectCI(context.Background(), unused, names, "running"))
	assert.Equal(t, CIStatus{State: CIIdle, Source: CISourceNoNames}, DetectCI(context.Background(), unused, nil, ""))
}

// The darwin lister reads `ps -axo comm=` through the Runner.
func TestPSLister_ParsesCommColumn(t *testing.T) {
	t.Parallel()
	recorder := &execRecorder{answers: map[string]string{
		"ps -axo comm=": "/sbin/launchd\n/opt/actions-runner/bin/Runner.Worker\n\n  /bin/zsh  \n",
	}}

	executables, err := psLister{run: recorder.run}.Executables(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"/sbin/launchd", "/opt/actions-runner/bin/Runner.Worker", "/bin/zsh"}, executables)
	assert.Equal(t, []string{"ps -axo comm="}, recorder.calls)
}

// The linux lister reads comm and the exe link of every numeric /proc entry.
func TestProcLister_ReadsCommAndExe(t *testing.T) {
	t.Parallel()
	proc := t.TempDir()
	write := func(rel, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(proc, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(proc, rel), []byte(content), 0o644))
	}
	write("101/comm", "Runner.Worker\n")
	require.NoError(t, os.Symlink("/opt/actions-runner/bin/Runner.Worker", filepath.Join(proc, "101", "exe")))
	write("202/comm", "zsh\n")
	write("self/comm", "Runner.Worker\n")
	write("sys/comm", "Runner.Worker\n")

	executables, err := procLister{root: proc}.Executables(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"Runner.Worker", "/opt/actions-runner/bin/Runner.Worker", "zsh"}, executables)
	_, err = procLister{root: filepath.Join(proc, "missing")}.Executables(context.Background())
	assert.Error(t, err)
}

// S6 live: on this machine the system lister answers without error, and a
// Runner.Worker counts only when one is really running.
func TestSystemProcessLister_ListsThisMachine(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no process lister on " + runtime.GOOS)
	}
	executables, err := SystemProcessLister(ExecRunner).Executables(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, executables)
	self, err := os.Executable()
	require.NoError(t, err)
	assert.Contains(t, bases(executables), executableBase(self)[:min(15, len(executableBase(self)))])
}

func bases(executables []string) []string {
	var out []string
	for _, executable := range executables {
		base := executableBase(executable)
		out = append(out, base[:min(15, len(base))])
	}
	return out
}
