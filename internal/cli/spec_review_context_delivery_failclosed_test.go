package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

func TestRunSpecReview_GPTStaleReceiptStopsBeforeProvider(t *testing.T) {
	root, specID, _ := writeGPTReviewContextProject(t)
	restoreWD := chdirForSpecReviewTest(t, root)
	defer restoreWD()
	stubGPTSpecReviewProviders(t)

	originalBuilder := specReviewBuildContextDelivery
	specReviewBuildContextDelivery = func(opts promptlayer.ContextDeliveryOptions) (promptlayer.ContextDeliveryResult, error) {
		receipt, err := originalBuilder(opts)
		if err == nil {
			err = os.WriteFile(filepath.Join(opts.Root, "AGENTS.md"), []byte("changed after snapshot"), 0o600)
		}
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

func TestRunSpecReview_GPTWrongSpecIdentityStopsBeforeProvider(t *testing.T) {
	root, specID, _ := writeGPTReviewContextProject(t)
	specPath := filepath.Join(root, ".autopus", "specs", specID, "spec.md")
	body, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		specPath,
		[]byte(strings.ReplaceAll(string(body), specID, "SPEC-REPLAYED-CONTEXT-999")),
		0o600,
	))
	restoreWD := chdirForSpecReviewTest(t, root)
	defer restoreWD()
	stubGPTSpecReviewProviders(t)

	providerCalls := 0
	originalRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		providerCalls++
		return &orchestra.OrchestraResult{}, nil
	}
	defer func() { specReviewRunOrchestra = originalRunner }()

	err = runSpecReviewWithOptions(context.Background(), specID, "consensus", 10, specReviewOptions{})

	require.Error(t, err)
	assert.ErrorContains(t, err, "wrong-SPEC context")
	assert.Zero(t, providerCalls)
}
