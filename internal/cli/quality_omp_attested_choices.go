package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
)

// installedOMPAttestedModelChoices lists the installed models the autopus OMP
// tab offers (SPEC-MODELUX-003 REQ-MUX3-13): a strict catalog's available
// models with their reported family and capabilities, or on a metadata-light
// catalog only the models declared names, under the declared family, after
// one note line. Every provider is listed together, ordered by selector.
// light reports the metadata-light catalog, where only declarations attest.
func installedOMPAttestedModelChoices(
	ctx context.Context,
	out io.Writer,
	runner omp.OMPModelCatalogRunner,
	declared map[string]string,
	note string,
) (choices []ompCustomModelChoice, light bool, err error) {
	strict, display, _, err := readInstalledOMPModelChoices(ctx, runner)
	if err != nil || display == nil {
		return strict, false, err
	}
	fmt.Fprintln(out, note)
	attested := make([]ompCustomModelChoice, 0, len(display))
	for _, choice := range display {
		if family := declared[choice.selector()]; family != "" {
			choice.Family = family
			attested = append(attested, choice)
		}
	}
	return attested, true, nil
}

// readInstalledOMPModelChoices reads what the installation ships: the strict
// catalog's choices, or, when it reports no metadata, the display catalog's.
// Any other probe failure is returned so no choice is made from a guess.
func readInstalledOMPModelChoices(
	ctx context.Context,
	runner omp.OMPModelCatalogRunner,
) ([]ompCustomModelChoice, []ompCustomModelChoice, string, error) {
	probe, err := probeInstalledOMPCatalog(ctx, runner)
	version := safeOMPOperatorVersion(probe.Version)
	switch {
	case err == nil:
		return sortedOMPCustomModelChoices(strictOMPCustomModelChoices(probe.Catalog.Models)), nil, version, nil
	case probe.Reason != ompRawCatalogFallbackReason:
		return nil, nil, version, err
	}
	body, runErr := runner.Run(ctx, "omp", "models", "--json", "--no-extensions")
	if runErr != nil {
		return nil, nil, version, runErr
	}
	models, reason := normalizeOMPRawDisplayCatalog(body, ompModelDoctorProbeOutput)
	if reason != "catalog_ready" {
		return nil, nil, version, ompCatalogUnavailableError{reason: reason}
	}
	return nil, sortedOMPCustomModelChoices(displayOMPCustomModelChoices(models)), version, nil
}
