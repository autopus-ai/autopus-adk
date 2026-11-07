package cli

// SPEC-REVMODEL-001 T10 (REQ-RVM-18; acceptance S14): the three helps, both
// quality references (docs/quality.md, docs/quality.ko.md), and the v0.50.127 CHANGELOG entry document the review tab, the
// three keys it writes, the live check's most model calls, the CLI backend
// models, and that orchestra commands and --multi keep every provider.

import (
	"strings"
	"testing"
)

// reviewDocWants are the statements every document carries: the written
// keys, the call bound, the CLI backend, and the commands that keep every
// provider.
var reviewDocWants = []string{
	"spec.review_gate.providers", "orchestra.providers.<p>.model", "spec.review_gate.judge",
	"3n", "CLI backend", "auto spec review --multi", "brainstorm",
}

func TestReviewDocs_S14HelpsReadmesAndChangelog(t *testing.T) {
	changelog := readRepoFile(t, "CHANGELOG.md")
	unreleased := changelog[strings.Index(changelog, "## [Unreleased]"):]
	if end := strings.Index(unreleased[1:], "\n## ["); end >= 0 {
		unreleased = unreleased[:end+1]
	}
	docs := []struct {
		name, text, tab string
	}{
		{"auto update --help", newUpdateCmd().Long, composeReviewTextKO.Tab},
		{"auto init --help", newInitCmd().Long, composeReviewTextKO.Tab},
		{"auto quality --help", newQualityCmd().Long, composeReviewTextEN.Tab},
		{"docs/quality.md", readRepoFile(t, "docs", "quality.md"), composeReviewTextEN.Tab},
		{"docs/quality.ko.md", readRepoFile(t, "docs", "quality.ko.md"), composeReviewTextKO.Tab},
		{"CHANGELOG.md v0.50.127", unreleased, composeReviewTextKO.Tab},
	}
	for _, doc := range docs {
		assertComposeFormDocs(t, doc.name, doc.text, append([]string{doc.tab, "SPEC-REVMODEL-001"}, reviewDocWants...)...)
	}
}
