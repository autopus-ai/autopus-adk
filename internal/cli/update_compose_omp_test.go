package cli

// SPEC-MODELUX-002 T6, probe A3 (REQ-MUX2-11; acceptance S8): an OMP pin
// confirmed inside the update's composition step survives the same run's
// later saves and reaches its OMP generation, because the step syncs the
// loaded config from the file it wrote.

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
)

// useComposeUpdateOMPDeps replaces the OMP dependencies of the update's
// composition step for one test.
func useComposeUpdateOMPDeps(t *testing.T, deps ompPlatformDependencies) {
	t.Helper()
	original := updateComposeOMPDeps
	t.Cleanup(func() { updateComposeOMPDeps = original })
	updateComposeOMPDeps = deps
}

// usePlatformOMPModelRunner makes the OMP platform generation read the
// catalog and its written config through runner instead of an omp binary.
func usePlatformOMPModelRunner(t *testing.T, runner omp.OMPModelCatalogRunner) {
	t.Helper()
	original := platformOMPModelRunner
	t.Cleanup(func() { platformOMPModelRunner = original })
	platformOMPModelRunner = runner
}
