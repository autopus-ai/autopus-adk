package content_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/stretchr/testify/require"
)

// writeStubAuto writes a fake harness binary that records the argv it received,
// so a hook run proves which binary answered rather than which string appears.
func writeStubAuto(t *testing.T, dir string) (bin, log string) {
	t.Helper()
	bin = filepath.Join(dir, "stub-auto")
	log = filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\nexit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return bin, log
}

func runHookScript(t *testing.T, script, autopusBin string, args ...string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hook")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))

	cmd := exec.Command("/bin/sh", append([]string{path}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AUTOPUS_BIN="+autopusBin)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// A self-hosting checkout commits source that the released binary on PATH does
// not contain, so the hooks must be able to run the build under test. These
// assert the override actually reaches the process git spawns.
func TestPreCommitHook_RunsAutopusBinOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin, log := writeStubAuto(t, dir)

	_, gitHooks, err := content.GenerateHookConfigs(config.HooksConf{PreCommitArch: true}, "gemini", false)
	require.NoError(t, err)
	require.NotEmpty(t, gitHooks)

	runHookScript(t, gitHooks[0].Content, bin)

	recorded, readErr := os.ReadFile(log)
	require.NoError(t, readErr)
	require.Equal(t, "check --hygiene --arch --quiet --staged\n", string(recorded))
}

func TestCommitMsgHook_RunsAutopusBinOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin, log := writeStubAuto(t, dir)
	msg := filepath.Join(dir, "COMMIT_EDITMSG")
	require.NoError(t, os.WriteFile(msg, []byte("fix(x): y\n"), 0o644))

	_, gitHooks, err := content.GenerateHookConfigs(
		config.HooksConf{PreCommitArch: true, PreCommitLore: true}, "gemini", false,
	)
	require.NoError(t, err)

	var script string
	for _, hook := range gitHooks {
		if hook.Path == ".git/hooks/commit-msg" {
			script = hook.Content
		}
	}
	require.NotEmpty(t, script)

	runHookScript(t, script, bin, msg)

	recorded, readErr := os.ReadFile(log)
	require.NoError(t, readErr)
	require.Equal(t,
		"check --lore --quiet --message "+msg+"\nlore validate "+msg+"\n",
		string(recorded),
	)
}
