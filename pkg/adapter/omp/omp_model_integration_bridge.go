package omp

import (
	"fmt"

	"github.com/insajin/autopus-adk/pkg/config"
)

// validateOMPIntegrationOverrides re-checks the optional role/capability pins
// on agent overrides. config.Validate already rejects unroutable agents and
// candidate shape; only agreement with the resolved identity is confirmed
// here, for the 16 ADK keys and the five native keys alike.
func validateOMPIntegrationOverrides(profile config.RoleModelProfileConf) error {
	for agent, override := range profile.Agents {
		resolved, err := config.ResolveOMPPolicyAgent(agent)
		if err != nil {
			return err
		}
		if override.Role != "" && override.Role != resolved.Role ||
			override.Capability != "" && override.Capability != resolved.Capability {
			return fmt.Errorf("role_capability_mismatch: agent=%s", agent)
		}
	}
	return nil
}

// bridgeOMPIntegrationRoutes builds one route request per native OMP agent,
// keyed by native agent name. The governing route is the single
// operator-written override that collapses onto that agent, or the
// representative ADK role's route when the operator wrote none.
func bridgeOMPIntegrationRoutes(
	profile config.RoleModelProfileConf,
) (map[string]OMPModelRouteRequest, error) {
	natives := config.OMPNativeAgentNames()
	routes := make(map[string]OMPModelRouteRequest, len(natives))
	diverseRoles := make(map[string]struct{}, len(profile.FamilyDiversity.Roles))
	if profile.FamilyDiversity.Enabled {
		for _, role := range profile.FamilyDiversity.Roles {
			diverseRoles[role] = struct{}{}
		}
	}
	for _, native := range natives {
		resolved, route, err := profile.OMPNativeAgentRoute(native)
		if err != nil {
			return nil, err
		}
		_, preferDistinctFamily := diverseRoles[resolved.Role]
		request := OMPModelRouteRequest{
			Agent: native, Role: resolved.Role, Capability: resolved.Capability,
			Required: route.Required, DegradedAction: route.DegradedAction,
			PreferDistinctExecutorFamily: preferDistinctFamily,
			Candidates:                   make([]OMPRoutingCandidate, 0, len(route.Candidates)),
		}
		for _, candidate := range route.Candidates {
			request.Candidates = append(request.Candidates, OMPRoutingCandidate{
				Selector: candidate.Selector, Thinking: candidate.Thinking, Family: candidate.Family,
			})
		}
		routes[native] = request
	}
	return routes, nil
}

// projectOMPIntegrationAgents keeps only agents whose route resolved to an
// exact selector; a required route that stays unresolved fails closed.
func projectOMPIntegrationAgents(
	catalog OMPModelCatalog,
	routes map[string]OMPModelRouteRequest,
	compilation OMPModelRoutingCompilation,
) ([]OMPProjectionAgent, error) {
	resolved := make(map[string]OMPModelRouteResolution, len(compilation.Resolutions))
	for _, resolution := range compilation.Resolutions {
		route, exists := routes[resolution.Agent]
		if !exists {
			return nil, fmt.Errorf("route_missing: %s", resolution.Agent)
		}
		if resolution.Status == "selected" && resolution.EffectiveSelector != "" {
			resolved[resolution.Agent] = resolution
			continue
		}
		if route.Required && route.DegradedAction != "runtime_default" {
			return nil, fmt.Errorf("required_route_unresolved: %s: %s", resolution.Agent, resolution.Reason)
		}
	}
	result := make([]OMPProjectionAgent, 0, len(resolved))
	for _, native := range config.OMPNativeAgentNames() {
		resolution, ok := resolved[native]
		if !ok {
			continue
		}
		projection := OMPProjectionAgent{
			Agent: native, Role: resolution.RequestedRole, Capability: resolution.Capability,
			Selector: resolution.EffectiveProvider + "/" + resolution.EffectiveModel,
			Thinking: resolution.Thinking,
		}
		for _, candidate := range routes[native].Candidates {
			if formatOMPRoutingSelector(candidate) == resolution.EffectiveSelector {
				continue
			}
			if _, reason := matchOMPModelCandidate(catalog.Models, resolution.Capability, candidate); reason == "compatible" {
				projection.Fallbacks = append(projection.Fallbacks, OMPProjectionCandidate{
					Selector: candidate.Selector, Thinking: candidate.Thinking,
				})
			}
		}
		result = append(result, projection)
	}
	return result, nil
}
