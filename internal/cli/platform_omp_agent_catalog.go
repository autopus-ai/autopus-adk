package cli

import (
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ompNativeAgentRegistrySource names where task comes from: the bundled
// registry.
const ompNativeAgentRegistrySource = "omp_bundled_registry"

// ompAutopusAgentRegistrySource names the generated .omp/agents definitions
// autopus mode serves the canonical agents from.
const ompAutopusAgentRegistrySource = "autopus_generated"

type ompAgentCatalogSummary struct {
	Status   string
	Reason   string
	Expected int
	Source   string
	Shadowed int
}

func newOMPModelOperatorProjection(
	rows []ompEffectiveModelProjection,
	catalog ompAgentCatalogSummary,
) ompModelOperatorProjection {
	return ompModelOperatorProjection{
		Status: "disabled", Reason: "profile_not_selected", CatalogStatus: "not_probed",
		CatalogReason: "profile_not_selected", CatalogTrust: config.RoleModelCatalogTrustStrict,
		ReceiptStatus:      "not_applicable",
		AgentCatalogStatus: catalog.Status, AgentCatalogReason: catalog.Reason,
		AgentCatalogSource: catalog.Source, ExpectedAgents: catalog.Expected,
		ShadowedAgents: catalog.Shadowed, Models: rows,
	}
}

// buildOMPAgentCatalog projects one inherited baseline row per agent OMP
// serves. A generated definition under its own name is the agent, not a
// shadow, so Shadowed stays zero; a missing one, a user file OMP serves in
// its place, and a task.disabledAgents left unwritten, because the project
// config cannot be read safely or the user owns it, which keeps the bundled
// agents enabled, are the degradations an operator has to see.
func buildOMPAgentCatalog(root string) ([]ompEffectiveModelProjection, ompAgentCatalogSummary) {
	agents := config.OMPAutopusAgentNames()
	rows := make([]ompEffectiveModelProjection, 0, len(agents))
	summary := ompAgentCatalogSummary{
		Expected: len(agents), Source: ompAutopusAgentRegistrySource,
		Status: "ready", Reason: "autopus_agent_registry",
	}
	for _, agent := range agents {
		source := ompAutopusAgentRegistrySource
		if agent == config.OMPAutopusDefaultAgent {
			source = ompNativeAgentRegistrySource
		}
		row := ompEffectiveModelProjection{
			Agent: agent, ModelSource: "inherit", Source: source, ConfigSource: "inherited",
			Status: "inherited", Reason: "profile_not_selected", FallbackAttempts: []ompFallbackProjection{},
		}
		if policy, err := config.ResolveOMPPolicyAgent(agent); err == nil {
			row.Role, row.Capability = policy.Role, policy.Capability
		}
		rows = append(rows, row)
	}
	report, err := omp.InspectOMPAgentSurface(root)
	switch {
	case err != nil:
		summary.Status, summary.Reason = "degraded", "agent_surface_unreadable"
	case len(report.MissingAgents) > 0:
		summary.Status, summary.Reason = "degraded", "autopus_agent_missing"
	case len(report.UserAgentFiles) > 0:
		summary.Status, summary.Reason = "degraded", "autopus_agent_user_file"
	case report.ProjectConfigUnclaimable:
		summary.Status, summary.Reason = "degraded", "project_config_unclaimable"
	case report.DisabledAgentsUserOwned:
		summary.Status, summary.Reason = "degraded", "disabled_agents_user_owned"
	}
	return rows, summary
}

func overlayOMPAgentCatalog(
	rows []ompEffectiveModelProjection,
	resolutions []omp.OMPModelRouteResolution,
	profile config.RoleModelProfileConf,
	receiptVerified bool,
) []ompEffectiveModelProjection {
	byAgent := make(map[string]omp.OMPModelRouteResolution, len(resolutions))
	for _, resolution := range resolutions {
		if _, exists := byAgent[resolution.Agent]; !exists {
			byAgent[resolution.Agent] = resolution
		}
	}
	for index := range rows {
		row := &rows[index]
		resolution, exists := byAgent[row.Agent]
		if !exists {
			continue
		}
		row.ModelSource = config.OMPNativeAgentModelOverridesKey
		row.Source = "autopus.yaml"
		row.ConfigSource = safeOMPOperatorToken(profile.ConfigMode)
		row.Status = safeOMPOperatorReason(resolution.Status)
		row.Reason = safeOMPOperatorReason(resolution.Reason)
		row.FallbackAttempts = projectOMPFallbackAttempts(resolution.FallbackAttempts)
		row.FallbackUsed = ompFallbackWasUsed(resolution.FallbackAttempts)
		row.Verified = receiptVerified && resolution.Status == "selected"
		if resolution.RequestedRole != "" {
			row.Role = safeOMPOperatorToken(resolution.RequestedRole)
		}
		if resolution.Capability != "" {
			row.Capability = safeOMPOperatorToken(resolution.Capability)
		}
		if resolution.Status != "selected" {
			continue
		}
		row.Provider = safeOMPOperatorToken(resolution.EffectiveProvider)
		row.Model = safeOMPOperatorToken(resolution.EffectiveModel)
		row.Thinking = safeOMPOperatorToken(resolution.Thinking)
		row.EffectiveSelector = safeOMPOperatorToken(resolution.EffectiveSelector)
	}
	return rows
}

func projectOMPFallbackAttempts(attempts []omp.OMPRoutingAttempt) []ompFallbackProjection {
	projected := make([]ompFallbackProjection, 0, len(attempts))
	for _, attempt := range attempts {
		projected = append(projected, ompFallbackProjection{
			Index: attempt.Index, Selector: safeOMPOperatorToken(attempt.Selector),
			Status: safeOMPOperatorReason(attempt.Status), Reason: safeOMPOperatorReason(attempt.Reason),
		})
	}
	return projected
}

func ompFallbackWasUsed(attempts []omp.OMPRoutingAttempt) bool {
	for _, attempt := range attempts {
		if attempt.Status == "selected" && attempt.Index > 0 {
			return true
		}
	}
	return false
}
