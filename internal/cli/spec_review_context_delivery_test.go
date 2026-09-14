package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

func TestRunSpecReview_GPTPromptConsumesVerifiedContextExactlyOnce(t *testing.T) {
	root, specID, markers := writeGPTReviewContextProject(t)
	extraRef := "docs/review-task.md"
	extraMarker := "REVIEW_TASK_EXTRA_TAIL"
	extraBody := "REVIEW_TASK_EXTRA_HEAD\n" + strings.Repeat("task-specific review evidence\n", 1400) + extraMarker
	writeCLIReviewContextFile(t, root, extraRef, extraBody)
	markers[extraRef] = extraMarker
	expectedHashes := expectedCLIReviewContextHashes(t, root, markers)
	restoreWD := chdirForSpecReviewTest(t, root)
	defer restoreWD()
	restoreProviders := stubGPTSpecReviewProviders()
	defer restoreProviders()

	var capturedPrompt string
	providerCalls := 0
	originalRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		providerCalls++
		capturedPrompt = cfg.Prompt
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{
			Provider: "codex", Output: "VERDICT: PASS",
		}}}, nil
	}
	defer func() { specReviewRunOrchestra = originalRunner }()

	err := runSpecReviewWithOptions(context.Background(), specID, "consensus", 10, specReviewOptions{
		requiredDocuments: []string{extraRef, "ARCHITECTURE.md"},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, providerCalls)
	assert.Contains(t, capturedPrompt, "GPT_ARCHITECTURE_HEAD")
	assert.Contains(t, capturedPrompt, "REVIEW_TASK_EXTRA_HEAD")
	for ref, marker := range markers {
		assert.Equalf(t, 1, strings.Count(capturedPrompt, marker), "document %s must be delivered once", ref)
		assert.Contains(t, capturedPrompt, "source_ref="+ref)
		assert.Contains(t, capturedPrompt, "source_sha256="+expectedHashes[ref][0])
		assert.Contains(t, capturedPrompt, "prompt_sha256="+expectedHashes[ref][1])
	}
}

func TestRunSpecReview_GPTMissingRequiredDocumentStopsBeforeProvider(t *testing.T) {
	root, specID, _ := writeGPTReviewContextProject(t)
	restoreWD := chdirForSpecReviewTest(t, root)
	defer restoreWD()
	restoreProviders := stubGPTSpecReviewProviders()
	defer restoreProviders()

	providerCalls := 0
	originalRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		providerCalls++
		return &orchestra.OrchestraResult{}, nil
	}
	defer func() { specReviewRunOrchestra = originalRunner }()

	err := runSpecReviewWithOptions(context.Background(), specID, "consensus", 10, specReviewOptions{
		requiredDocuments: []string{"docs/missing-extra.md"},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "missing-extra.md")
	assert.Zero(t, providerCalls)
}

func TestRunSpecReview_GPTTamperedReceiptStopsBeforeProvider(t *testing.T) {
	root, specID, _ := writeGPTReviewContextProject(t)
	restoreWD := chdirForSpecReviewTest(t, root)
	defer restoreWD()
	restoreProviders := stubGPTSpecReviewProviders()
	defer restoreProviders()

	originalBuilder := specReviewBuildContextDelivery
	specReviewBuildContextDelivery = func(opts promptlayer.ContextDeliveryOptions) (promptlayer.ContextDeliveryResult, error) {
		receipt, err := originalBuilder(opts)
		receipt.SnapshotHash = "sha256:" + strings.Repeat("0", 64)
		return receipt, err
	}
	defer func() { specReviewBuildContextDelivery = originalBuilder }()
	providerCalls := 0
	originalRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		providerCalls++
		return &orchestra.OrchestraResult{}, nil
	}
	defer func() { specReviewRunOrchestra = originalRunner }()

	err := runSpecReviewWithOptions(context.Background(), specID, "consensus", 10, specReviewOptions{})

	require.Error(t, err)
	assert.ErrorContains(t, err, "context integrity failed")
	assert.Zero(t, providerCalls)
}

func TestRunSpecReview_GPTOmittedRequiredSetStopsBeforeProvider(t *testing.T) {
	root, specID, _ := writeGPTReviewContextProject(t)
	extraRef := "docs/set-bound-extra.md"
	writeCLIReviewContextFile(t, root, extraRef, "SET_BOUND_EXTRA")
	restoreWD := chdirForSpecReviewTest(t, root)
	defer restoreWD()
	restoreProviders := stubGPTSpecReviewProviders()
	defer restoreProviders()

	originalBuilder := specReviewBuildContextDelivery
	specReviewBuildContextDelivery = func(opts promptlayer.ContextDeliveryOptions) (promptlayer.ContextDeliveryResult, error) {
		weakened := opts
		weakened.RequiredReferences = nil
		return originalBuilder(weakened)
	}
	defer func() { specReviewBuildContextDelivery = originalBuilder }()
	providerCalls := 0
	originalRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		providerCalls++
		return &orchestra.OrchestraResult{}, nil
	}
	defer func() { specReviewRunOrchestra = originalRunner }()

	err := runSpecReviewWithOptions(context.Background(), specID, "consensus", 10, specReviewOptions{
		requiredDocuments: []string{extraRef},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "incomplete, or tampered")
	assert.Zero(t, providerCalls)
}

func TestPrepareSpecReviewContextDelivery_NormalizesSetsAndProfiles(t *testing.T) {
	t.Parallel()

	root, specID, _ := writeGPTReviewContextProject(t)
	signatureRef := ".autopus/context/signatures.md"
	writeCLIReviewContextFile(t, root, signatureRef, "SIGNATURE_CONTEXT")
	for _, ref := range []string{"docs/a.md", "docs/b.md"} {
		writeCLIReviewContextFile(t, root, ref, ref)
	}
	specDir := filepath.Join(root, ".autopus", "specs", specID)
	providers := []orchestra.ProviderConfig{{Name: "codex"}}
	first, err := prepareSpecReviewContextDelivery(specDir, providers, specReviewOptions{
		requiredDocuments: []string{"docs/b.md", "docs/a.md"}, conditionalProfiles: []string{" SIGNATURE "},
	})
	require.NoError(t, err)
	second, err := prepareSpecReviewContextDelivery(specDir, providers, specReviewOptions{
		requiredDocuments: []string{"docs/a.md", "docs/b.md"}, conditionalProfiles: []string{"signature"},
	})
	require.NoError(t, err)
	firstReceipt, err := first.buildVerified()
	require.NoError(t, err)
	secondReceipt, err := second.buildVerified()
	require.NoError(t, err)

	assert.Equal(t, firstReceipt.SnapshotHash, secondReceipt.SnapshotHash)
	assert.Equal(t, firstReceipt.RequiredDocuments, secondReceipt.RequiredDocuments)
}

func writeGPTReviewContextProject(t *testing.T) (string, string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	specID := "SPEC-GPT-REVIEW-CONTEXT-001"
	specDir := scaffoldReviewSpec(t, root, specID)
	markers := map[string]string{
		"AGENTS.md":                     "GPT_AGENTS_TAIL",
		".autopus/project/workspace.md": "GPT_WORKSPACE_TAIL",
		"ARCHITECTURE.md":               "GPT_ARCHITECTURE_TAIL",
	}
	for ref, marker := range markers {
		content := ref + "\n" + marker
		if ref == "ARCHITECTURE.md" {
			content = "GPT_ARCHITECTURE_HEAD\n" + strings.Repeat("architecture detail survives\n", 1400) + marker
		}
		writeCLIReviewContextFile(t, root, ref, content)
	}
	for _, name := range []string{"spec.md", "plan.md", "research.md", "acceptance.md"} {
		path := filepath.Join(specDir, name)
		body, err := os.ReadFile(path)
		require.NoError(t, err)
		ref := filepath.ToSlash(filepath.Join(".autopus", "specs", specID, name))
		marker := "GPT_" + strings.ToUpper(strings.TrimSuffix(name, ".md")) + "_TAIL"
		require.NoError(t, os.WriteFile(path, append(body, []byte("\n"+marker+"\n")...), 0o600))
		markers[ref] = marker
	}
	return root, specID, markers
}

func writeCLIReviewContextFile(t *testing.T, root, ref, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(ref))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func chdirForSpecReviewTest(t *testing.T, dir string) func() {
	t.Helper()
	original, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	return func() { require.NoError(t, os.Chdir(original)) }
}

func stubGPTSpecReviewProviders() func() {
	original := specReviewConfigProviders
	specReviewConfigProviders = func(_ *config.HarnessConfig, _ []string) []orchestra.ProviderConfig {
		return []orchestra.ProviderConfig{{Name: "codex", Binary: "codex"}}
	}
	return func() { specReviewConfigProviders = original }
}

func expectedCLIReviewContextHashes(t *testing.T, root string, markers map[string]string) map[string][2]string {
	t.Helper()
	hashes := make(map[string][2]string, len(markers))
	for ref := range markers {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref)))
		require.NoError(t, err)
		delivered := promptlayer.SanitizeContent(string(raw), promptlayer.ContextOptions{
			Required: true, PreserveInjectionEvidence: true,
		}).Content
		hashes[ref] = [2]string{canonicalCLIReviewHash(raw), canonicalCLIReviewHash([]byte(delivered))}
	}
	return hashes
}

func canonicalCLIReviewHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
