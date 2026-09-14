package promptlayer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Delivery assembles a frozen, profile-scoped document set: these tests pin which
// documents each command includes or excludes and which inputs are rejected outright.

func deliveryRefs(t *testing.T, result promptlayer.ContextDeliveryResult) []string {
	t.Helper()
	refs := make([]string, 0, len(result.RequiredDocuments))
	for _, document := range result.RequiredDocuments {
		refs = append(refs, document.SourceRef)
	}
	return refs
}

func TestBuildContextDelivery_RejectsUnknownCommandAndBlankReference(t *testing.T) {
	t.Parallel()
	root := writeContextDeliveryProject(t)

	_, err := promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "deploy", SpecDir: deliverySpecDir,
	})
	require.ErrorContains(t, err, "unknown context profile command")

	_, err = promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "", SpecDir: deliverySpecDir,
	})
	require.ErrorContains(t, err, "unknown context profile command")

	_, err = promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "go", SpecDir: deliverySpecDir, RequiredReferences: []string{"   "},
	})
	require.ErrorContains(t, err, "path is required")

	_, err = promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "go", SpecDir: deliverySpecDir,
		RequiredReferences: []string{filepath.Join(root, "AGENTS.md")},
	})
	require.ErrorContains(t, err, "path must be relative")
}

func TestBuildContextDelivery_CommandNormalizationAndDuplicateReferences(t *testing.T) {
	t.Parallel()
	root := writeContextDeliveryProject(t)

	canonical, err := promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "go", SpecDir: deliverySpecDir,
	})
	require.NoError(t, err)

	normalized, err := promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "  GO  ", SpecDir: "./" + deliverySpecDir + "/",
		// A duplicate of an already-required document must not double-include it.
		RequiredReferences: []string{"AGENTS.md", "./AGENTS.md"},
	})
	require.NoError(t, err)
	assert.Equal(t, canonical.SnapshotHash, normalized.SnapshotHash)
	assert.Equal(t, deliveryRefs(t, canonical), deliveryRefs(t, normalized))
}

func TestBuildContextDelivery_SpecScopePerCommand(t *testing.T) {
	t.Parallel()
	root := writeContextDeliveryProject(t)
	for _, rel := range []string{
		"ARCHITECTURE.md", ".autopus/project/product.md",
		".autopus/project/structure.md", ".autopus/project/tech.md",
		".autopus/project/scenarios.md",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("body for "+rel+"\n"), 0o600))
	}

	planned, err := promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "plan", SpecDir: deliverySpecDir,
	})
	require.NoError(t, err)
	planRefs := deliveryRefs(t, planned)
	assert.Contains(t, planRefs, deliverySpecDir+"/spec.md")
	assert.NotContains(t, planRefs, deliverySpecDir+"/plan.md")
	assert.NotContains(t, planRefs, deliverySpecDir+"/acceptance.md")
	assert.Contains(t, planRefs, "ARCHITECTURE.md")

	// "test" declares no relevant spec, so no spec document may leak in and
	// the spec directory is optional.
	tested, err := promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "test",
	})
	require.NoError(t, err)
	testRefs := deliveryRefs(t, tested)
	assert.Contains(t, testRefs, ".autopus/project/scenarios.md")
	for _, ref := range testRefs {
		assert.NotContains(t, ref, deliverySpecDir)
	}
	assert.Empty(t, tested.SpecDir)
}

func TestBuildContextDelivery_ExplicitArchitectureSuppressesDefaultConditionals(t *testing.T) {
	t.Parallel()
	root := writeContextDeliveryProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "ARCHITECTURE.md"), []byte("arch body\n"), 0o600))

	implicit, err := promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "go", SpecDir: deliverySpecDir,
	})
	require.NoError(t, err)
	assert.Contains(t, deliveryRefs(t, implicit), "ARCHITECTURE.md")

	explicit, err := promptlayer.BuildContextDelivery(promptlayer.ContextDeliveryOptions{
		Root: root, Command: "go", SpecDir: deliverySpecDir, ExplicitArchitecture: true,
	})
	require.NoError(t, err)
	assert.NotContains(t, deliveryRefs(t, explicit), "ARCHITECTURE.md")
	assert.NotEqual(t, implicit.SnapshotHash, explicit.SnapshotHash)
}

func TestCommandContextProfile_ConditionalDocumentsAreSeparateFromRequired(t *testing.T) {
	t.Parallel()

	profile, ok := promptlayer.ResolveCommandContextProfile("plan")
	require.True(t, ok)
	required := profile.RequiredDocuments()
	conditional := profile.ConditionalDocuments()
	assert.Contains(t, required, "AGENTS.md")
	assert.Contains(t, required, "ARCHITECTURE.md")
	assert.Contains(t, conditional, ".autopus/context/signatures.md")
	assert.Contains(t, conditional, ".autopus/learnings/pipeline.jsonl")
	for _, document := range conditional {
		assert.NotContains(t, required, document, "a conditional document must never be silently required")
	}

	canary, ok := promptlayer.ResolveCommandContextProfile("canary")
	require.True(t, ok)
	assert.Equal(t, []string{".autopus/learnings/pipeline.jsonl"}, canary.ConditionalDocuments())

	_, ok = promptlayer.ResolveCommandContextProfile("deploy")
	assert.False(t, ok)
}
