package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specFingerprintGolden was computed outside Go from the rule itself: the
// documents below with their header status lines removed, each preceded by
// "\n--- <name>\n", research.md missing and so empty. The review receipt
// writer and status must keep producing it.
const specFingerprintGolden = "sha256:52b60a343407ed83778897936dab018fa0e8007767b89f6bac10acf3a9367466"

func writeFingerprintFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"spec.md": "---\nid: SPEC-FP-001\nstatus: approved\n---\n# SPEC-FP-001: fingerprint fixture\n\n" +
			"**Status**: draft\n\n## Requirements\n\nStatus: this body line stays.\nThe fixture works.\n",
		"plan.md":       "# Plan\n\nStatus: draft\n\n## Tasks\n\n- T1\n",
		"acceptance.md": "# Acceptance\n\nGiven the fixture, then it works.\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	return dir
}

func TestSpecDocumentsFingerprint_PinsTheRule(t *testing.T) {
	t.Parallel()
	dir := writeFingerprintFixture(t)
	fingerprint := func() string {
		t.Helper()
		value, err := specDocumentsFingerprint(dir)
		require.NoError(t, err)
		return value
	}
	assert.Equal(t, specFingerprintGolden, fingerprint())

	require.NoError(t, os.WriteFile(filepath.Join(dir, "research.md"), nil, 0o644))
	assert.Equal(t, specFingerprintGolden, fingerprint(), "a missing document hashes as an empty one")

	spec := filepath.Join(dir, "spec.md")
	body, err := os.ReadFile(spec)
	require.NoError(t, err)
	promoted := strings.NewReplacer("status: approved", "status: completed", "**Status**: draft", "**Status**: done").
		Replace(string(body))
	require.NoError(t, os.WriteFile(spec, []byte(promoted), 0o644))
	assert.Equal(t, specFingerprintGolden, fingerprint(), "status lines never change the fingerprint")

	require.NoError(t, os.WriteFile(spec, []byte(strings.Replace(promoted,
		"Status: this body line stays.", "Status: a body line is a requirement.", 1)), 0o644))
	assert.NotEqual(t, specFingerprintGolden, fingerprint(), "a status-like line after the first H2 is content")

	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		require.NoError(t, os.Chmod(spec, 0o000))
		t.Cleanup(func() { _ = os.Chmod(spec, 0o644) })
		_, err := specDocumentsFingerprint(dir)
		assert.Error(t, err, "a document that exists but cannot be read is an error, not empty")
	}
}
