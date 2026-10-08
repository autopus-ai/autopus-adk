package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// Canaries a child environment must never hold.
const (
	exportTokenCanary = "ghs_harneval_canary_token"
	exportOIDCCanary  = "oidc_harneval_canary_token"
)

func TestHarnessChildEnv_KeepsOnlyTheAllowlistedNames(t *testing.T) {
	t.Parallel()
	parent := []string{
		"PATH=/usr/bin:/bin", "HARNESS_EVAL_SIGNING_KEY=c2VjcmV0", "HOME=/Users/runner", "GITHUB_TOKEN=" + exportTokenCanary,
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN=" + exportOIDCCanary, "ACTIONS_ID_TOKEN_REQUEST_URL=https://token.invalid",
		"CODEX_API_KEY=sk-canary", "GOOGLE_APPLICATION_CREDENTIALS=/creds.json", "TMPDIR=/tmp/runner",
		"GOCACHE=/cache", "GOMODCACHE=/mod", "GOFLAGS=-toolexec=/x", "GOPROXY=https://proxy.invalid",
		"PATHEXT=.exe", "NOEQUALS", "=hidden", "LANG=C.UTF-8", "GOTOOLCHAIN=local",
	}
	assert.Equal(t, []string{
		"PATH=/usr/bin:/bin", "HOME=/Users/runner", "TMPDIR=/tmp/runner", "GOCACHE=/cache", "GOMODCACHE=/mod",
		"LANG=C.UTF-8", "GOTOOLCHAIN=local",
	}, harnessChildEnv(parent))
	assert.Equal(t, []string{}, harnessChildEnv(nil), "no parent gives an empty, non-nil environment")
}

// exportGitTree is the standard golden set with the runner tree roots,
// committed and tagged v0.50.122, the manifest's baseline_ref.
func exportGitTree(t *testing.T) *harnessTree {
	t.Helper()
	tree := standardHarnessTree(t)
	tree.write("scripts/benchmarks/harness/golden.py", "print('golden')\n")
	tree.write("pkg/harneval/verdict.go", "package harneval\n")
	tree.write("cmd/harneval-oracle/main.go", "package main\n")
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "fixture"}, {"tag", "v0.50.122"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = tree.root
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=harneval", "GIT_AUTHOR_EMAIL=harneval@example.invalid",
			"GIT_COMMITTER_NAME=harneval", "GIT_COMMITTER_EMAIL=harneval@example.invalid"}
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return tree
}

// recordingWrapper writes a script that appends its environment to record
// and then runs target with the same arguments.
func recordingWrapper(t *testing.T, dir, name, target, record string) string {
	t.Helper()
	for _, value := range []string{dir, target, record} {
		require.NotContains(t, value, "'", "the script quotes paths with single quotes")
	}
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nenv >> '" + record + "'\nexec '" + target + "' \"$@\"\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

// assertCleanEnvironment checks a recorded environment without printing it:
// it never holds the key name or bytes, and a child's never holds a canary.
func assertCleanEnvironment(t *testing.T, priv ed25519.PrivateKey, where string, env []byte, child bool) {
	t.Helper()
	require.NotEmpty(t, env, where+" was recorded")
	leaks := []string{"HARNESS_EVAL_SIGNING_KEY"}
	if child {
		leaks = append(leaks, exportTokenCanary, exportOIDCCanary)
	}
	for _, leak := range leaks {
		assert.False(t, bytes.Contains(env, []byte(leak)), "%s holds %s", where, leak)
	}
	assertNoKey(t, priv, where, env)
}

// TestEvalHarnessExport_S2_ChildProcessesSeeOnlyTheAllowlist runs the
// production binding: git resolves the baseline tag under the allowlist, and
// the reconstruction receives the same allowlist. Serial: it changes the
// process environment, starts git, and generates.
func TestEvalHarnessExport_S2_ChildProcessesSeeOnlyTheAllowlist(t *testing.T) {
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	pub, priv, key := exportKey(t)
	tree := exportGitTree(t)
	bin, record := t.TempDir(), filepath.Join(t.TempDir(), "git-env")
	recordingWrapper(t, bin, "git", realGit, record)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HARNESS_EVAL_SIGNING_KEY", key)
	t.Setenv("GITHUB_TOKEN", exportTokenCanary)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", exportOIDCCanary)

	var request harnessExportRequest
	seams := exportSeams(t, exportSession("pass", "pass"), pub)
	seams.binding, seams.environ = nil, nil
	seams.reconstruct = func(_ context.Context, req harnessExportRequest) (harnessSignable, error) {
		request = req
		return harnessSignable{Session: exportSession("pass", "pass")}, nil
	}
	dir := tree.root
	deps := harnessDeps(harnessRouter)
	cmd := newEvalHarnessExportCmdWith(deps, &dir, seams)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(key))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--input", "unsigned", "--run-meta", "meta.json", "--output", filepath.Join(t.TempDir(), "evidence")})
	require.NoError(t, cmd.Execute(), stderr.String())

	recorded, err := os.ReadFile(record)
	require.NoError(t, err, "the binding resolved the baseline tag through git")
	assertCleanEnvironment(t, priv, "git child environment", recorded, true)
	assertCleanEnvironment(t, priv, "reconstruction environment", []byte(strings.Join(request.Env, "\n")), true)
	assert.Equal(t, harnessChildEnv(os.Environ()), request.Env)
	assert.Equal(t, tree.root, request.Root)

	want, err := harneval.ComputeBinding(context.Background(), tree.root, harneval.BindingOptions{Adapters: deps.run.Adapters})
	require.NoError(t, err)
	assert.Equal(t, want, request.Binding, "the reconstruction receives the trusted binding")
	assert.Equal(t, want.Digest(), request.BindingDigest)
	assert.Equal(t, "eval-regression: ok (version="+want.Digest()+")\n", stdout.String())
}

// withEnv is base without the overridden names, followed by the overrides.
func withEnv(base []string, overrides map[string]string) []string {
	env := []string{}
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[name]; !replaced {
			env = append(env, entry)
		}
	}
	for name, value := range overrides {
		env = append(env, name+"="+value)
	}
	return env
}

// TestEvalHarnessExport_S2_ExportPipelineKeepsTheKeyOutOfEveryEnvironment
// runs the REQ-HR-01 export command shape against a built binary: the shell
// reads HARNESS_EVAL_SIGNING_KEY and env -u removes the same name, so the
// key reaches auto on stdin alone. auto's own environment and its git child's
// are recorded. This build has no reconstruction, so the run refuses after
// the key was read and the binding was computed.
func TestEvalHarnessExport_S2_ExportPipelineKeepsTheKeyOutOfEveryEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an auto binary")
	}
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	_, priv, key := exportKey(t)
	tree := exportGitTree(t)
	auto := buildHarnessBinary(t, "v0.50.123")
	bin, records := t.TempDir(), t.TempDir()
	recordingWrapper(t, bin, "git", realGit, filepath.Join(records, "git-env"))
	wrapper := recordingWrapper(t, bin, "auto-recorded", auto, filepath.Join(records, "auto-env"))
	out := filepath.Join(t.TempDir(), "evidence")

	shell := exec.Command("/bin/sh", "-c", `printf '%s' "$HARNESS_EVAL_SIGNING_KEY" | env -u HARNESS_EVAL_SIGNING_KEY `+
		`"$EXPORT_AUTO" eval harness export --dir "$EXPORT_DIR" --input unsigned --run-meta meta.json --output "$EXPORT_OUT"`)
	shell.Env = withEnv(os.Environ(), map[string]string{
		"PATH": bin + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME": t.TempDir(),
		"HARNESS_EVAL_SIGNING_KEY": key, "GITHUB_TOKEN": exportTokenCanary, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": exportOIDCCanary,
		"EXPORT_AUTO": wrapper, "EXPORT_DIR": tree.root, "EXPORT_OUT": out,
	})
	output, err := shell.CombinedOutput()
	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr), "export must refuse in a build without the reconstruction: %s", output)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, string(output), "harness-eval: export refused: reconstruction_unavailable", "the key arrived on stdin")
	assertNoKey(t, priv, "export output", output)
	assert.NoDirExists(t, out)
	// auto itself keeps the step environment minus the key; its children get
	// the allowlist, so the token and OIDC canaries stop at auto.
	for name, child := range map[string]bool{"auto-env": false, "git-env": true} {
		recorded, err := os.ReadFile(filepath.Join(records, name))
		require.NoError(t, err, name)
		assertCleanEnvironment(t, priv, name, recorded, child)
	}
}
