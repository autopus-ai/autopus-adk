package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ompadapter "github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// OMP ships its agents inside the binary and Autopus installs none, so a
// workspace with no project agent file is the normal state. It must project the
// five bundled agents and must not block the platform on absent local files.
func TestOMPAgentCatalog_ProjectsBundledRegistryWithoutProjectDefinitions(t *testing.T) {
	root := writeOMPConfigOnlyWorkspace(t)
	runner := &ompCLIFakeRunner{catalog: ompCLIReadyCatalogJSON()}
	deps := normalizeOMPPlatformDependencies(ompPlatformDependencies{
		newRunner: func() ompadapter.OMPModelCatalogRunner { return runner },
	})
	require.NoFileExists(t, filepath.Join(root, ".omp", "agents", "reviewer.md"))

	status := decodeOMPAgentCatalogStatus(t, executeOMPSubcommand(
		t, newStatusCmdWithOMPDependencies(deps), "--dir", root, "--platform", "omp", "--json",
	))
	dir := root
	explain := decodeOMPAgentCatalogExplain(t, executeOMPSubcommand(
		t, newPlatformOMPExplainCmd(&dir, deps), "--json",
	))

	assertOMPAgentCatalogBaseline(t, status.Models.Models)
	assertOMPAgentCatalogBaseline(t, explain.Models.Models)
	assert.Equal(t, status.Models.Models, explain.Models.Models)
	for _, projection := range []ompModelOperatorProjection{status.Models, explain.Models} {
		assert.Equal(t, "ready", projection.AgentCatalogStatus)
		assert.Equal(t, "native_agent_registry", projection.AgentCatalogReason)
		assert.Equal(t, "omp_bundled_registry", projection.AgentCatalogSource)
		assert.Equal(t, 5, projection.ExpectedAgents)
		assert.Zero(t, projection.ShadowedAgents)
	}
	assert.Equal(t, "ready", status.Status)
	assert.Equal(t, "ready", explain.Status)
	assert.Empty(t, status.Blockers)
	assert.Empty(t, explain.Blockers)
	assert.Empty(t, runner.calls, "an inherited registry must not probe provider routing")
}

// A project file whose name matches a bundled agent wins OMP's exact-name
// resolution and silently replaces it. That is observable locally and is the
// one thing this projection can honestly report about the registry.
func TestOMPAgentCatalog_ReportsProjectFileShadowingBundledAgent(t *testing.T) {
	root := writeOMPConfigOnlyWorkspace(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".omp", "agents"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".omp", "agents", "reviewer.md"), []byte("---\nname: reviewer\n---\n"), 0o600,
	))

	projection := buildOMPPlatformProjection(
		context.Background(), root, &ompCLIFakeRunner{catalog: ompCLIReadyCatalogJSON()},
		configTimeForOMPAgentCatalog(),
	)

	assert.Equal(t, "degraded", projection.Models.AgentCatalogStatus)
	assert.Equal(t, "native_agent_shadowed", projection.Models.AgentCatalogReason)
	assert.Equal(t, 1, projection.Models.ShadowedAgents)
	assert.Equal(t, "degraded", projection.Status)
	assert.Contains(t, projection.Blockers, "agents:native_agent_shadowed")
	for _, row := range projection.Models.Models {
		assert.Equal(t, row.Agent == "reviewer", row.Shadowed, row.Agent)
	}
}

// A symlink at the same path shadows the bundled agent just as a regular file
// does, so the probe must stat without following instead of ignoring it.
func TestOMPAgentCatalog_TreatsSymlinkedDefinitionAsShadow(t *testing.T) {
	root := writeOMPConfigOnlyWorkspace(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".omp", "agents"), 0o750))
	outside := filepath.Join(t.TempDir(), "task.md")
	require.NoError(t, os.WriteFile(outside, []byte("---\nname: task\n---\n"), 0o600))
	link := filepath.Join(root, ".omp", "agents", "task.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	projection := buildOMPPlatformProjection(
		context.Background(), root, &ompCLIFakeRunner{}, configTimeForOMPAgentCatalog(),
	)

	assert.Equal(t, "degraded", projection.Models.AgentCatalogStatus)
	assert.Equal(t, 1, projection.Models.ShadowedAgents)
}

// A selected profile binds a concrete model to each bundled agent through the
// native override key. No alias appears because none is emitted any more.
func TestOMPAgentCatalog_SelectedRoutingOverlaysNativeOverrideAndExactSelector(t *testing.T) {
	root, runner, _ := writeSelectedOMPProfile(t)
	projection := buildOMPPlatformProjection(context.Background(), root, runner, configTimeForOMPAgentCatalog())
	natives := config.OMPNativeAgentNames()
	require.Len(t, projection.Models.Models, len(natives))

	for index, row := range projection.Models.Models {
		assert.Equal(t, natives[index], row.Agent)
		assert.Equal(t, config.OMPNativeAgentModelOverridesKey, row.ModelSource, row.Agent)
		assert.NotContains(t, row.ModelSource, "@", row.Agent)
		require.NotEmpty(t, row.Provider, row.Agent)
		require.NotEmpty(t, row.Model, row.Agent)
		require.NotEmpty(t, row.Thinking, row.Agent)
		assert.Equal(t, fmt.Sprintf("%s/%s:%s", row.Provider, row.Model, row.Thinking),
			row.EffectiveSelector, row.Agent)
		assert.False(t, row.Shadowed, row.Agent)
	}
	assert.Equal(t, "ready", projection.Models.AgentCatalogStatus)
	assert.NotContains(t, projection.Blockers, "agents:native_agent_shadowed")
}

func assertOMPAgentCatalogBaseline(t *testing.T, rows []ompEffectiveModelProjection) {
	t.Helper()
	natives := config.OMPNativeAgentNames()
	require.Equal(t, []string{"scout", "reviewer", "security-reviewer", "task", "sonic"}, natives)
	require.Len(t, rows, len(natives))
	for index, row := range rows {
		native := natives[index]
		policy, err := config.ResolveOMPPolicyAgent(native)
		require.NoError(t, err, native)
		assert.Equal(t, native, row.Agent)
		assert.Equal(t, policy.Role, row.Role, native)
		assert.Equal(t, policy.Capability, row.Capability, native)
		assert.Equal(t, "inherit", row.ModelSource, native)
		assert.Equal(t, "omp_bundled_registry", row.Source, native)
		assert.Empty(t, row.EffectiveSelector, native)
		assert.Equal(t, "inherited", row.Status, native)
		assert.Equal(t, "profile_not_selected", row.Reason, native)
		assert.False(t, row.Shadowed, native)
	}
}

func writeOMPConfigOnlyWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-agent-catalog")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	return root
}

func decodeOMPAgentCatalogStatus(t *testing.T, encoded string) ompPlatformProjection {
	t.Helper()
	var envelope ompCLIJSONEnvelope
	require.NoError(t, json.Unmarshal([]byte(encoded), &envelope))
	var projection ompPlatformProjection
	require.NoError(t, json.Unmarshal(envelope.Data, &projection))
	return projection
}

type ompAgentCatalogExplainPayload struct {
	Status   string                     `json:"status"`
	Models   ompModelOperatorProjection `json:"models"`
	Blockers []string                   `json:"blockers"`
}

func decodeOMPAgentCatalogExplain(t *testing.T, encoded string) ompAgentCatalogExplainPayload {
	t.Helper()
	var envelope ompCLIJSONEnvelope
	require.NoError(t, json.Unmarshal([]byte(encoded), &envelope))
	var projection ompAgentCatalogExplainPayload
	require.NoError(t, json.Unmarshal(envelope.Data, &projection))
	return projection
}

func configTimeForOMPAgentCatalog() time.Time {
	return time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
}
