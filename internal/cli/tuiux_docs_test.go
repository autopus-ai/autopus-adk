package cli

// SPEC-TUIUX-001 T11 (REQ-TUX-18; acceptance S19): the three helps, docs/quality.md,
// and the v0.50.127 CHANGELOG entry document Esc back, the accessible token,
// the first-step notice, the frame, and the init review with what it leaves
// out. The strings come from the code that prints them.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

func TestTUIUXDocs_S19HelpsReadmeAndChangelog(t *testing.T) {
	changelog := readRepoFile(t, "CHANGELOG.md")
	unreleased := changelog[strings.Index(changelog, "## [Unreleased]"):]
	if end := strings.Index(unreleased[1:], "\n## ["); end >= 0 {
		unreleased = unreleased[:end+1]
	}
	ko := []string{
		"SPEC-TUIUX-001", "Esc", "`b`·`back`·`뒤로`", "TERM=dumb", "첫 단계입니다. 취소하려면 Ctrl+C를 누르세요.",
		"방향키 이동 · Enter 선택 · Esc 뒤로 · Ctrl+C 취소", "적용할 설정", "적용", "뒤로", "취소",
	}
	en := []string{
		"SPEC-TUIUX-001", "Esc", "TERM=dumb", "This is the first step; press Ctrl+C to cancel.",
		"Arrows move · Enter select · Esc back · Ctrl+C cancel", "Chosen so far", "Settings to apply",
		"apply", "back", "cancel",
	}
	review := []string{"statusLine", "parent rule", "features.rea"}
	for _, doc := range []struct {
		name, text string
		wants      []string
	}{
		{"auto init --help", newInitCmd().Long, append(append(ko, "지금까지 고른 값", "init · 4/6 리뷰 게이트", "이대로 진행할까요?",
			"답한 값", "REA 결정", "orchestra 정규화"), review...)},
		{"auto update --help", newUpdateCmd().Long, append(ko, "지금까지 고른 값", "update · 모델 구성 › 구성 편집 › Codex › 역할 그룹")},
		{"auto quality --help", newQualityCmd().Long, append(en, "b, back, or 뒤로", "quality · Model composition › edit the composition › Codex")},
		{"docs/quality.md", readRepoFile(t, "docs", "quality.md"), append(append(en, "`b`, `back`, or `뒤로`", "Proceed with these settings?",
			"init · 4/6 Review Gate", "Answers", "Changed settings", "REA MCP: on · add", "orchestra normalization"), review...)},
		{"CHANGELOG.md v0.50.127", unreleased, append(append(ko, "지금까지 고른 값", "이대로 진행할까요?", "orchestra 정규화"), review...)},
	} {
		text := flowText(doc.text)
		for _, want := range doc.wants {
			assert.Contains(t, text, flowText(want), "%s is missing %q", doc.name, want)
		}
	}
	// The documented notice and footers are the ones the helper prints.
	var out strings.Builder
	value := "a"
	form := composeDocsSelect(&value)
	_ = tui.RunNavAccessible(form, tui.NavFrame{First: true, Lang: "ko"}, strings.NewReader("b\n"), &out)
	assert.Contains(t, out.String(), "첫 단계입니다. 취소하려면 Ctrl+C를 누르세요.\n")
}

func composeDocsSelect(value *string) *huh.Form {
	return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Title("Q").Options(huh.NewOptions("a", "b")...).Value(value)))
}
