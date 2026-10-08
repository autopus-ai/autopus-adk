package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// TestEvalHarnessExport_DefaultOracleAssertions_AreMainsBlackBoxDefinitions:
// the production source hands the signer the assertion ids of the committed
// black-box task definitions, and none of a white-box task.
func TestEvalHarnessExport_DefaultOracleAssertions_AreMainsBlackBoxDefinitions(t *testing.T) {
	t.Parallel()
	set, err := harneval.LoadSet(filepath.Join("..", ".."))
	require.NoError(t, err)

	got, err := harnessTrustSources{}.withDefaults().oracleAssertions(set)

	require.NoError(t, err)
	assert.Equal(t, map[string][]string{
		"GT-AGENT-A01": {"exit", "stdout"}, "GT-AGENT-A02": {"exit", "stdout"},
		"GT-AGENT-A05": {"exit", "stdout", harneval.PositiveControlExitID, harneval.PositiveControlStdoutID},
		"GT-AGENT-A06": {"exit", "stdout"}, "GT-AGENT-B04": {"exit", "stdout"},
	}, got)
	assert.GreaterOrEqual(t, len(got), set.Manifest.Floors.SignedAgentTasks, "the committed set meets its signed-lane floor")
}

// TestEvalHarnessExport_DefaultBaselineSurface_RebuildsTheBindingCommit: the
// production source rebuilds the binding's baseline_commit through
// harneval.ArmSurfaceDigest and files a failure under baseline_surface_failed.
func TestEvalHarnessExport_DefaultBaselineSurface_RebuildsTheBindingCommit(t *testing.T) {
	t.Parallel()
	req := harnessExportRequest{Root: t.TempDir(), Binding: harneval.Binding{BaselineRef: "v0.50.123", Pins: harneval.Pins{GeneratorVersion: "v0.50.123"}}}

	_, err := harnessTrustSources{}.withDefaults().baselineSurface(context.Background(), req)

	require.Error(t, err)
	assert.Equal(t, `baseline_surface_failed: arm surface: "" is not a 40-hex commit`, err.Error())
}
