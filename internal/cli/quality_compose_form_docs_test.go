package cli

// SPEC-MODELUX-002 T10 (REQ-MUX2-19, REQ-MUX2-21; acceptance S18, S19): the
// three helps, both READMEs, and the v0.50.125 CHANGELOG entry document the
// arrow-key forms, the tool tab, the per-tool binding, the OMP tab and where
// its condition is read, the label notation, Ctrl+C, and the OSC 11 query.
// They are added to the SPEC-MODELUX-001 text, which its own docs tests
// still pin.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func assertComposeFormDocs(t *testing.T, name, text string, wants ...string) {
	t.Helper()
	text = flowText(text)
	for _, want := range wants {
		assert.Contains(t, text, want, "%s is missing %q", name, want)
	}
}

func TestComposeFormDocs_HelpsDocumentTheForms(t *testing.T) {
	update := newUpdateCmd()
	assertComposeFormDocs(t, "auto update --help", update.Long,
		"화살표와 Enter", "도구 탭", "<provider>-custom", "quality.providers.<provider>", "opus-5-5",
		"autopus.yaml에 저장된 platforms에 omp", "Ctrl+C", "update 전체를 끝냅니다", "감지한 플랫폼")
	assertComposeFormDocs(t, "auto init --help", newInitCmd().Long,
		"화살표와 Enter", "도구 탭", "<provider>-custom", "quality.providers.<provider>", "opus-5-5",
		"이번 init에서 고른 platforms에 omp", "Ctrl+C", "init cancelled.")
	assertComposeFormDocs(t, "auto quality --help", newQualityCmd().Long,
		"arrow keys and Enter", "tool tab", "<provider>-custom", "quality.providers.<provider>", "opus-5-5",
		"platforms saved in autopus.yaml include omp", "Ctrl+C", "choose the default preset")
}

func TestComposeFormDocs_ReadmesAndChangelogDocumentTheForms(t *testing.T) {
	assertComposeFormDocs(t, "README.md", readRepoFile(t, "README.md"),
		"arrow keys and Enter", "same for all tools", "<provider>-custom", "quality.providers.<provider>",
		"opus-5-5", "platforms saved in autopus.yaml include omp", "the platforms this init chose include omp", "Ctrl+C")
	assertComposeFormDocs(t, "docs/README.ko.md", readRepoFile(t, "docs", "README.ko.md"),
		"화살표와 Enter", "모든 도구 같게", "<provider>-custom", "quality.providers.<provider>",
		"opus-5-5", "autopus.yaml에 저장된 platforms에 omp", "이번 init에서 고른 platforms에 omp", "Ctrl+C")
	changelog := readRepoFile(t, "CHANGELOG.md")
	unreleased := changelog[strings.Index(changelog, "## [Unreleased]"):]
	if end := strings.Index(unreleased[1:], "\n## ["); end >= 0 {
		unreleased = unreleased[:end+1]
	}
	assertComposeFormDocs(t, "CHANGELOG.md v0.50.125", unreleased,
		"SPEC-MODELUX-002", "화살표와 Enter", "도구 탭", "codex-custom", "OMP 탭", "opus-5-5",
		"감지한 플랫폼", "OSC 11", "bubbletea v1.3.6", "tea_init.go", "init()")
}
