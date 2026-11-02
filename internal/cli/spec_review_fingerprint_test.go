package cli

// SPEC-REVIEWREUSE-001 S8, S10, S11 (REQ-RR-REUSE-01, REQ-RR-REUSE-03,
// REQ-RR-REUSE-04): the receipt fingerprint names every input, a required
// reference change invalidates it, only complete undegraded results are
// reusable, and only a version-naming model selector is pinned.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	sum := sha256.Sum256(readFixtureFile(t, path))
	return hex.EncodeToString(sum[:])
}

func TestRunSpecReview_ReceiptFingerprintNamesEveryInput(t *testing.T) {
	fixture, _ := reuseFixture(t, config.ReviewBudgetConf{}, nil)
	before, err := spec.ReviewDocumentDigests(fixture.specDir)
	require.NoError(t, err)

	runReviewOutput(t, fixture, specReviewOptions{})

	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.Equal(t, "approved", receipt.CurrentStatus)
	fp := receipt.Fingerprint
	require.NotNil(t, fp)
	assert.Equal(t, specReviewFingerprintSchema, fp.Schema)
	assert.True(t, fp.Complete, fp.Incomplete)
	assert.Equal(t, before, fp.Documents, "the promotion does not change the spec.md digest")
	head, err := exec.Command("git", "-C", fixture.root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(head)), fp.Code.Head)
	assert.Len(t, fp.Code.Worktree, 64)
	assert.Len(t, fp.Checklist, 64)
	assert.Len(t, fp.Contract, 64)
	require.Len(t, fp.Providers, 2)
	for _, provider := range fp.Providers {
		assert.True(t, provider.Pinned, provider.Name)
		assert.Equal(t, specReviewRoleReviewer, provider.Role)
	}
	assert.Equal(t, fileSHA256(t, filepath.Join(fixture.specDir, "review-findings.json")), fp.Evidence.ReviewFindings)
	assert.Equal(t, fileSHA256(t, filepath.Join(fixture.specDir, "review.md")), fp.Evidence.ReviewMD)
}

func TestRunSpecReview_FingerprintOutsideGitIsIncomplete(t *testing.T) {
	fixture, _ := budgetFixture(t, config.ReviewBudgetConf{}, nil)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(fixture.root))

	runReviewOutput(t, fixture, specReviewOptions{})

	fp := readSpecReviewReceipt(t, fixture.specDir).Fingerprint
	require.NotNil(t, fp)
	assert.False(t, fp.Complete)
	assert.Contains(t, fp.Incomplete, "code: not a git work tree")
}

func TestRunSpecReview_ChangedRequiredDocumentInvalidates(t *testing.T) {
	fixture, backend := reuseFixture(t, config.ReviewBudgetConf{}, nil)
	require.NoError(t, os.MkdirAll(filepath.Join(fixture.root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.root, "docs", "ref.md"), []byte("# Ref\n"), 0o644))
	commitFixtureRepo(t, fixture.root, "ref")
	backend.replies["claude/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	opts := specReviewOptions{requiredDocuments: []string{"docs/ref.md"}}
	runReviewOutput(t, fixture, opts)
	runReviewOutput(t, fixture, opts)
	require.Equal(t, 2, backend.count(), "unchanged reference: reused")

	appendFixtureFile(t, filepath.Join(fixture.root, "docs", "ref.md"), "More.\n")
	_, stderr := runReviewOutput(t, fixture, opts)

	assert.Equal(t, 4, backend.count())
	assert.Contains(t, stderr, "references: docs/ref.md")
}

func TestReusableSpecReviewResult_OnlyCompleteUndegradedResults(t *testing.T) {
	t.Parallel()

	receipt := func(verdict, gate, loop string, degraded []string, blocking ...string) specReviewPromotionReceipt {
		r := specReviewPromotionReceipt{Verdict: verdict, GateStatus: gate, LoopStatus: loop, DegradedReasons: degraded}
		for _, policy := range blocking {
			r.BlockingReasons = append(r.BlockingReasons, spec.BlockingReason{Policy: policy})
		}
		return r
	}
	cases := []struct {
		name   string
		r      specReviewPromotionReceipt
		state  string
		reused bool
	}{
		{"pass", receipt("PASS", "passed", "converged", nil), "converged", true},
		{"pass with quorum gap", receipt("PASS", "passed", "converged", []string{"provider_quorum"}), "", false},
		{"pass degraded gate", receipt("PASS", "degraded", "converged", nil), "", false},
		{"reject", receipt("REJECT", "blocked", "revisions_exhausted", nil, "provider_reject"), "revisions_exhausted", true},
		{"reject on budget", receipt("REJECT", "blocked", "budget_exhausted", nil, "provider_reject"), "", false},
		{"revise awaiting", receipt("REVISE", "blocked", "awaiting_changes", nil, "open_correctness_finding"), "awaiting_changes", true},
		{"revise exhausted", receipt("REVISE", "blocked", "revisions_exhausted", nil, "open_blocking_finding"), "awaiting_changes", true},
		{"revise provider outage", receipt("REVISE", "blocked", "provider_unavailable", nil, "no_usable_review"), "", false},
		{"revise without usable review", receipt("REVISE", "blocked", "revisions_exhausted", nil, "no_usable_review"), "", false},
	}
	for _, tc := range cases {
		state, ok := reusableSpecReviewResult(tc.r)
		assert.Equal(t, tc.reused, ok, tc.name)
		assert.Equal(t, tc.state, state, tc.name)
	}
}

func TestSpecReviewModelSelectorPinned(t *testing.T) {
	t.Parallel()

	cases := []struct {
		provider orchestra.ProviderConfig
		selector string
		pinned   bool
	}{
		{orchestra.ProviderConfig{Args: []string{"--print", "--model", "claude-opus-5-5"}}, "claude-opus-5-5", true},
		{orchestra.ProviderConfig{Args: []string{"exec", "-m", "gpt-6-astra"}}, "gpt-6-astra", true},
		{orchestra.ProviderConfig{Args: []string{"--model=gemini-3.8-flash"}}, "gemini-3.8-flash", true},
		{orchestra.ProviderConfig{Model: "anthropic/claude-sonnet-5-5:high", Args: []string{"--model", "opus"}}, "anthropic/claude-sonnet-5-5:high", true},
		{orchestra.ProviderConfig{Args: []string{"--model", "opus"}}, "opus", false},
		{orchestra.ProviderConfig{Args: []string{"--model", "gpt-6-latest"}}, "gpt-6-latest", false},
		{orchestra.ProviderConfig{Args: []string{"--print"}}, "", false},
		{orchestra.ProviderConfig{Model: "default"}, "default", false},
	}
	for _, tc := range cases {
		selector := specReviewModelSelector(tc.provider)
		assert.Equal(t, tc.selector, selector)
		assert.Equal(t, tc.pinned, specReviewModelPinned(selector), selector)
	}
}
