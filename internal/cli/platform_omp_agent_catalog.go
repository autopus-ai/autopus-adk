package cli

import (
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ompNativeAgentRegistrySource names where OMP agents come from. Autopus
// installs no agent definition file, so a row's identity is the bundled
// registry entry, never a generated document.
const ompNativeAgentRegistrySource = "omp_bundled_registry"

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

// buildOMPAgentCatalog projects one inherited baseline row per bundled native
// OMP agent. Nothing is counted on disk because Autopus installs no agent
// definition: the only locally observable fact is whether a project file
// shadows a bundled agent, which is reported instead of guessed at.
func buildOMPAgentCatalog(root string) ([]ompEffectiveModelProjection, ompAgentCatalogSummary) {
	natives := config.OMPNativeAgentNames()
	rows := make([]ompEffectiveModelProjection, 0, len(natives))
	summary := ompAgentCatalogSummary{
		Expected: len(natives), Source: ompNativeAgentRegistrySource,
		Status: "ready", Reason: "native_agent_registry",
	}
	for _, native := range natives {
		row := ompEffectiveModelProjection{
			Agent: native, ModelSource: "inherit", EffectiveSelector: "",
			Source: ompNativeAgentRegistrySource, ConfigSource: "inherited",
			Status: "inherited", Reason: "profile_not_selected",
			Shadowed: shadowsOMPNativeAgent(root, native), FallbackAttempts: []ompFallbackProjection{},
		}
		if policy, err := config.ResolveOMPPolicyAgent(native); err == nil {
			row.Role, row.Capability = policy.Role, policy.Capability
		}
		if row.Shadowed {
			summary.Shadowed++
		}
		rows = append(rows, row)
	}
	if summary.Shadowed > 0 {
		summary.Status, summary.Reason = "degraded", "native_agent_shadowed"
	}
	return rows, summary
}

// shadowsOMPNativeAgent reports whether a project definition would override a
// bundled agent. OMP resolves exact agent names first-wins with the project
// directory ahead of its own registry, so anything present at that path
// replaces the bundled agent this projection describes. The path is only
// stat-ed, never followed or read, so a symlink counts as a shadow too.
func shadowsOMPNativeAgent(root, native string) bool {
	definitionPath := filepath.Join(".omp", "agents", native+".md")
	_, err := os.Lstat(filepath.Join(root, definitionPath))
	return err == nil
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
