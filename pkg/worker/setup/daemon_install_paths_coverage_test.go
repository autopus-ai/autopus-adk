package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// In-process installation must refuse the running test binary: its basename is
// not "auto". Guards against the daemon being installed from an arbitrary
// executable.
func TestInstallAndStartDaemon_RejectsNonAutoExecutable(t *testing.T) {
	err := installAndStartDaemon()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to install worker daemon")
}

// The identity of the running binary must report this module while still
// failing validation, proving the module check alone cannot authorize a
// non-release build.
func TestCurrentDaemonExecutableIdentityAndValidate(t *testing.T) {
	identity, err := currentDaemonExecutableIdentity()
	require.NoError(t, err)

	assert.Equal(t, autoModulePath, identity.ModulePath)
	assert.NotEqual(t, autoMainPackagePath, identity.PackagePath)

	err = validateDaemonExecutable(filepath.Join(t.TempDir(), "auto"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "untrusted auto build identity")
}

// Home-directory resolution failure must abort both installers before any
// filesystem or service-manager side effect.
func TestInstallDaemon_MissingHomeDir(t *testing.T) {
	t.Setenv("HOME", "")

	err := installSystemdDaemon("/usr/local/bin/auto")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get home dir")

	if runtime.GOOS == "darwin" {
		// launchctl probing would inspect the real user session.
		return
	}
	err = installLaunchdDaemon("/usr/local/bin/auto")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get home dir")
}

// The systemd installer must write the unit under the resolved HOME and report
// which step failed; each failing step gets its own error context.
func TestInstallSystemdDaemon_UnitPathFailures(t *testing.T) {
	dir := t.TempDir()

	// HOME is a regular file: the unit directory cannot be created.
	homeFile := filepath.Join(dir, "home-file")
	require.NoError(t, os.WriteFile(homeFile, []byte("x"), 0o600))
	t.Setenv("HOME", homeFile)

	err := installSystemdDaemon("/usr/local/bin/auto")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create systemd user dir")

	// Unit path occupied by a directory: the unit file cannot be written.
	home := filepath.Join(dir, "home")
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	require.NoError(t, os.MkdirAll(filepath.Join(unitDir, "autopus-worker.service"), 0o755))
	t.Setenv("HOME", home)

	err = installSystemdDaemon("/usr/local/bin/auto")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write unit file")
}

// The launchd installer must create the log and LaunchAgents directories under
// the resolved HOME and write a plist whose ProgramArguments point at the given
// binary. Skipped on darwin because launchctl would touch the real session.
func TestInstallLaunchdDaemon_WritesPlistUnderHome(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("launchctl probe and load would affect the real user session")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	// launchctl is absent on non-darwin, so the load step fails after the
	// plist has been written.
	err := installLaunchdDaemon("/usr/local/bin/auto")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "launchctl load")

	plist, readErr := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", "co.autopus.worker.plist"))
	require.NoError(t, readErr)
	logDir := filepath.Join(home, ".config", "autopus", "logs")
	assert.Contains(t, string(plist), "<string>/usr/local/bin/auto</string>")
	assert.Contains(t, string(plist), logDir+"/autopus-worker.out.log")

	info, statErr := os.Stat(logDir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// Failure to create the log directory must stop the launchd install before any
// plist is written.
func TestInstallLaunchdDaemon_LogDirFailure(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("launchctl probe would affect the real user session")
	}

	dir := t.TempDir()
	homeFile := filepath.Join(dir, "home-file")
	require.NoError(t, os.WriteFile(homeFile, []byte("x"), 0o600))
	t.Setenv("HOME", homeFile)

	err := installLaunchdDaemon("/usr/local/bin/auto")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create log dir")
}
