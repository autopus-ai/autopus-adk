package promptlayer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExplicitArchitectureContextKeepsRequiredContract(t *testing.T) {
	t.Parallel()
	root := writeContextDeliveryProject(t)
	for _, ref := range []string{"ARCHITECTURE.md", ".autopus/project/product.md", ".autopus/project/structure.md", ".autopus/project/tech.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, ref), []byte("OPTIONAL_ARCHITECTURE_DETAIL"), 0o600))
	}
	opts := promptlayer.ContextDeliveryOptions{Root: root, Command: "go", SpecDir: deliverySpecDir, ExplicitArchitecture: true}
	selected, err := promptlayer.BuildContextDelivery(opts)
	require.NoError(t, err)
	require.NoError(t, promptlayer.VerifyContextDeliveryForOptions(opts, selected))
	assert.NotContains(t, selected.Prompt, "OPTIONAL_ARCHITECTURE_DETAIL")
	refs := make(map[string]bool)
	for _, document := range selected.RequiredDocuments {
		refs[document.SourceRef] = true
	}
	assert.True(t, refs["AGENTS.md"])
	assert.True(t, refs[deliverySpecDir+"/spec.md"])
	assert.True(t, refs[deliverySpecDir+"/acceptance.md"])

	opts.ConditionalProfiles = []promptlayer.ContextProfileName{promptlayer.ProfileArchitecture}
	expanded, err := promptlayer.BuildContextDelivery(opts)
	require.NoError(t, err)
	assert.Contains(t, expanded.Prompt, "OPTIONAL_ARCHITECTURE_DETAIL")
	require.Error(t, promptlayer.VerifyContextDeliveryForOptions(opts, selected))
	require.NoError(t, promptlayer.VerifyContextDeliveryForOptions(opts, expanded))
}

func TestExplicitArchitectureDoesNotChangeCanonicalFullDelivery(t *testing.T) {
	t.Parallel()
	root := writeContextDeliveryProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "ARCHITECTURE.md"), []byte("CANONICAL_ARCHITECTURE"), 0o600))
	full, err := promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{Root: root, Command: "go", SpecDir: deliverySpecDir})
	require.NoError(t, err)
	assert.Contains(t, full.Prompt, "CANONICAL_ARCHITECTURE")
}
