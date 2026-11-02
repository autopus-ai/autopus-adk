package spec_test

// SPEC-REVIEWREUSE-001 S8 (REQ-RR-REUSE-01): the document digests cover every
// SPEC document except review.md, survive the review's own status promotion,
// and change with any other edit; the prompt contract is stable.

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestReviewDocumentDigests_IgnoreTheStatusPromotionOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	write("spec.md", "# SPEC-FP-001: Fingerprint\n\n**Status**: draft\n\n## Requirements\n\nBody.\n")
	write("plan.md", "# Plan\n")
	write("review.md", "# Review\n")
	write("notes.txt", "not a document\n")

	before, err := spec.ReviewDocumentDigests(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"plan.md", "spec.md"}, slices.Sorted(maps.Keys(before)))
	for _, digest := range before {
		assert.Len(t, digest, 64)
	}

	require.NoError(t, spec.UpdateStatus(dir, "approved"))
	write("review.md", "# Review, rewritten\n")
	after, err := spec.ReviewDocumentDigests(dir)
	require.NoError(t, err)
	assert.Equal(t, before, after)

	write("spec.md", "# SPEC-FP-001: Fingerprint\n\n**Status**: approved\n\n## Requirements\n\nBody changed.\n")
	changed, err := spec.ReviewDocumentDigests(dir)
	require.NoError(t, err)
	assert.NotEqual(t, before["spec.md"], changed["spec.md"])
	assert.Equal(t, before["plan.md"], changed["plan.md"])
}

func TestReviewPromptContract_IsStable(t *testing.T) {
	t.Parallel()

	first := spec.ReviewPromptContract()
	assert.Equal(t, first, spec.ReviewPromptContract())
	assert.Contains(t, first, "Instructions (Verify Mode)")
	assert.Contains(t, first, "## SPEC: SPEC-PROBE-001")
}
