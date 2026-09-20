package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkflowContextImplementationIdentityCommand(t *testing.T) {
	cmd := newWorkflowContextImplementationIdentityCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--format", "json"})
	require.NoError(t, cmd.Execute())

	var identity workflowContextImplementationIdentityV1
	require.NoError(t, json.Unmarshal(output.Bytes(), &identity))
	require.Equal(t, workflowContextImplementationIdentitySchemaV1, identity.SchemaVersion)
	require.Equal(t, "sha256:eb94cbb82bca2acd874fcc68018c90cd8253ccd3afb721ae7dca5beee3809692",
		identity.PipelineImplementationDigest)
	require.Equal(t, "autopus.omp-pipeline-managed-rpc.v3", identity.RPCIdentity)
	require.Equal(t, pipelineOMPActivePolicyIdentity, identity.PolicyIdentity)
	require.NotEmpty(t, identity.BridgeTarget)
	require.NotEmpty(t, identity.BridgeSHA256)
	require.NotEmpty(t, identity.RouteTarget)
	require.NotEmpty(t, identity.RouteSHA256)
}

func TestWorkflowContextImplementationIdentityRejectsUnsupportedFormat(t *testing.T) {
	cmd := newWorkflowContextImplementationIdentityCmd()
	cmd.SetArgs([]string{"--format", "yaml"})
	require.EqualError(t, cmd.Execute(),
		"workflow context-runtime implementation-identity requires --format json")
}
