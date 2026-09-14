package promptlayer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Required context must fail closed instead of silently degrading to an
// empty/truncated layer, and optional context must never escape the root.

func TestLoadContextLayer_RequiredFailsClosedOnMissingEmptyAndTruncated(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	_, err := promptlayer.LoadContextLayer(root, "docs/absent.md", promptlayer.ContextOptions{Required: true})
	require.ErrorContains(t, err, "required context is missing")

	require.NoError(t, os.WriteFile(filepath.Join(root, "blank.md"), []byte("   \n\t\n"), 0o600))
	_, err = promptlayer.LoadContextLayer(root, "blank.md", promptlayer.ContextOptions{Required: true})
	require.ErrorContains(t, err, "required context is empty")

	require.NoError(t, os.WriteFile(filepath.Join(root, "big.md"), []byte(strings.Repeat("payload\n", 64)), 0o600))
	_, err = promptlayer.LoadContextLayer(root, "big.md", promptlayer.ContextOptions{Required: true, MaxBytes: 16})
	require.ErrorContains(t, err, "was truncated")

	// The same document is admitted, truncated and flagged, when it is optional.
	layer, err := promptlayer.LoadContextLayer(root, "big.md", promptlayer.ContextOptions{MaxBytes: 16})
	require.NoError(t, err)
	assert.Equal(t, promptlayer.RedactionRedacted, layer.RedactionStatus)
	assert.Contains(t, layer.InvalidationReason, promptlayer.InvalidationSizeCap)
	assert.False(t, layer.CacheEligible)
	assert.LessOrEqual(t, len(layer.Content), 16)
}

func TestLoadContextLayer_TruncationStopsOnRuneBoundary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// "é" is two bytes: a byte-exact cut at 5 would split the third rune.
	require.NoError(t, os.WriteFile(filepath.Join(root, "utf8.md"), []byte(strings.Repeat("é", 10)), 0o600))

	layer, err := promptlayer.LoadContextLayer(root, "utf8.md", promptlayer.ContextOptions{MaxBytes: 5})
	require.NoError(t, err)
	assert.True(t, utf8.ValidString(layer.Content), "truncation must not emit a partial rune")
	assert.Equal(t, strings.Repeat("é", 2), layer.Content)
	assert.Contains(t, layer.InvalidationReason, promptlayer.InvalidationSizeCap)
}

func TestLoadContextLayer_UnreadableAndUnresolvablePathsError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "dir.md"), 0o700))
	_, err := promptlayer.LoadContextLayer(root, "dir.md", promptlayer.ContextOptions{})
	require.Error(t, err, "a directory standing in for a context file must not read as content")

	_, err = promptlayer.LoadContextLayer(filepath.Join(root, "no-such-root"), "a.md", promptlayer.ContextOptions{})
	require.ErrorContains(t, err, "context root unavailable")

	require.NoError(t, os.Symlink(filepath.Join(root, "nowhere.md"), filepath.Join(root, "dangling.md")))
	_, err = promptlayer.LoadContextLayer(root, "dangling.md", promptlayer.ContextOptions{})
	require.ErrorContains(t, err, "symlink is unavailable")
}

func TestLoadContextLayer_RejectsEscapeThroughSymlinkedParent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked")))

	// The leaf does not exist, so containment is decided on the resolved parent.
	_, err := promptlayer.LoadContextLayer(root, "linked/absent.md", promptlayer.ContextOptions{})
	require.ErrorContains(t, err, "escapes root")

	require.NoError(t, os.WriteFile(filepath.Join(outside, "present.md"), []byte("outside body"), 0o600))
	_, err = promptlayer.LoadContextLayer(root, "linked/present.md", promptlayer.ContextOptions{})
	require.ErrorContains(t, err, "escapes root")
}
