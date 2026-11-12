package omp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ompRPCWaitDelay             = 250 * time.Millisecond
	ompClosedProxy              = "http://127.0.0.1:1"
	ompLiveOutboundControlScope = "os_level_loopback_only_exact_endpoint_deny_by_default"
)

type ompRPCNetworkSandboxEvidence struct {
	AllowedEndpointConnections int
	DeniedOtherLoopback        int
	DeniedNonLoopback          int
}

func TestOMPRPCNetworkSandbox_ExactEndpointContract(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evidence, err := probeOMPRPCNetworkSandbox(ctx, "http://"+listener.Addr().String())
	if runtime.GOOS != "darwin" {
		require.ErrorContains(t, err, "network sandbox unsupported")
		assert.Empty(t, evidence)
		return
	}
	require.NoError(t, err)
	assert.Equal(t, ompRPCNetworkSandboxEvidence{
		AllowedEndpointConnections: 1,
		DeniedOtherLoopback:        1,
		DeniedNonLoopback:          2,
	}, evidence)
}

func TestOMPRPCNetworkSandboxDialHelper(t *testing.T) {
	if os.Getenv("AUTOPUS_OMP_NETWORK_SANDBOX_HELPER") != "1" {
		return
	}

	allowed := os.Getenv("AUTOPUS_OMP_NETWORK_ALLOWED")
	otherLoopback := os.Getenv("AUTOPUS_OMP_NETWORK_OTHER_LOOPBACK")
	nonLoopback := os.Getenv("AUTOPUS_OMP_NETWORK_NON_LOOPBACK")
	testNet := os.Getenv("AUTOPUS_OMP_NETWORK_TEST_NET")
	for label, target := range map[string]string{
		"allowed":        allowed,
		"other_loopback": otherLoopback,
		"non_loopback":   nonLoopback,
		"test_net":       testNet,
	} {
		require.NotEmpty(t, target, label)
	}

	connection, err := net.DialTimeout("tcp", allowed, time.Second)
	require.NoError(t, err, "exact allowed endpoint must remain reachable")
	require.NoError(t, connection.Close())
	for label, target := range map[string]string{
		"other_loopback": otherLoopback,
		"non_loopback":   nonLoopback,
		"test_net":       testNet,
	} {
		connection, err = net.DialTimeout("tcp", target, time.Second)
		if err == nil {
			_ = connection.Close()
			t.Fatalf("%s counterexample unexpectedly reached a listener", label)
		}
		if !errors.Is(err, syscall.EPERM) && !errors.Is(err, syscall.EACCES) {
			t.Fatalf("%s counterexample failed without an OS sandbox denial", label)
		}
	}
}

func isolatedOMPLiveEnv(profile, overlay string) ([]string, error) {
	if !filepath.IsAbs(profile) || !filepath.IsAbs(overlay) {
		return nil, fmt.Errorf("live isolation paths must be absolute")
	}
	profileInfo, err := os.Stat(profile)
	if err != nil || !profileInfo.IsDir() {
		return nil, fmt.Errorf("live profile must be a directory")
	}
	overlayInfo, err := os.Lstat(overlay)
	if err != nil || !overlayInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("live overlay must be a regular file")
	}
	resolvedProfile, err := filepath.EvalSymlinks(profile)
	if err != nil {
		return nil, fmt.Errorf("resolve live profile: %w", err)
	}
	resolvedOverlay, err := filepath.EvalSymlinks(overlay)
	if err != nil || !pathWithinOMPTestRoot(resolvedProfile, resolvedOverlay) {
		return nil, fmt.Errorf("live overlay must be profile-owned")
	}

	directories := []string{
		filepath.Join(profile, "home"),
		filepath.Join(profile, "tmp"),
		filepath.Join(profile, "xdg", "cache"),
		filepath.Join(profile, "xdg", "config"),
		filepath.Join(profile, "xdg", "config-dirs"),
		filepath.Join(profile, "xdg", "data"),
		filepath.Join(profile, "xdg", "data-dirs"),
		filepath.Join(profile, "xdg", "runtime"),
		filepath.Join(profile, "xdg", "state"),
	}
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("create live isolation directory: %w", err)
		}
	}

	return []string{
		"HOME=" + filepath.Join(profile, "home"),
		"XDG_CACHE_HOME=" + filepath.Join(profile, "xdg", "cache"),
		"XDG_CONFIG_DIRS=" + filepath.Join(profile, "xdg", "config-dirs"),
		"XDG_CONFIG_HOME=" + filepath.Join(profile, "xdg", "config"),
		"XDG_DATA_DIRS=" + filepath.Join(profile, "xdg", "data-dirs"),
		"XDG_DATA_HOME=" + filepath.Join(profile, "xdg", "data"),
		"XDG_RUNTIME_DIR=" + filepath.Join(profile, "xdg", "runtime"),
		"XDG_STATE_HOME=" + filepath.Join(profile, "xdg", "state"),
		"TMPDIR=" + filepath.Join(profile, "tmp"),
		"PI_CODING_AGENT_DIR=" + profile,
		"PI_CONFIG_FILES=" + overlay,
		"PATH=" + os.Getenv("PATH"),
		"NO_PROXY=127.0.0.1,localhost,::1",
		"no_proxy=127.0.0.1,localhost,::1",
		"HTTP_PROXY=" + ompClosedProxy,
		"HTTPS_PROXY=" + ompClosedProxy,
		"ALL_PROXY=" + ompClosedProxy,
	}, nil
}

// ompLiveVersionProbeCeiling bounds the identity probe these live tests run
// before dialling. It is a variable because the assertion is on the shape of
// the version string, not on how fast omp prints it: a machine saturated by a
// parallel suite would otherwise preempt the probe and fail the test for load.
var ompLiveVersionProbeCeiling = 60 * time.Second

func probeOMPLiveVersion(t *testing.T, executable, profile, overlay string) string {
	t.Helper()
	isolatedEnv, err := isolatedOMPLiveEnv(profile, overlay)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), ompLiveVersionProbeCeiling)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "--version")
	cmd.Dir = profile
	cmd.Env = isolatedEnv
	cmd.WaitDelay = ompRPCWaitDelay
	require.NoError(t, configureOMPRPCNetworkSandbox(cmd, ompClosedProxy))
	require.NoError(t, configureOMPRPCProcessGroup(cmd))

	output, err := cmd.Output()
	reapOMPRPCProcessGroup(cmd)
	require.NoError(t, err)
	version := strings.TrimSpace(string(output))
	require.Regexp(t, regexp.MustCompile(`^omp/[0-9]+\.[0-9]+\.[0-9]+$`), version)
	return version
}

func TestIsolatedOMPLiveEnv_UsesMinimalTaskOwnedAllowlist(t *testing.T) {
	sentinel := "SEC_OMP002_005_PARENT_SENTINEL"
	for _, key := range []string{
		"OPENAI_API_KEY", "ANTHROPIC_TOKEN", "SESSION_COOKIE", "HTTP_PROXY",
		"PI_CONFIG_FILES", "NETRC", "SSH_AUTH_SOCK", "DOCKER_AUTH_CONFIG",
		"AWS_SHARED_CREDENTIALS_FILE", "GOOGLE_APPLICATION_CREDENTIALS",
	} {
		t.Setenv(key, sentinel)
	}
	profile := t.TempDir()
	overlay := filepath.Join(profile, "live-config.yml")
	require.NoError(t, os.WriteFile(overlay, []byte("skills: {}\n"), 0o600))

	env, err := isolatedOMPLiveEnv(profile, overlay)
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(env, "\n"), sentinel)

	values := make(map[string]string, len(env))
	keys := make([]string, 0, len(env))
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		require.True(t, ok)
		values[key] = value
		keys = append(keys, key)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{
		"ALL_PROXY", "HOME", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "PATH",
		"PI_CODING_AGENT_DIR", "PI_CONFIG_FILES", "TMPDIR", "XDG_CACHE_HOME",
		"XDG_CONFIG_DIRS", "XDG_CONFIG_HOME", "XDG_DATA_DIRS", "XDG_DATA_HOME",
		"XDG_RUNTIME_DIR", "XDG_STATE_HOME", "no_proxy",
	}, keys)
	assert.Equal(t, overlay, values["PI_CONFIG_FILES"])
	for _, key := range []string{
		"HOME", "PI_CODING_AGENT_DIR", "TMPDIR", "XDG_CACHE_HOME", "XDG_CONFIG_DIRS",
		"XDG_CONFIG_HOME", "XDG_DATA_DIRS", "XDG_DATA_HOME", "XDG_RUNTIME_DIR",
		"XDG_STATE_HOME",
	} {
		assert.True(t, pathWithinOMPTestRoot(profile, values[key]), key)
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		assert.Equal(t, ompClosedProxy, values[key])
	}
}

func TestIsolatedOMPLiveEnv_RejectsOverlayOutsideProfile(t *testing.T) {
	profile := t.TempDir()
	overlay := filepath.Join(t.TempDir(), "live-config.yml")
	require.NoError(t, os.WriteFile(overlay, []byte("skills: {}\n"), 0o600))

	_, err := isolatedOMPLiveEnv(profile, overlay)
	require.ErrorContains(t, err, "profile-owned")
}
