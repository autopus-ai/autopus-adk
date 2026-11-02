package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-REVIEWPOLICY-001 S7 regression: on 109c7ea4 an explicit --providers on
// a lean SPEC switched the whole lean execution policy off, so the configured
// judge still ran beside the one named reviewer and the quorum stayed at the
// configured majority. Provider selection must not change the profile.
func TestRunSpecReview_ProvidersOverrideKeepsTheLeanPolicy(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	writeGPTReviewDocuments(t, fixture.root)
	path := filepath.Join(fixture.specDir, "spec.md")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	lean := "status: draft\nprofile: lean\nchange_class: test_only\nsurface: internal/cli/spec_timing_test.go\n"
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(body), "status: draft\n", lean, 1)), 0o644))
	backend := fixture.useFakeBackend(t)

	require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{providers: []string{"codex"}}))

	assert.Equal(t, 1, backend.calls("codex", "reviewer"))
	assert.Zero(t, backend.calls("claude", "judge"), "a lean review runs no judge, --providers or not")
	assert.Equal(t, 1, backend.total())
}
