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

// bridgeOMPIntegrationRoutes builds one route request per agent the profile's
// source serves, keyed by the name OMP binds a model to. In autopus mode that
// is each canonical agent and task, routed by the Route Contract; in native
// mode it is each bundled agent, routed by the single operator-written
// override that collapses onto it, or the representative ADK role's route.
func bridgeOMPIntegrationRoutes(
	profile config.RoleModelProfileConf,
) (map[string]OMPModelRouteRequest, error) {
	agents := profile.OMPAgentNames()
	routes := make(map[string]OMPModelRouteRequest, len(agents))
	diverseRoles := make(map[string]struct{}, len(profile.FamilyDiversity.Roles))
	if profile.FamilyDiversity.Enabled {
		for _, role := range profile.FamilyDiversity.Roles {
			diverseRoles[role] = struct{}{}
		}
	}
	for _, agent := range agents {
		resolved, route, err := profile.OMPAgentRoute(agent)
		if err != nil {
			return nil, err
		}
		_, preferDistinctFamily := diverseRoles[resolved.Role]
		request := OMPModelRouteRequest{
			Agent: agent, Role: resolved.Role, Capability: resolved.Capability,
			Required: route.Required, DegradedAction: route.DegradedAction,
			PreferDistinctExecutorFamily: preferDistinctFamily,
			Candidates:                   make([]OMPRoutingCandidate, 0, len(route.Candidates)),
		}
		for _, candidate := range route.Candidates {
			request.Candidates = append(request.Candidates, OMPRoutingCandidate{
				Selector: candidate.Selector, Thinking: candidate.Thinking, Family: candidate.Family,
			})
		}
		routes[agent] = request
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
		if route.Required && route.DegradedAction != ompRuntimeDefaultAction {
			err := fmt.Errorf("required_route_unresolved: %s: %s", resolution.Agent, resolution.Reason)
			if hint := ompLoginHint(ompUnavailableCandidateProviders(catalog, route)); hint != "" {
				err = fmt.Errorf("%w (%s)", err, hint)
			}
			return nil, err
		}
	}
	// The compilation is already in the served set's rank order.
	result := make([]OMPProjectionAgent, 0, len(resolved))
	for _, compiled := range compilation.Resolutions {
		resolution, ok := resolved[compiled.Agent]
		if !ok {
			continue
		}
		agent := resolution.Agent
		projection := OMPProjectionAgent{
			Agent: agent, Role: resolution.RequestedRole, Capability: resolution.Capability,
			Selector: resolution.EffectiveProvider + "/" + resolution.EffectiveModel,
			Thinking: resolution.Thinking,
		}
		for _, candidate := range routes[agent].Candidates {
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
