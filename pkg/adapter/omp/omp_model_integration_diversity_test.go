package omp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Family diversity is optional. A profile that routes every agent on one
// family, as the balanced built-in does, must still activate: independent
// review is an orchestra provider setting, not a routing requirement.
func TestOMPModelIntegration_FamilyDiversityDisabledStillActivates(t *testing.T) {
	t.Parallel()

	cfg := integrationHarnessConfig("overlay")
	profile := cfg.RoleModelPolicy.Profiles["p1"]
	profile.FamilyDiversity = config.FamilyDiversityPolicyConf{}
	cfg.RoleModelPolicy.Profiles["p1"] = profile

	files, err := NewWithRoot(t.TempDir()).
		WithModelIntegrationRunner(newModelIntegrationRunner()).
		prepareFiles(context.Background(), cfg)
	require.NoError(t, err)

	var receipt OMPModelResolutionReceipt
	require.NoError(t, json.Unmarshal(
		integrationMappingsByPath(files)[OMPModelReceiptRelativePath].Content, &receipt))
	require.Len(t, receipt.Roles, len(config.OMPNativeAgentNames()))
	for _, role := range receipt.Roles {
		assert.Equal(t, "not_applicable", role.FamilyDiversity.Status, role.Agent)
	}

	routes, err := bridgeOMPIntegrationRoutes(profile)
	require.NoError(t, err)
	for agent, route := range routes {
		assert.False(t, route.PreferDistinctExecutorFamily, agent)
	}
}
