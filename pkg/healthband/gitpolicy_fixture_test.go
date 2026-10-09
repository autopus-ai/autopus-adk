//go:build unix

package healthband

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gpRunnerLFS stands in for git-lfs 3.x: its clean, smudge, and
// filter-process commands first install its hooks into the repository
// (commands.installHooks) and then filter.
const gpRunnerLFS = "mkdir -p .git/hooks .git/lfs/tmp\n" +
	"for hook in pre-push post-checkout post-commit post-merge; do\n" +
	"\tprintf '#!/bin/sh\\ngit lfs %s \"$@\"\\n' \"$hook\" > \".git/hooks/$hook\"\n" +
	"done\nexec cat\n"

// gpLFSInstallSystem is what git lfs install --system writes, the
// configuration of a GitHub-hosted runner.
const gpLFSInstallSystem = "[filter \"lfs\"]\n\tclean = git-lfs clean -- %f\n\tsmudge = git-lfs smudge -- %f\n" +
	"\tprocess = git-lfs filter-process\n\trequired = true\n"

// The hostile fixture on a CI runner whose git creates hooks before the
// fixture links its markers: in CI run 37871017866 setup git created
// .git/hooks/post-checkout ("symlink ... post-checkout: file exists"). A
// runner reaches raw git through its template directory and its system
// configuration; the fixture still links every hook to its marker, runs no
// runner source, leaves no .git/lfs, and raw git runs the fixture's own hook
// and git-lfs driver.
func TestGPHostile_RunnerGitCreatesHooks_FixtureKeepsItsOwnSources(t *testing.T) {
	t.Parallel()
	for name, runner := range map[string]func(t *testing.T, dir string) []string{
		// GIT_TEMPLATE_DIR stands in for a default template that holds hooks.
		"template directory": func(t *testing.T, dir string) []string {
			return []string{"GIT_TEMPLATE_DIR=" + gpRunnerTemplate(t, dir)}
		},
		"system init.templateDir": func(t *testing.T, dir string) []string {
			return gpRunnerSystem(t, dir, "[init]\n\ttemplateDir = "+gpRunnerTemplate(t, dir)+"\n")
		},
		"system git-lfs filter": func(t *testing.T, dir string) []string {
			lfs := filepath.Join(dir, "lfs")
			require.NoError(t, os.WriteFile(lfs, []byte("#!/bin/sh\n"+gpRunnerLFS), 0o700))
			return gpRunnerSystem(t, dir, "[filter \"lfs\"]\n\tclean = "+lfs+" clean -- %f\n\tsmudge = "+lfs+
				" smudge -- %f\n\trequired = true\n")
		},
		"system git lfs install": func(t *testing.T, dir string) []string {
			if _, err := exec.LookPath("git-lfs"); err != nil {
				t.Skip("git-lfs is not installed")
			}
			return gpRunnerSystem(t, dir, gpLFSInstallSystem)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			h := newGPHostile(t, runner(t, dir)...)

			hooks, err := os.ReadDir(filepath.Join(h.repo, ".git", "hooks"))
			require.NoError(t, err)
			linked := make([]string, 0, len(hooks))
			for _, hook := range hooks {
				target, err := os.Readlink(filepath.Join(h.repo, ".git", "hooks", hook.Name()))
				require.NoError(t, err, "hook %s is the fixture's link", hook.Name())
				assert.Equal(t, filepath.Join(h.bin, "mark"), target, hook.Name())
				linked = append(linked, hook.Name())
			}
			assert.ElementsMatch(t, gpHookNames, linked, "the hooks are exactly the fixture's")
			assert.NoDirExists(t, filepath.Join(h.repo, ".git", "lfs"), "setup ran no git-lfs")

			h.git(h.controlEnv, h.repo, "worktree", "add", "--detach", filepath.Join(h.root, "control-wt"), h.base)
			assert.Subset(t, h.markerNames(), []string{"hook-post-checkout", "lfs-smudge"},
				"raw git runs the fixture's hook and git-lfs driver")
			ran, err := filepath.Glob(filepath.Join(dir, "ran-*"))
			require.NoError(t, err)
			assert.Empty(t, ran, "no runner hook ran")
		})
	}
}

// gpRunnerTemplate writes a template directory under dir whose hooks record
// that they ran.
func gpRunnerTemplate(t *testing.T, dir string) string {
	t.Helper()
	template := filepath.Join(dir, "template")
	require.NoError(t, os.MkdirAll(filepath.Join(template, "hooks"), 0o700))
	for _, name := range []string{"post-checkout", "pre-commit", "post-commit"} {
		hook := "#!/bin/sh\ntouch \"" + filepath.Join(dir, "ran-"+name) + "\"\n"
		require.NoError(t, os.WriteFile(filepath.Join(template, "hooks", name), []byte(hook), 0o700))
	}
	return template
}

// gpRunnerSystem writes config as the system configuration of raw git.
func gpRunnerSystem(t *testing.T, dir, config string) []string {
	t.Helper()
	path := filepath.Join(dir, "gitconfig")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	return []string{"GIT_CONFIG_SYSTEM=" + path}
}
