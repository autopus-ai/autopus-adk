package cli

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestRunSpecReview_PrintsChecklistSummary(t *testing.T) {
	dir := t.TempDir()
	specDir := scaffoldReviewSpec(t, dir, "SPEC-REVIEW-CHECKLIST-001")
	setFakeProviderOnPath(t, dir, "claude")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		// Default config has 2 providers (claude, gemini); both respond so the
		// SPEC-SPECREV-001 dropped-provider rule does not downgrade verdict to REVISE.
		return &orchestra.OrchestraResult{
			Responses: []orchestra.ProviderResponse{
				{
					Provider: "claude",
					Output: `VERDICT: PASS
CHECKLIST: Q-CORR-01 | PASS
CHECKLIST: Q-COMP-03 | FAIL | acceptance.md에 error path 시나리오 부재
CHECKLIST: Q-STYLE-01 | PASS`,
				},
				{Provider: "gemini", Output: "VERDICT: PASS"},
			},
		}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	stdout := captureStdout(t, func() {
		require.NoError(t, runSpecReview(context.Background(), "SPEC-REVIEW-CHECKLIST-001", "consensus", 10))
	})

	// SPEC-SPECREV-001 follow-up: stdout summary now includes N/A count so
	// reviewers can distinguish "all dimensions evaluated" from "some waived".
	assert.Contains(t, stdout, "체크리스트 결과: 3건 (PASS: 2, FAIL: 1, N/A: 0)")
	assert.Contains(t, stdout, "- [FAIL] Q-COMP-03: acceptance.md에 error path 시나리오 부재")

	// SPEC-REVIEWPOLICY-001 REQ-RP-POL-03 (codex review F-001): a failing
	// checklist item blocks whatever the reviewer voted, as the canonical rule
	// line tells reviewers, so the PASS vote no longer approves the SPEC.
	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "draft", doc.Status)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	defer func() { os.Stdout = orig }()

	fn()

	require.NoError(t, writer.Close())
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(data)
}
