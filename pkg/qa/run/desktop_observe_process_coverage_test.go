package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Guards the cancellation contract of the resolver surface: once the caller's
// context is done, no resolver method may fall through to process work. A
// regression here would spawn or probe binaries after abort.
func TestProcessResolverRejectsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := &processDesktopProviderResolver{}

	_, localErr := resolver.ResolveLocal(ctx)
	assert.ErrorIs(t, localErr, context.Canceled)

	_, lookErr := resolver.LookPath(ctx, "sh")
	assert.ErrorIs(t, lookErr, context.Canceled)

	_, orcaErr := resolver.ResolveOrca(ctx, os.Args[0])
	assert.ErrorIs(t, orcaErr, context.Canceled)

	client, clientErr := resolver.NewOrcaClient(ctx, os.Args[0])
	assert.ErrorIs(t, clientErr, context.Canceled)
	assert.Nil(t, client)
}

// Guards that LookPath normalizes an exec.LookPath miss into the provider
// sentinel instead of leaking exec.ErrNotFound, which callers do not match on.
func TestProcessResolverLookPathNormalizesMiss(t *testing.T) {

	resolver := &processDesktopProviderResolver{}
	_, err := resolver.LookPath(context.Background(), "autopus-definitely-not-a-real-binary")
	assert.ErrorIs(t, err, errDesktopProviderUnavailable)
	assert.False(t, errors.Is(err, context.Canceled))

	t.Setenv("PATH", filepath.Dir(os.Args[0]))
	path, err := resolver.LookPath(context.Background(), filepath.Base(os.Args[0]))
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(os.Args[0]), filepath.Base(path))
}

// Guards ResolveOrca's admission rules: only an existing, absolute, executable
// regular file resolves. Directories and non-executable files must be refused
// so the runner never execs a path it cannot run.
func TestProcessResolverResolveOrcaAdmission(t *testing.T) {
	t.Parallel()

	resolver := &processDesktopProviderResolver{}
	dir := t.TempDir()

	_, missingErr := resolver.ResolveOrca(context.Background(), filepath.Join(dir, "absent"))
	assert.ErrorIs(t, missingErr, errDesktopProviderUnavailable)

	_, dirErr := resolver.ResolveOrca(context.Background(), dir)
	assert.ErrorIs(t, dirErr, errDesktopProviderUnavailable)

	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o644))
	_, plainErr := resolver.ResolveOrca(context.Background(), plain)
	assert.ErrorIs(t, plainErr, errDesktopProviderUnavailable)

	executable := filepath.Join(dir, "orca")
	require.NoError(t, os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	resolved, err := resolver.ResolveOrca(context.Background(), executable)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(resolved))
	assert.Equal(t, "orca", filepath.Base(resolved))
}

// Guards that a symlinked orca path resolves to its real target rather than
// being handed to exec as the link, which would defeat later identity pinning.
func TestProcessResolverResolveOrcaFollowsSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "real-orca")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	link := filepath.Join(dir, "link-orca")
	require.NoError(t, os.Symlink(target, link))

	resolver := &processDesktopProviderResolver{}
	resolved, err := resolver.ResolveOrca(context.Background(), link)
	require.NoError(t, err)

	realTarget, evalErr := filepath.EvalSymlinks(target)
	require.NoError(t, evalErr)
	assert.Equal(t, realTarget, resolved)
}

// Guards NewOrcaClient's file admission: a non-executable path must not yield
// a live client even though the context is healthy.
func TestProcessResolverNewOrcaClientRequiresExecutable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("x"), 0o644))

	resolver := &processDesktopProviderResolver{}
	client, err := resolver.NewOrcaClient(context.Background(), plain)
	assert.ErrorIs(t, err, errDesktopProviderUnavailable)
	assert.Nil(t, client)

	executable := filepath.Join(dir, "orca")
	require.NoError(t, os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	live, liveErr := resolver.NewOrcaClient(context.Background(), executable)
	require.NoError(t, liveErr)
	assert.NotNil(t, live)
}

// Guards desktopExecutableFile's mode test, the shared gate both orca paths
// depend on: directories and unset exec bits are not executables.
func TestDesktopExecutableFileModeGate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	assert.False(t, desktopExecutableFile(dir))
	assert.False(t, desktopExecutableFile(filepath.Join(dir, "missing")))

	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("x"), 0o600))
	assert.False(t, desktopExecutableFile(plain))

	require.NoError(t, os.Chmod(plain, 0o700))
	assert.True(t, desktopExecutableFile(plain))
}

// Guards the local-provider bundle layout check, which must reject anything
// that is not an absolute .app directory tree with an executable inside.
func TestDesktopLocalProviderExecutableRejectsBadLayout(t *testing.T) {
	t.Parallel()

	for name, path := range map[string]string{
		"empty":       "",
		"relative":    "relative/Bundle.app",
		"no-app-ext":  filepath.Join(t.TempDir(), "Bundle"),
		"nonexistent": filepath.Join(t.TempDir(), "Missing.app"),
	} {
		_, err := desktopLocalProviderExecutable(path)
		assert.ErrorIs(t, err, errDesktopProviderUnavailable, name)
	}

	// Complete tree except the executable bit, which must still be refused.
	bundle := filepath.Join(t.TempDir(), "Bundle.app")
	binDir := filepath.Join(bundle, "Contents", "MacOS")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(binDir, desktopProviderExecutableName), []byte("x"), 0o644,
	))
	_, err := desktopLocalProviderExecutable(bundle)
	assert.ErrorIs(t, err, errDesktopProviderUnavailable)

	require.NoError(t, os.Chmod(filepath.Join(binDir, desktopProviderExecutableName), 0o755))
	resolved, okErr := desktopLocalProviderExecutable(bundle)
	require.NoError(t, okErr)
	assert.Equal(t, filepath.Join(binDir, desktopProviderExecutableName), resolved)
}

// Guards the nil-transport guard on RoundTrip: a zero transport must fail
// closed instead of dereferencing its command fields.
func TestProcessTransportRoundTripNilFailsClosed(t *testing.T) {
	t.Parallel()

	var transport *processDesktopEnvelopeTransport
	body, err := transport.RoundTrip(context.Background(), []byte(`{}`))
	assert.ErrorIs(t, err, errDesktopProviderUnavailable)
	assert.Nil(t, body)
}

// Guards that ResolveLocal refuses when no signed artifact path is configured
// in either the options or the environment override.
func TestResolveLocalWithoutArtifactPath(t *testing.T) {
	t.Setenv(desktopSignedArtifactEnvironment, "")

	resolver := &processDesktopProviderResolver{}
	client, err := resolver.ResolveLocal(context.Background())
	assert.ErrorIs(t, err, errDesktopProviderUnavailable)
	assert.Nil(t, client)
}

// Guards that the environment override is actually consulted and still passes
// through the same layout admission instead of being trusted blindly.
func TestResolveLocalRejectsEnvironmentArtifactWithBadLayout(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "Bundle.app")
	require.NoError(t, os.MkdirAll(bundle, 0o755))
	t.Setenv(desktopSignedArtifactEnvironment, bundle)

	resolver := &processDesktopProviderResolver{}
	_, err := resolver.ResolveLocal(context.Background())
	assert.ErrorIs(t, err, errDesktopProviderUnavailable)
}
