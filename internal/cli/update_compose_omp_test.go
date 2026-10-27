package cli

// SPEC-MODELUX-002 T6, probe A3 (REQ-MUX2-11; acceptance S8): an OMP pin
// confirmed inside the update's composition step survives the same run's
// later saves and reaches its OMP generation, because the step syncs the
// loaded config from the file it wrote.

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
)

// composeUpdateS8 answers S8 in `auto update` over O3: 구성 편집, OMP,
// 에이전트별 지정, GPT, task, openai-codex/gpt-6-astra, max, 완료, 적용,
// 유지. The update menu has no default preset choice.
var composeUpdateS8 = []string{"2\n", "4\n", "4\n", "1\n", "5\n", "3\n", "2\n", "\n", "2\n", "\n"}

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
