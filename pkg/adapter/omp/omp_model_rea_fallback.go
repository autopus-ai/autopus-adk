package omp

import (
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// dropOMPReaAnalysisFallbackRoutes removes each analysis-rung agent's route
// when the catalog cannot serve the built-in balanced analysis rung
// (config.OMPReaAnalysisFallback, SPEC-REA-001 Gate 1). Without a route the
// agent gets no task.agentModelOverrides key and no projection entry, so OMP
// runs it on the session model and its definition keeps its own thinking
// level. Every other route is left for the existing fail-closed checks. The
// structured fallback fact of each route given up is returned for the notice,
// which the CLI prints in the run's language.
func dropOMPReaAnalysisFallbackRoutes(
	profile config.RoleModelProfileConf,
	catalog OMPModelCatalog,
	routes map[string]OMPModelRouteRequest,
) []adapter.ReaFallback {
	var dropped []adapter.ReaFallback
	for _, agent := range config.AnalysisRungAgents() {
		request, ok := routes[agent]
		if !ok || len(request.Candidates) != 1 {
			continue
		}
		_, reason := matchOMPModelCandidate(catalog.Models, request.Capability, request.Candidates[0])
		if !config.OMPReaAnalysisFallback(profile, agent, reason) {
			continue
		}
		delete(routes, agent)
		dropped = append(dropped, adapter.ReaFallback{
			Catalog: "OMP", Agent: agent, Requested: request.Candidates[0].Selector,
		})
	}
	return dropped
}

// OMPReaSessionModelStatus is the routing status of the route generation
// drops for the analysis-rung fallback, in a compilation that keeps it.
const OMPReaSessionModelStatus = "session_model"

// MarkOMPReaAnalysisFallbacks rewrites, in a compilation that kept every
// route, each unresolved route config.OMPReaAnalysisFallback lets generation
// drop, so doctor and plan rows report the session-model fallback instead of
// a blocked route. Every other resolution is left as compiled.
func MarkOMPReaAnalysisFallbacks(profile config.RoleModelProfileConf, compilation *OMPModelRoutingCompilation) {
	for index := range compilation.Resolutions {
		resolution := &compilation.Resolutions[index]
		if resolution.Status == "selected" || len(resolution.FallbackAttempts) != 1 {
			continue
		}
		if config.OMPReaAnalysisFallback(profile, resolution.Agent, resolution.FallbackAttempts[0].Reason) {
			resolution.Status = OMPReaSessionModelStatus
			resolution.Reason = "rea_analysis_fallback"
		}
	}
}
