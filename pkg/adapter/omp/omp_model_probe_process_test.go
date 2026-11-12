package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMPModelProbeProcess_RunUsesPinnedCredentialFreeSandbox(t *testing.T) {
	secret := "OMP-MODEL-PROBE-SECRET-SENTINEL"
	t.Setenv("OMP_MODEL_PROBE_SECRET", secret)
	t.Setenv("OPENAI_API_KEY", secret)

	executable := writeOMPModelProbeExecutable(t, `#!/bin/sh
printf 'args=%s\n' "$*"
printf 'home=%s\n' "$HOME"
printf 'tmp=%s\n' "$TMPDIR"
printf 'config=%s\n' "$XDG_CONFIG_HOME"
printf 'data=%s\n' "$XDG_DATA_HOME"
printf 'state=%s\n' "$XDG_STATE_HOME"
printf 'cache=%s\n' "$XDG_CACHE_HOME"
printf 'lang=%s/%s\n' "$LANG" "$LC_ALL"
printf 'secret=%s/%s\n' "${OMP_MODEL_PROBE_SECRET-unset}" "${OPENAI_API_KEY-unset}"
printf 'cwd=%s\n' "$PWD"
`)
	process, err := NewOMPModelProbeProcess(executable, 4096)
	require.NoError(t, err)

	output, err := process.Run(t.Context(), "models", "--json")
	require.NoError(t, err)
	text := string(output)
	assert.Contains(t, text, "args=models --json\n")
	assert.Contains(t, text, "lang=C/C\n")
	assert.Contains(t, text, "secret=unset/unset\n")
	assert.NotContains(t, text, secret)
	for _, name := range []string{"home", "tmp", "config", "data", "state", "cache"} {
		line := ompModelProbeOutputValue(t, text, name)
		assert.Contains(t, line, "autopus-omp-model-probe-")
		assert.True(t, filepath.IsAbs(line))
	}
	assert.Contains(t, ompModelProbeOutputValue(t, text, "cwd"), "autopus-omp-model-probe-")
}

func TestOMPInstalledModelProbeProcess_RunUsesOnlyInstalledProfileLocators(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profile := filepath.Join(root, "profile")
	require.NoError(t, os.Mkdir(home, 0o700))
	require.NoError(t, os.Mkdir(profile, 0o700))
	canonicalHome, err := filepath.EvalSymlinks(home)
	require.NoError(t, err)
	canonicalProfile, err := filepath.EvalSymlinks(profile)
	require.NoError(t, err)
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", profile)
	t.Setenv("OPENAI_API_KEY", "blocked-api-key")
	t.Setenv("ANTHROPIC_API_KEY", "blocked-api-key")
	t.Setenv("OMP_ACCESS_TOKEN", "blocked-token")
	t.Setenv("AUTOPUS_GENERAL_ENV", "blocked-general")
	t.Setenv("PI_CONFIG_FILES", filepath.Join(root, "project.yml"))

	executable := writeOMPModelProbeExecutable(t, `#!/bin/sh
printf 'args=%s\n' "$*"
printf 'home=%s\n' "$HOME"
printf 'profile=%s\n' "$PI_CODING_AGENT_DIR"
printf 'tmp=%s\n' "$TMPDIR"
printf 'config=%s\n' "$XDG_CONFIG_HOME"
printf 'data=%s\n' "$XDG_DATA_HOME"
printf 'state=%s\n' "$XDG_STATE_HOME"
printf 'cache=%s\n' "$XDG_CACHE_HOME"
printf 'secrets=%s/%s/%s/%s/%s\n' "${OPENAI_API_KEY-unset}" "${ANTHROPIC_API_KEY-unset}" "${OMP_ACCESS_TOKEN-unset}" "${AUTOPUS_GENERAL_ENV-unset}" "${PI_CONFIG_FILES-unset}"
printf 'cwd=%s\n' "$PWD"
`)
	process, err := NewOMPInstalledModelProbeProcess(executable, 4096)
	require.NoError(t, err)
	output, err := process.Run(context.Background(), "models", "--json", "--no-extensions")
	require.NoError(t, err)

	text := string(output)
	assert.Contains(t, text, "args=models --json --no-extensions\n")
	assert.Equal(t, canonicalHome, ompModelProbeOutputValue(t, text, "home"))
	assert.Equal(t, canonicalProfile, ompModelProbeOutputValue(t, text, "profile"))
	assert.Equal(t, "unset/unset/unset/unset/unset", ompModelProbeOutputValue(t, text, "secrets"))
	assert.NotContains(t, text, "blocked-api-key")
	assert.NotContains(t, text, "blocked-token")
	assert.NotContains(t, text, "blocked-general")
	for _, name := range []string{"tmp", "config", "data", "state", "cache", "cwd"} {
		assert.Contains(t, ompModelProbeOutputValue(t, text, name), "autopus-omp-model-probe-")
	}
}

// omp 18.1.x `config get` reads overlays only from PI_CONFIG_FILES, so a
// `--config` readback must forward the same file through the environment
// while a plain metadata probe must not inherit the caller's overlay.
func TestOMPModelProbeProcess_ForwardsConfigOverlayThroughEnvironment(t *testing.T) {
	t.Setenv("PI_CONFIG_FILES", "/caller/overlay.yml")
	executable := writeOMPModelProbeExecutable(t, `#!/bin/sh
printf 'args=%s\n' "$*"
printf 'overlay=%s\n' "${PI_CONFIG_FILES-unset}"
`)
	process, err := NewOMPModelProbeProcess(executable, 4096)
	require.NoError(t, err)

	output, err := process.Run(context.Background(),
		"--config", "/tmp/routing.yml", "config", "get", "retry.fallbackChains", "--json")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/routing.yml", ompModelProbeOutputValue(t, string(output), "overlay"))

	output, err = process.Run(context.Background(), "models", "--json", "--no-extensions")
	require.NoError(t, err)
	assert.Equal(t, "unset", ompModelProbeOutputValue(t, string(output), "overlay"))
}

// omp 18.12.3 `models` refreshes stale catalog rows online before answering,
// so a probe that inherits open network access measures the operator's
// network instead of omp (one stalled hop: 10s wall, 0.8s CPU). Both
// constructors close remote access exactly as the readiness runner does,
// with loopback left open for local providers and the auth broker.
func TestOMPModelProbeProcess_RunClosesRemoteNetworkLikeReadinessProbes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("HTTPS_PROXY", "http://caller-proxy.invalid:3128")
	t.Setenv("NO_PROXY", "caller.invalid")
	executable := writeOMPModelProbeExecutable(t, `#!/bin/sh
printf 'http=%s\n' "${HTTP_PROXY-unset}"
printf 'https=%s\n' "${HTTPS_PROXY-unset}"
printf 'all=%s\n' "${ALL_PROXY-unset}"
printf 'noproxy=%s\n' "${NO_PROXY-unset}"
printf 'noproxylower=%s\n' "${no_proxy-unset}"
`)
	sandboxed, err := NewOMPModelProbeProcess(executable, 4096)
	require.NoError(t, err)
	installed, err := NewOMPInstalledModelProbeProcess(executable, 4096)
	require.NoError(t, err)

	for name, process := range map[string]*OMPModelProbeProcess{"sandboxed": sandboxed, "installed": installed} {
		output, runErr := process.Run(context.Background(), "models", "--json", "--no-extensions")
		require.NoError(t, runErr, name)
		text := string(output)
		for _, key := range []string{"http", "https", "all"} {
			assert.Equal(t, ompClosedProxy, ompModelProbeOutputValue(t, text, key), name+" "+key)
		}
		assert.Equal(t, "127.0.0.1,localhost,::1", ompModelProbeOutputValue(t, text, "noproxy"), name)
		assert.Equal(t, "127.0.0.1,localhost,::1", ompModelProbeOutputValue(t, text, "noproxylower"), name)
	}
	// The readiness runner is the other probe surface; the two must not drift.
	readiness := ompProbeTaskEnvironment(t.TempDir())
	isolation := ompProbeNetworkIsolationEnvironment()
	require.Len(t, isolation, 5)
	for _, entry := range isolation {
		assert.Contains(t, readiness, entry)
	}
}

func TestNewOMPInstalledModelProbeProcess_RejectsUnsafeProfileLocator(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", "relative-profile")

	process, err := NewOMPInstalledModelProbeProcess("/bin/sh", 1024)
	assert.Nil(t, process)
	assert.ErrorContains(t, err, "PI_CODING_AGENT_DIR locator is invalid")
}

func TestOMPModelProbeProcess_RunRejectsChangedOrUnpinnedIdentity(t *testing.T) {
	t.Parallel()

	var nilProcess *OMPModelProbeProcess
	output, err := nilProcess.Run(context.Background(), "--version")
	assert.Nil(t, output)
	assert.ErrorContains(t, err, "not pinned")

	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	executable := filepath.Join(dir, "omp-fixture")
	require.NoError(t, os.WriteFile(executable, []byte("#!/bin/sh\nprintf old > "+marker+"\n"), 0o700))
	process, err := NewOMPModelProbeProcess(executable, 1024)
	require.NoError(t, err)
	require.NoError(t, os.Rename(executable, executable+".old"))
	require.NoError(t, os.WriteFile(executable, []byte("#!/bin/sh\nprintf changed > "+marker+"\n"), 0o700))

	output, err = process.Run(context.Background(), "--version")
	assert.Nil(t, output)
	assert.ErrorContains(t, err, "identity changed")
	_, statErr := os.Stat(marker)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestNewOMPModelProbeProcess_RejectsInvalidExecutableContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		executable string
		maxOutput  int
		want       string
	}{
		{name: "empty executable", maxOutput: 1, want: "inputs are invalid"},
		{name: "zero output budget", executable: "/bin/sh", want: "inputs are invalid"},
		{name: "missing executable", executable: filepath.Join(t.TempDir(), "missing"), maxOutput: 1, want: "resolve"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			process, err := NewOMPModelProbeProcess(tc.executable, tc.maxOutput)
			assert.Nil(t, process)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func writeOMPModelProbeExecutable(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "omp-model-probe-fixture")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o700))
	return path
}

func ompModelProbeOutputValue(t *testing.T, output, key string) string {
	t.Helper()
	prefix := key + "="
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("missing %s output in %q", key, output)
	return ""
}
