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
		return OMPModelCatalogProbeResult{}, ompCatalogUnavailableError(probe.Reason)
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

// ompCatalogUnavailableError keeps the machine-readable reason first and adds
// the operator's next step for the reasons that have one.
func ompCatalogUnavailableError(reason string) error {
	err := fmt.Errorf("model_catalog_unavailable: %s", reason)
	switch reason {
	case "catalog_timeout":
		return fmt.Errorf("%w (`omp models --json --no-extensions` did not answer within %s; run it once to check OMP, then retry)",
			err, defaultOMPModelProbeTimeout)
	case "identity_unverified":
		return fmt.Errorf("%w (`omp --version` did not identify the Oh My Pi CLI; check the omp on PATH)", err)
	}
	return err
}
