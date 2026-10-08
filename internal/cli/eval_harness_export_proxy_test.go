package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// exportWithArgs runs the production export over the world with extra flags.
func (w *reconstructWorld) exportWithArgs(t *testing.T, sources harnessTrustSources, extra ...string) harnessOutcome {
	t.Helper()
	pub, _, key := exportKey(t)
	seams := exportSeams(t, nil, pub)
	seams.reconstruct, seams.binding, seams.sources = nil, nil, sources
	seams.now = func() time.Time { return reconstructStart.Add(30 * time.Hour) }
	cmd := newEvalHarnessExportCmdWith(w.deps, &w.root, seams)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(key))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"--input", w.input, "--run-meta", w.meta, "--output", filepath.Join(t.TempDir(), "evidence")}, extra...))
	code := 0
	if err := cmd.Execute(); err != nil {
		code = exitCodeForError(err)
	}
	return harnessOutcome{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// TestEvalHarnessExport_Proxy_IsAFileURLHandedToTheBaselineRebuild: --proxy
// names the file:// module proxy the trusted module step filled, and the
// baseline arm rebuild receives it; no other GOPROXY value is accepted, so
// the signer's children never reach the network.
func TestEvalHarnessExport_Proxy_IsAFileURLHandedToTheBaselineRebuild(t *testing.T) {
	world := newReconstructWorld(t, 1)
	var got []string
	sources := world.sources
	rebuild := sources.baselineSurface
	sources.baselineSurface = func(ctx context.Context, req harnessExportRequest) (string, error) {
		got = append(got, req.Proxy)
		return rebuild(ctx, req)
	}
	proxy := "file://" + filepath.Join(t.TempDir(), "modules", "cache", "download")

	outcome := world.exportWithArgs(t, sources, "--proxy", proxy)

	require.Equal(t, 0, outcome.code, outcome.stderr)
	assert.Equal(t, []string{proxy}, got)
	for _, bad := range []string{"https://proxy.golang.org", "off", "direct", "file://relative/download",
		"file:///abs/download,https://proxy.golang.org", "file:///abs/download?x=1", "file://host/abs/download", "file:///"} {
		outcome := world.exportWithArgs(t, sources, "--proxy", bad)
		assert.Equal(t, 1, outcome.code, bad)
		assert.Contains(t, outcome.stderr, "--proxy must be a file:// URL of an absolute directory", bad)
	}
	assert.Len(t, got, 1, "a refused --proxy reaches no rebuild")
}

// TestEvalHarnessExport_DefaultBaselineSurface_DownloadsFromTheGivenProxy:
// the production rebuild fetches the baseline arm's modules from req.Proxy,
// not from the local module cache (macOS: the rebuild needs sandbox-exec).
func TestEvalHarnessExport_DefaultBaselineSurface_DownloadsFromTheGivenProxy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("harneval.ArmSurfaceDigest rebuilds on macOS only")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on PATH")
	}
	t.Parallel()
	root := t.TempDir()
	writeReconstructFile(t, filepath.Join(root, "go.mod"), []byte("module example.com/arm\n\ngo 1.26\n\nrequire example.com/dep v1.0.0\n"))
	writeReconstructFile(t, filepath.Join(root, "go.sum"), []byte("example.com/dep v1.0.0/go.mod h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"))
	writeReconstructFile(t, filepath.Join(root, filepath.FromSlash(harneval.SurfaceDriverPackage), "main.go"), []byte("package main\n\nfunc main() {}\n"))
	commit := gitCommitTree(t, root)
	proxy := "file://" + filepath.Join(t.TempDir(), "empty-proxy-7f3e")
	req := harnessExportRequest{Root: root, Proxy: proxy, Env: []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()},
		Binding: harneval.Binding{BaselineCommit: commit, Pins: harneval.Pins{GeneratorVersion: "v0.50.123", ProjectName: "p",
			CodexCLIVersion: "c", OpencodeCLIVersion: "o"}}}

	_, err := harnessTrustSources{}.withDefaults().baselineSurface(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "baseline_surface_failed: ")
	assert.Contains(t, err.Error(), "empty-proxy-7f3e", "the download read the given proxy")
}

// gitCommitTree commits every file of root in a new repository with a fixed
// identity and no user config, and returns the commit.
func gitCommitTree(t *testing.T, root string) string {
	t.Helper()
	var out []byte
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "arm"}, {"rev-parse", "HEAD"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=harneval", "GIT_AUTHOR_EMAIL=harneval@example.invalid",
			"GIT_COMMITTER_NAME=harneval", "GIT_COMMITTER_EMAIL=harneval@example.invalid"}
		var err error
		out, err = cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return strings.TrimSpace(string(out))
}
