package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusReadOnlyGit are the git subcommands status may run; each only reads.
var statusReadOnlyGit = map[string]bool{
	"rev-parse": true, "diff": true, "ls-files": true, "status": true, "check-ignore": true,
}

// S12: status finds the installed AI CLIs and the Desktop helper but runs
// none of them, runs nothing but read-only git, and leaves every project file
// as it was. The CLIs and helper on PATH are traps that record any execution.
func TestStatusEvidence_StatusRunsNothingButReadOnlyGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the traps are POSIX shell scripts")
	}
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	f := newStatusFixture(t, "claude-code")
	f.writeSpec("SPEC-RUN-003", "approved")
	f.commit("baseline")
	f.stoppedRun("SPEC-RUN-003")
	f.write(".autopus/specs/SPEC-RUN-003/review-receipt.json",
		`{"schema":"spec_review_promotion_receipt.v1","run_id":"r","finished_at":"2026-11-01T00:00:00Z","verdict":"PASS"}`)
	f.write("pkg/app/app.go", "package app\n\n// uncommitted\n")

	bin, logs := t.TempDir(), t.TempDir()
	executed, gitLog := filepath.Join(logs, "executed"), filepath.Join(logs, "git.log")
	for _, name := range append(append([]string{}, statusAICLIs...), "orca", "npx", "node", runtimeHelperBinaryName) {
		trap := "#!/bin/sh\necho " + name + " >> '" + executed + "'\nexit 97\n"
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(trap), 0o755))
	}
	wrapper := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + gitLog + "'\nexec '" + realGit + "' \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0o755))
	t.Setenv("PATH", bin)
	t.Setenv(runtimeHelperOverrideEnv, filepath.Join(bin, runtimeHelperBinaryName))
	f.probe = defaultStatusProbe()
	before := statusTreeSnapshot(t, f.root)

	text := f.runStatus()
	f.runStatus("--json")

	assert.Equal(t, before, statusTreeSnapshot(t, f.root), "status writes no project file")
	assert.NoFileExists(t, executed, "status runs no AI CLI and no Desktop helper")
	assert.Contains(t, text, "AI CLIs on PATH: claude, codex, agy, opencode, omp", "the traps were found, not run")
	assert.Contains(t, text, "Desktop: installed, not connected")
	calls, err := os.ReadFile(gitLog)
	require.NoError(t, err)
	for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		assert.True(t, statusReadOnlyGit[gitSubcommand(strings.Fields(line))], "git %s", line)
	}
}
