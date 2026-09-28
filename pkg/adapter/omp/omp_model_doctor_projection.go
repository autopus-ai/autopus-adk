package omp

import (
	"fmt"

	"github.com/insajin/autopus-adk/pkg/config"
)

// OMPModelDoctorActivationExpectation is compiled from the same policy,
// catalog, projection, and safety inputs used by generation.
type OMPModelDoctorActivationExpectation struct {
	ConfigHash     string
	ConfigBytes    []byte
	ExpectedValues map[string]any
}

func CompileOMPModelDoctorActivationExpectation(
	profile config.RoleModelProfileConf,
	catalog OMPModelCatalog,
) (OMPModelDoctorActivationExpectation, error) {
	if err := validateOMPIntegrationOverrides(profile); err != nil {
		return OMPModelDoctorActivationExpectation{}, err
	}
	routes, err := bridgeOMPIntegrationRoutes(profile)
	if err != nil {
		return OMPModelDoctorActivationExpectation{}, err
	}
	DegradeUnservedDerivedOMPRoutes(profile, catalog, routes)
	routing := CompileOMPModelRouting(OMPModelRoutingInput{
		Catalog: catalog, CatalogReason: "catalog_ready", Routes: routes,
	})
	agents, err := projectOMPIntegrationAgents(catalog, routes, routing)
	if err != nil {
		return OMPModelDoctorActivationExpectation{}, err
	}
	projection, err := CompileOMPModelProjection(OMPModelProjectionInput{Agents: agents})
	if err != nil {
		return OMPModelDoctorActivationExpectation{}, err
	}
	overlay, err := OMPModelOverlayFromProjection(projection)
	if err != nil {
		return OMPModelDoctorActivationExpectation{}, err
	}
	configData, err := compileOMPIntegratedOverlay(overlay, profile.Safety)
	if err != nil {
		return OMPModelDoctorActivationExpectation{}, err
	}
	expected := ompIntegratedExpectedValues(overlay, profile.Safety)
	if len(expected) == 0 {
		return OMPModelDoctorActivationExpectation{}, fmt.Errorf("doctor projection is empty")
	}
	return OMPModelDoctorActivationExpectation{
		ConfigHash: OMPModelSHA256(configData), ConfigBytes: append([]byte(nil), configData...),
		ExpectedValues: expected,
	}, nil
}
