package content

import (
	"path/filepath"
	"testing"
)

func TestProviderQualityOperatorDocsExposeIndependentModes(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Dir(repoContentDir(t))
	files := map[string][]string{
		"README.md": {
			"providers:\n    claude: ultra\n    codex: balanced",
			"Provider-specific `--apply` refreshes only that configured platform",
		},
		"docs/README.ko.md": {
			"providers:\n    claude: ultra\n    codex: balanced",
			"Provider별 `--apply`는 해당 configured platform만 갱신",
		},
	}
	for relativePath, fragments := range files {
		document := readOpus5ContractFile(t, repoRoot, relativePath)
		assertOpus5ContractFragments(t, relativePath, document, fragments)
	}
}
