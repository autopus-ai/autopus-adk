package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestOMPStatusProjectionReportsStaleCatalogAndMissingReceipt(t *testing.T) {
	root, runner, _ := writeSelectedOMPProfile(t)
	missing := buildOMPPlatformProjection(context.Background(), root, runner, time.Now().UTC())
	assert.Equal(t, "blocked", missing.Models.Status)
	assert.Equal(t, "receipt_missing", missing.Models.Reason)
	assert.Contains(t, missing.Blockers, "models:receipt_missing")
	var missingText bytes.Buffer
	missingTextCmd := &cobra.Command{Use: "status"}
	missingTextCmd.SetOut(&missingText)
	require.NoError(t, renderOMPPlatformStatus(missingTextCmd, missing, false))
	assert.Contains(t, missingText.String(), "reason=receipt_missing")

	var missingJSON bytes.Buffer
	missingJSONCmd := &cobra.Command{Use: "status"}
	missingJSONCmd.SetOut(&missingJSON)
	require.NoError(t, renderOMPPlatformStatus(missingJSONCmd, missing, true))
	var missingEnvelope struct {
		Data ompPlatformProjection `json:"data"`
	}
	require.NoError(t, json.Unmarshal(missingJSON.Bytes(), &missingEnvelope))
	assert.Equal(t, "receipt_missing", missingEnvelope.Data.Models.Reason)

	writeStaleOMPModelReceipt(t, root)
	stale := buildOMPPlatformProjection(context.Background(), root, runner, time.Now().UTC())
	assert.Equal(t, "catalog_stale", stale.Models.Reason)
	assert.False(t, stale.Models.ReceiptVerified)
	assert.Contains(t, stale.Blockers, "models:catalog_stale")
	var staleText bytes.Buffer
	staleTextCmd := &cobra.Command{Use: "status"}
	staleTextCmd.SetOut(&staleText)
	require.NoError(t, renderOMPPlatformStatus(staleTextCmd, stale, false))
	assert.Contains(t, staleText.String(), "reason=catalog_stale")

	var out bytes.Buffer
	cmd := &cobra.Command{Use: "status"}
	cmd.SetOut(&out)
	require.NoError(t, renderOMPPlatformStatus(cmd, stale, true))
	var statusEnvelope struct {
		Data ompPlatformProjection `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &statusEnvelope))
	assert.Equal(t, "catalog_stale", statusEnvelope.Data.Models.Reason)
	assert.False(t, statusEnvelope.Data.ReceiptVerification.ModelVerified)
}

// OMP installs its own agents, so a workspace with no project agent file is
// ready, not blocked. Status reports where the registry comes from instead of
// counting local definitions that Autopus never writes.
func TestOMPStatusNoOptInReportsBundledRegistryAndBareStatusIsCompatible(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-status")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	runner := &ompCLIFakeRunner{catalog: ompCLIReadyCatalogJSON()}
	deps := normalizeOMPPlatformDependencies(ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		now:       func() time.Time { return time.Now().UTC() },
	})
	status := newStatusCmdWithOMPDependencies(deps)
	var operatorOut bytes.Buffer
	status.SetOut(&operatorOut)
	status.SetErr(&operatorOut)
	status.SetArgs([]string{"--dir", root, "--platform", "omp"})
	require.NoError(t, status.Execute())
	assert.Contains(t, operatorOut.String(), "Agent registry: status=ready")
	assert.Contains(t, operatorOut.String(), "source=omp_bundled_registry")
	assert.Contains(t, operatorOut.String(), "expected=5 shadowed=0")
	assert.NotContains(t, operatorOut.String(), "agent_catalog_incomplete")
	assert.Contains(t, operatorOut.String(), ompLiveStateLimitation)
	assert.Contains(t, operatorOut.String(), ompLiveStateNextCommand)
	assert.Empty(t, runner.calls, "no opt-in must not probe OMP")
	jsonStatusCmd := newStatusCmdWithOMPDependencies(deps)
	var jsonOut bytes.Buffer
	jsonStatusCmd.SetOut(&jsonOut)
	jsonStatusCmd.SetErr(&jsonOut)
	jsonStatusCmd.SetArgs([]string{"--dir", root, "--platform", "omp", "--json"})
	require.NoError(t, jsonStatusCmd.Execute())
	var statusEnvelope ompCLIJSONEnvelope
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &statusEnvelope))
	assert.Equal(t, jsonStatusOK, statusEnvelope.Status)
	var statusPayload ompPlatformProjection
	require.NoError(t, json.Unmarshal(statusEnvelope.Data, &statusPayload))
	assert.Equal(t, "ready", statusPayload.Status)
	assert.Equal(t, "native_agent_registry", statusPayload.Models.AgentCatalogReason)
	assert.Equal(t, ompLiveStateLimitation, statusPayload.ChildRuntime.Limitation)
	assert.Empty(t, runner.calls, "JSON no opt-in must not probe OMP")

	var want bytes.Buffer
	legacy := &cobra.Command{Use: "status"}
	legacy.SetOut(&want)
	require.NoError(t, runStatusText(legacy, root))
	bare := newStatusCmdWithOMPDependencies(deps)
	var got bytes.Buffer
	bare.SetOut(&got)
	bare.SetErr(&got)
	bare.SetArgs([]string{"--dir", root})
	require.NoError(t, bare.Execute())
	assert.Equal(t, want.String(), got.String())
}
