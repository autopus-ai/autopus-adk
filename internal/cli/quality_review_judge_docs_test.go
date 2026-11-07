package cli

// SPEC-JUDGEMODEL-001 S10 (REQ-JM-DOC-01): the helps, both READMEs, the
// CHANGELOG's unreleased section document the judge model. The skill sources
// that also did (spec-review, using-autopus, adaptive-quality) are no longer
// installed.

import (
	"strings"
	"testing"
)

func TestJudgeModelDocs(t *testing.T) {
	changelog := readRepoFile(t, "CHANGELOG.md")
	unreleased := changelog[strings.Index(changelog, "## [Unreleased]"):]
	if end := strings.Index(unreleased[1:], "\n## ["); end >= 0 {
		unreleased = unreleased[:end+1]
	}
	wants := []string{"spec.review_gate.judge_model", "judge_effort", "SPEC-JUDGEMODEL-001", "--model"}
	for _, doc := range []struct{ name, text string }{
		{"auto quality --help", newQualityCmd().Long},
		{"auto quality review --help", qualityReviewCmdLong},
		{"README.md", readRepoFile(t, "README.md")},
		{"docs/README.ko.md", readRepoFile(t, "docs", "README.ko.md")},
		{"CHANGELOG.md unreleased", unreleased},
	} {
		docWants := wants
		if doc.name == "auto quality --help" {
			docWants = wants[:3]
		}
		assertComposeFormDocs(t, doc.name, doc.text, docWants...)
	}
}
