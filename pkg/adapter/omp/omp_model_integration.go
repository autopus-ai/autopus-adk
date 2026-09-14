package omp

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
)

type ompModelIntegration struct {
	profileName string
	profile     config.RoleModelProfileConf
	probe       OMPModelCatalogProbeResult
	routing     OMPModelRoutingCompilation
	projection  OMPModelProjection
}

// WithModelIntegrationRunner injects metadata and config-readback execution.
func (a *Adapter) WithModelIntegrationRunner(runner OMPModelCatalogRunner) *Adapter {
	a.modelIntegrationRunner = runner
	return a
}

// WithModelIntegrationClock makes generated_at testable without changing its digest.
func (a *Adapter) WithModelIntegrationClock(clock func() time.Time) *Adapter {
	a.modelIntegrationClock = clock
	return a
}

// @AX:WARN [AUTO]: model integration preparation contains 8 if branches.
// @AX:REASON [AUTO]: policy opt-in, probe evidence, catalog normalization, routing, and projection preparation are fail-closed.
func (a *Adapter) prepareModelIntegration(
	ctx context.Context,
	cfg *config.HarnessConfig,
) (*ompModelIntegration, error) {
	if cfg.RoleModelPolicy.Profile == "" {
		return nil, nil
	}
	if err := cfg.RoleModelPolicy.Validate(); err != nil {
		return nil, err
	}
	profileName, profile, ok := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
	if !ok {
		return nil, fmt.Errorf("role_model_policy.profile_unknown: %q", profileName)
	}
	if err := validateOMPIntegrationOverrides(profile); err != nil {
		return nil, err
	}
	probe, err := a.probeIntegratedModelCatalog(ctx, profile)
	if err != nil {
		return nil, err
	}
	routes, err := bridgeOMPIntegrationRoutes(profile)
	if err != nil {
		return nil, err
	}
	routing := CompileOMPModelRouting(OMPModelRoutingInput{
		Catalog: probe.Catalog, CatalogReason: probe.Reason, Routes: routes,
	})
	projected, err := projectOMPIntegrationAgents(probe.Catalog, routes, routing)
	if err != nil {
		return nil, err
	}
	projection, err := CompileOMPModelProjection(OMPModelProjectionInput{Agents: projected})
	if err != nil {
		return nil, err
	}
	return &ompModelIntegration{
		profileName: profileName, profile: profile, probe: probe,
		routing: routing, projection: projection,
	}, nil
}

func isOwnerOnlyOMPModelPath(path string) bool {
	normalized := filepath.ToSlash(filepath.Clean(path))
	return normalized == configFile || normalized == DefaultOMPModelOverlayPath ||
		normalized == OMPModelReceiptRelativePath ||
		normalized == OMPModelProjectOwnershipRelativePath
}
