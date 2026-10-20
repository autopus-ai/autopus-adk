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
	require.Equal(t, "sha256:6e378a249a6a6f2842645203d2545b7209b2f73997521e97838cc034b99d9f92",
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
