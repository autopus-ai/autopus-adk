package omp

import (
	"context"
	"fmt"
	"sort"

	"github.com/insajin/autopus-adk/pkg/config"
)

func (a *Adapter) probeIntegratedModelCatalog(
	ctx context.Context,
	profile config.RoleModelProfileConf,
) (OMPModelCatalogProbeResult, error) {
	settings := []string{
		config.OMPNativeAgentModelOverridesKey, "retry.fallbackChains", "retry.modelFallback",
	}
	if profile.Safety.ApprovalMode != "" {
		settings = append(settings, "tools.approvalMode")
	}
	if profile.Safety.IsolationMode != "" {
		settings = append(settings, "task.isolation.mode")
	}
	sort.Strings(settings)
	opts := OMPModelCatalogProbeOptions{
		Executable: cliBinary, Runner: a.modelIntegrationRunner, Settings: settings,
	}
	probe := ProbeOMPModelCatalogForProfile(ctx, opts, profile)
	if probe.Status != "ready" || probe.Reason != "catalog_ready" {
		return OMPModelCatalogProbeResult{}, fmt.Errorf("model_catalog_unavailable: %s", probe.Reason)
	}
	supported := make(map[string]bool, len(probe.Settings))
	for _, setting := range probe.Settings {
		supported[setting.Key] = setting.Supported
	}
	for _, setting := range settings {
		if !supported[setting] {
			return OMPModelCatalogProbeResult{}, fmt.Errorf("model_setting_unsupported: %s", setting)
		}
	}
	return probe, nil
}
