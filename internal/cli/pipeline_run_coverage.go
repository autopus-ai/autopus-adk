package cli

import "github.com/insajin/autopus-adk/pkg/config"

// pipelineCoverageThreshold resolves the coverage floor a pipeline run reports
// against. Without this the config key was inert: nothing outside tests read
// cfg.Workflow.CoverageThreshold, so a declared threshold never reached the
// runner's coverage-gap hook.
//
// LoadPreview, not Load: reading a threshold must not rewrite the project's
// configuration file as a side effect, the same reason resolvePipelineRoute
// uses it. An unreadable config keeps the declared default instead of dropping
// to zero, because zero is the explicit opt-out and must stay distinguishable
// from "could not tell".
func pipelineCoverageThreshold(projectDir string) float64 {
	cfg, err := config.LoadPreview(projectDir)
	if err != nil {
		return float64(config.DefaultCoverageThreshold)
	}
	return float64(cfg.Workflow.CoverageThreshold)
}
