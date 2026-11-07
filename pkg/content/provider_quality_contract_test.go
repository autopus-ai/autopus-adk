package content

import (
	"path/filepath"
	"testing"
)

func TestProviderQualityOperatorDocsExposeIndependentModes(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Dir(repoContentDir(t))
	files := map[string][]string{
		// The quality reference moved out of the READMEs into docs/quality*.md.
		"docs/quality.md": {
			"providers:\n    claude: ultra\n    codex: balanced",
			"Provider-specific\n`--apply` refreshes only that platform",
		},
		"docs/quality.ko.md": {
			"providers:\n    claude: ultra\n    codex: balanced",
			"Provider별 `--apply`는 그 플랫폼만",
		},
	}
	for relativePath, fragments := range files {
		document := readOpus5ContractFile(t, repoRoot, relativePath)
		assertOpus5ContractFragments(t, relativePath, document, fragments)
	}
}
