package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/content"
)

// harnessProbeAgent is a one-agent content/ tree; the templates/ generated
// from it make the production template check start clean.
const harnessProbeAgent = "---\nname: probe\ndescription: probes the harness\n---\n\n# Probe\n\nInspect the tree.\n"

// harnessProducedAt matches the one time-dependent field of a result.
var harnessProducedAt = regexp.MustCompile(`"produced_at": "[^"]*"`)

// buildHarnessBinary builds cmd/auto with -trimpath and the given version
// ldflag into a directory of its own.
func buildHarnessBinary(t *testing.T, version string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "auto")
	build := exec.Command("go", "build", "-trimpath", "-ldflags",
		"-X github.com/insajin/autopus-adk/pkg/version.version="+version, "-o", binary, "./cmd/auto")
	build.Dir = moduleRootForTest(t)
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	return binary
}

// execHarnessBinary runs a built binary from its own directory with the given
// environment overrides and returns what the process wrote and its exit code.
func execHarnessBinary(t *testing.T, binary string, env map[string]string, args ...string) harnessOutcome {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = filepath.Dir(binary)
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else {
		require.NoError(t, err)
	}
	return harnessOutcome{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// TestEvalHarnessRun_S2_TwoBinariesAgreeAcrossBuildsAndHosts is the S2
// two-binary oracle on the production pipeline (template check, pinned
// adapters, sentinel): binaries built apart with version v0.50.123 and dev,
// run from different directories under different HOME, TMPDIR, and TZ, write
// results that are byte-identical once produced_at is removed, with one
// 64-hex surface digest and an empty sentinel log. Timestamped bookkeeping in
// the real surface would break the equality if it reached the digest.
func TestEvalHarnessRun_S2_TwoBinariesAgreeAcrossBuildsAndHosts(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two auto binaries")
	}
	tree := standardHarnessTree(t)
	tree.write("content/agents/probe.md", harnessProbeAgent)
	require.NoError(t, content.GenerateAllTemplates(filepath.Join(tree.root, "content"), filepath.Join(tree.root, "templates")))
	release, dev := buildHarnessBinary(t, "v0.50.123"), buildHarnessBinary(t, "dev")
	host := func(tz string) map[string]string {
		return map[string]string{"HOME": t.TempDir(), "TMPDIR": t.TempDir(), "TZ": tz}
	}
	versions := []string{
		strings.TrimSpace(execHarnessBinary(t, release, nil, "version", "--short").stdout),
		strings.TrimSpace(execHarnessBinary(t, dev, nil, "version", "--short").stdout),
	}
	require.Equal(t, []string{"v0.50.123", "dev"}, versions, "the builds differ in their version")
	pinned := execHarnessBinary(t, release, host("UTC"), "eval", "harness", "baseline", "--init", "--dir", tree.root)
	require.Equal(t, 0, pinned.code, pinned.stderr)

	var documents []string
	var digests []any
	for _, run := range []struct{ binary, tz string }{{release, "UTC"}, {dev, "Asia/Seoul"}} {
		got := execHarnessBinary(t, run.binary, host(run.tz), "eval", "harness", "run", "--format", "json", "--dir", tree.root)

		require.Equal(t, 0, got.code, got.stderr)
		doc := harnessDoc(t, got.stdout)
		assert.Equal(t, "pass", doc["status"])
		assert.Regexp(t, `^[0-9a-f]{64}$`, doc["surface_digest"])
		assert.NotContains(t, got.stderr, "sentinel:", "no host CLI was probed")
		documents = append(documents, harnessProducedAt.ReplaceAllString(got.stdout, `"produced_at": ""`))
		digests = append(digests, doc["surface_digest"])
	}
	assert.Equal(t, documents[0], documents[1], "byte-identical once produced_at is removed")
	assert.Equal(t, digests[0], digests[1])
}
