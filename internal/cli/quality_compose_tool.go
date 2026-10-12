package cli

import (
	"maps"

	"github.com/insajin/autopus-adk/pkg/config"
)

// toolStart is the starting composition of tool's tab (SPEC-MODELUX-002
// Per-Tool Binding Contract): every canonical agent at the tier the tool's
// provider view projects, its override preset when it has one and the
// global binding otherwise, with the fallback tier its embedded source
// declares.
func (r *composeSessionRun) toolStart(tool qualityModelTool) map[string]string {
	view := r.cfg.Quality.ForProvider(tool.provider)
	agents := config.CanonicalAgentNames()
	start := make(map[string]string, len(agents))
	for _, agent := range agents {
		start[agent] = view.EffectiveAgentTier(agent, r.sources[agent].FallbackTier)
	}
	return start
}

// toolCandidate returns the candidate function of tool's tab: a composition
// equal to the start leaves the loaded config as it is, and any other binds
// the tool to <provider>-custom holding it, in a detached copy built by the
// per-tool editor's withQualityToolPreset.
func (r *composeSessionRun) toolCandidate(tool qualityModelTool) func(map[string]string) *config.HarnessConfig {
	start := r.toolStart(tool)
	return func(composition map[string]string) *config.HarnessConfig {
		if maps.Equal(composition, start) {
			return r.view()
		}
		return withQualityToolPreset(r.view(), tool, qualityToolPresetName(tool), composition)
	}
}
