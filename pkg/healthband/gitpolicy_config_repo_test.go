//go:build unix

package healthband

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gpInit creates an empty repository and applies config and an
// info/attributes text through raw git.
func (f *gpFixture) gpInit(name string, config [][2]string, infoAttributes string) string {
	dir := filepath.Join(f.root, name)
	f.git(f.setupEnv, f.root, "init", "-q", dir)
	for _, item := range config {
		f.git(f.setupEnv, dir, "config", item[0], item[1])
	}
	if infoAttributes != "" {
		require.NoError(f.t, os.WriteFile(filepath.Join(dir, ".git", "info", "attributes"), []byte(infoAttributes), 0o600))
	}
	return dir
}

// SPEC-SIGMABAND-002 S6: each fixture, checked in the user's checkout by
// CheckConfig over real git, gives exactly its code.
func TestGitPolicyRunner_CheckConfigGivesTheS6Codes(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	env := f.home("band-home", "", "")
	ctx := gpContext(t)
	lfs := filepath.Join(f.bin, "git-lfs")
	cases := []struct {
		name           string
		config         [][2]string
		infoAttributes string
		want           string
	}{
		{"clean repository", nil, "", ""},
		{"info attributes", nil, "*.py filter=fmt\n", "git_config_unsafe:info_attributes"},
		{"info attributes comments only", nil, "# comment\n\n   # indented\r\n\t\n", ""},
		{"local clean", [][2]string{{"filter.mark.clean", "tools/clean.py"}}, "", "git_config_unsafe:filter.mark.clean"},
		{"interpreter smudge", [][2]string{{"filter.mark.smudge", "/usr/bin/python3 tools/smudge.py"}}, "", "git_config_unsafe:filter.mark.smudge"},
		{"lfs interpreter", [][2]string{{"filter.lfs.process", "/usr/bin/python3 tools/lfs.py"}}, "", "git_config_unsafe:filter.lfs.process"},
		{"textconv", [][2]string{{"diff.tc.textconv", "/bin/sh tools/tc.sh"}}, "", "git_config_unsafe:diff.tc.textconv"},
		{"merge driver", [][2]string{{"merge.m.driver", "tools/m.sh"}}, "", "git_config_unsafe:merge.m.driver"},
		{"lfs extension", [][2]string{{"lfs.extension.x.clean", "tools/x.sh"}}, "", "git_config_unsafe:lfs.extension.x.clean"},
		{"lfs custom transfer", [][2]string{{"lfs.customtransfer.y.path", "tools/y"}}, "", "git_config_unsafe:lfs.customtransfer.y.path"},
		{"alternate refs", [][2]string{{"core.alternateRefsCommand", "x"}}, "", "git_config_unsafe:core.alternateRefsCommand"},
		{"config hook", [][2]string{{"hook.lint.command", "make lint"}}, "", "git_config_unsafe:hook.lint.command"},
		{"lfs filter-process", [][2]string{{"filter.lfs.process", "git-lfs filter-process"}}, "", ""},
		{"lfs absolute", [][2]string{{"filter.lfs.process", lfs + " filter-process"}, {"filter.lfs.required", "true"}}, "", ""},
		{"lfs clean", [][2]string{{"filter.lfs.clean", "git-lfs clean -- %f"}}, "", ""},
		{"lfs newline", [][2]string{{"filter.lfs.process", "git-lfs\nfilter-process"}}, "", "git_config_unsafe:filter.lfs.process"},
		{"lfs two spaces", [][2]string{{"filter.lfs.process", "git-lfs  filter-process"}}, "", "git_config_unsafe:filter.lfs.process"},
		{"lfs substitution", [][2]string{{"filter.lfs.process", "/usr/local/$(id)/git-lfs filter-process"}}, "", "git_config_unsafe:filter.lfs.process"},
	}
	for i, tc := range cases {
		repo := f.gpInit(fmt.Sprintf("repo-%d", i), tc.config, tc.infoAttributes)
		code, err := f.runner(env, repo).CheckConfig(ctx)
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.want, code, tc.name)
	}
}

// A non-LFS filter that only global configuration sets is refused although
// no attribute source selects it: the intended fail-closed limitation of
// probe A3. A global attributes file with its driver is refused the same way.
func TestGitPolicyRunner_CheckConfigRefusesGlobalOnlyDrivers(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	ctx := gpContext(t)
	repo := f.gpInit("repo", nil, "")
	globalOnly := f.home("global-only", "[filter \"fmt\"]\n\tclean = tools/fmt.sh\n", "")
	code, err := f.runner(globalOnly, repo).CheckConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "git_config_unsafe:filter.fmt.clean", code)

	attrs := filepath.Join(f.root, "attrs")
	require.NoError(t, os.WriteFile(attrs, []byte("*.go filter=fmt\n"), 0o600))
	globalAttrs := f.home("global-attrs", "[core]\n\tattributesFile = "+attrs+"\n[filter \"fmt\"]\n\tsmudge = fmt\n", "")
	code, err = f.runner(globalAttrs, repo).CheckConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "git_config_unsafe:filter.fmt.smudge", code)
}

// S6 and CD-3 F-001: an includeIf "gitdir:**/worktrees/**" section of the
// global configuration passes the check in the user's checkout and is found
// inside the new worktree after git worktree add --no-checkout, before any
// file is checked out; the common directory's info/attributes is found from
// the worktree as well.
func TestGitPolicyRunner_CheckConfigInsideTheNewWorktree(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	ctx := gpContext(t)
	include := filepath.Join(f.root, "worktree.inc")
	require.NoError(t, os.WriteFile(include, []byte("[filter \"mark\"]\n\tsmudge = "+f.script("smudge", f.marker("smudge"))+"\n"), 0o600))
	env := f.home("band-home", "[includeIf \"gitdir:**/worktrees/**\"]\n\tpath = "+include+"\n", "")
	repo, base := f.repo("repo", map[string]string{".gitattributes": "*.go filter=mark\n", "a.go": "package a\n"})
	r := f.runner(env, repo)
	code, err := r.CheckConfig(ctx)
	require.NoError(t, err)
	assert.Empty(t, code, "the user's checkout does not match gitdir:**/worktrees/**")

	wt := filepath.Join(f.root, "lp", "key", "worktree")
	f.must(r.Run(ctx, "worktree", "add", "--no-checkout", "--detach", wt, base))
	code, err = r.In(wt).CheckConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "git_config_unsafe:filter.mark.smudge", code)
	entries, err := os.ReadDir(wt)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the .git file exists before the checkout")
	assert.Equal(t, ".git", entries[0].Name())
	assert.Empty(t, f.markerNames())

	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "info", "attributes"), []byte("*.go ident\n"), 0o600))
	code, err = r.In(wt).CheckConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "git_config_unsafe:info_attributes", code, "a linked worktree reads $GIT_COMMON_DIR/info/attributes")
}

// Fail-closed edges: an info/attributes band cannot read as a regular file,
// a directory that is no repository, and a configuration git cannot parse
// give a code; only a done context gives an error.
func TestGitPolicyRunner_CheckConfigFailsClosed(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	env := f.home("band-home", "", "")
	ctx := gpContext(t)

	linked := f.gpInit("linked", nil, "")
	require.NoError(t, os.WriteFile(filepath.Join(f.root, "attrs"), nil, 0o600))
	require.NoError(t, os.Symlink(filepath.Join(f.root, "attrs"), filepath.Join(linked, ".git", "info", "attributes")))
	directory := f.gpInit("directory", nil, "")
	require.NoError(t, os.Mkdir(filepath.Join(directory, ".git", "info", "attributes"), 0o700))
	broken := f.gpInit("broken", nil, "")
	require.NoError(t, os.WriteFile(filepath.Join(broken, ".git", "config"), []byte("[core\n"), 0o600))
	outside := filepath.Join(f.root, "outside")
	require.NoError(t, os.Mkdir(outside, 0o700))

	for dir, want := range map[string]string{
		linked: "git_config_unsafe:info_attributes", directory: "git_config_unsafe:info_attributes",
		broken: "git_config_unsafe:config_unreadable", outside: "git_config_unsafe:config_unreadable",
	} {
		code, err := f.runner(env, dir).CheckConfig(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, code, dir)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	code, err := f.runner(env, linked).CheckConfig(cancelled)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, code)
}
