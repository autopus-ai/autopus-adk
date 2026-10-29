package cli

// SPEC-TUIUX-001 T7 (REQ-TUX-02, 04, 08, 14, 15; acceptance S9, S11–S14): Esc
// inside the composition step moves by the Navigation Contract's composition
// rows, through the real `auto quality` and `auto update` with the accessible
// helper, where a `b` line is Esc.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	tuiuxNotice      = "첫 단계입니다. 취소하려면 Ctrl+C를 누르세요."
	tuiuxFooter      = "답을 입력하고 Enter를 누르세요 · 뒤로: b"
	tuiuxFooterFirst = "답을 입력하고 Enter를 누르세요"
)

// tuiuxHeaders lists, in order, the lines of out that start with prefix.
func tuiuxHeaders(out, prefix string) []string {
	var headers []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			headers = append(headers, line)
		}
	}
	return headers
}

// tuiuxEnters is n Enter lines.
func tuiuxEnters(n int) []string {
	return strings.Split(strings.Repeat("\n", n), "")[:n]
}

func tuiuxLines(groups ...[]string) []string {
	var lines []string
	for _, group := range groups {
		lines = append(lines, group...)
	}
	return lines
}

// tuiuxQualityD runs `auto quality` over a fresh D with the accessible
// helper and returns the output, the NPD after it, and the project.
func tuiuxQualityD(t *testing.T, lines ...string) (string, string, string) {
	t.Helper()
	dir := tuiuxFixtureD(t)
	useNavComposeForms(t)
	out, err := runQualityCompose(t, dir, formInput(lines...))
	require.NoError(t, err, out)
	return out, tuiuxNPD(t, dir), dir
}

// S9, composition part: every form carries the path that reached it, and
// going back walks it back; the menu has the first-form footer.
func TestComposeNavBack_S9HeadersWalkThePath(t *testing.T) {
	newTUIUXEnv(t, true, "")
	out, _, _ := tuiuxQualityD(t, "2\n", "3\n", "1\n", "b\n", "b\n", "b\n", "\n")

	assert.Equal(t, []string{
		"quality · 모델 구성",
		"quality · 모델 구성 › 구성 편집",
		"quality · 모델 구성 › 구성 편집 › Codex",
		"quality · 모델 구성 › 구성 편집 › Codex › 역할 그룹",
		"quality · 모델 구성 › 구성 편집 › Codex",
		"quality · 모델 구성 › 구성 편집",
		"quality · 모델 구성",
	}, tuiuxHeaders(out, "quality · "))
	for i, footer := range regexp.MustCompile(`(?m)^quality · .*\n(.*)$`).FindAllStringSubmatch(out, -1) {
		want := tuiuxFooter
		if footer[0] == "quality · 모델 구성\n"+footer[1] {
			want = tuiuxFooterFirst
		}
		assert.Equal(t, want, footer[1], "form %d", i+1)
	}

	composeLanguage = func() string { return "en" }
	out, _, _ = tuiuxQualityD(t, "2\n", "3\n", "1\n", "b\n", "b\n", "b\n", "\n")
	assert.Equal(t, "quality · Model composition › edit the composition › Codex › role groups", tuiuxHeaders(out, "quality · ")[3])
	assert.Contains(t, out, "\nquality · Model composition\nType an answer and press Enter\n")
	assert.Contains(t, out, "\nquality · Model composition › edit the composition\nType an answer and press Enter · back: b\n")
}

// S11 (b), (c): Esc on the method shows the tool tab again with that tab
// selected; Esc on 기본 프리셋 고르기 returns to the menu without a write.
func TestComposeNavBack_S11ToolTabMethodAndPreset(t *testing.T) {
	newTUIUXEnv(t, true, "")
	before := tuiuxNPD(t, tuiuxFixtureD(t))

	out, npd, _ := tuiuxQualityD(t, "2\n", "3\n", "b\n", "\n", "b\n", "b\n", "\n")
	assert.Equal(t, []string{
		"quality · 모델 구성", "quality · 모델 구성 › 구성 편집", "quality · 모델 구성 › 구성 편집 › Codex",
		"quality · 모델 구성 › 구성 편집", "quality · 모델 구성 › 구성 편집 › Codex",
		"quality · 모델 구성 › 구성 편집", "quality · 모델 구성",
	}, tuiuxHeaders(out, "quality · "), "Enter on the shown-again tool tab picks Codex")
	assert.Equal(t, 2, strings.Count(out, "구성을 바꿀까요? \n"))
	assert.Equal(t, before, npd)

	out, npd, _ = tuiuxQualityD(t, "3\n", "b\n", "\n")
	assert.NotRegexp(t, `(?m)^quality\.default = \S+$`, out, "no preset write line")
	assert.Contains(t, out, "quality · 모델 구성 › 기본 프리셋 고르기\n")
	assert.Equal(t, before, npd)
}

// S12 (a): Esc on 적용할까요? is 계속 편집; the run then applies what the
// same answers without Esc apply.
func TestComposeNavBack_S12ConfirmationEscKeepsEditing(t *testing.T) {
	newTUIUXEnv(t, true, "")
	head := tuiuxLines([]string{"2\n", "2\n", "1\n", "3\n"}, tuiuxEnters(7))
	out, npd, dir := tuiuxQualityD(t, tuiuxLines(head, []string{"b\n", "1\n"}, tuiuxEnters(8), []string{"2\n", "\n"})...)
	_, control, _ := tuiuxQualityD(t, tuiuxLines(head, []string{"2\n", "\n"})...)

	assert.Equal(t, 2, strings.Count(out, "어떻게 고칠까요? \n"))
	assert.Equal(t, 2, strings.Count(out, "적용할까요? \n"))
	assert.NotContains(t, out, composeTextKO.Cancelled)
	assert.Equal(t, control, npd)
	assert.Equal(t, "opus", composeBindLoad(t, dir).Quality.Presets["claude-custom"].Agents["planner"])
}

// S12 (b): Esc on the override question is 계속 편집 with the edit kept.
func TestComposeNavBack_S12OverrideEscKeepsEditing(t *testing.T) {
	newTUIUXEnv(t, true, "")
	useNavComposeForms(t)
	run := func(lines ...string) (string, string) {
		dir := writeComposeProjectP(t)
		out, err := runQualityForms(t, dir, nil, formInput(lines...))
		require.NoError(t, err, out)
		return out, tuiuxNPD(t, dir)
	}
	out, npd := run(tuiuxLines(composeFormS16, []string{"b\n", "\n"}, tuiuxEnters(4), []string{"n\n", "2\n", "\n"})...)
	_, control := run(tuiuxLines(composeFormS16, []string{"n\n", "2\n", "\n"})...)

	assert.Equal(t, 2, strings.Count(out, composeFormOverrideQuestion))
	assert.Equal(t, control, npd)
}

// S14 (a), (b): the update and quality menus are first forms: `b` prints the
// notice once and the next line answers the menu.
func TestComposeNavBack_S14MenuIsAFirstForm(t *testing.T) {
	newTUIUXEnv(t, true, "")
	before := tuiuxNPD(t, tuiuxFixtureD(t))
	out, npd, _ := tuiuxQualityD(t, "b\n", "\n")
	assert.Equal(t, 1, strings.Count(out, tuiuxNotice+"\n"))
	assert.Equal(t, 1, strings.Count(out, "에이전트 모델 구성 ─"), "the table is drawn once")
	assert.Equal(t, before, npd)

	update := func(lines ...string) string {
		dir := tuiuxFixtureD(t)
		cfg, err := config.Load(dir)
		require.NoError(t, err)
		off := false
		cfg.Features.Rea.Enabled = &off // a decided project: update asks no REA line first
		require.NoError(t, config.Save(dir, cfg))
		useNavComposeForms(t)
		out, err := runComposeUpdate(t, formInput(lines...), "--dir", dir)
		require.NoError(t, err, out)
		return strings.ReplaceAll(out, dir, "<DIR>")
	}
	withBack, control := update("b\n", "\n"), update("\n")
	assert.Contains(t, withBack, "update · 모델 구성\n"+tuiuxFooterFirst+"\n")
	assert.Equal(t, control, strings.Replace(withBack, tuiuxNotice+"\n", "", 1))
	assert.NotEqual(t, control, withBack)
}

// tuiuxOMPRun is one `auto quality` over an OMP fixture with the accessible
// helper and the OMP deps, and the project digest before and after it.
func tuiuxOMPRun(t *testing.T, dir string, catalog []byte, lines ...string) (string, *composeOMPActivations) {
	t.Helper()
	deps, activations := composeOMPDeps(catalog)
	useNavComposeForms(t)
	before := tuiuxNPD(t, dir)
	out, err := runQualityForms(t, dir, &deps, formInput(lines...))
	require.NoError(t, err, out)
	assert.Equal(t, before, tuiuxNPD(t, dir), "autopus.yaml and .omp stay byte-identical")
	return out, activations
}

// S13: Esc on any form of the OMP tab or the review tab returns to the tool
// tab with nothing written, nothing activated, and no warning line.
func TestComposeNavBack_S13TabsGoBackToTheToolTab(t *testing.T) {
	ompML2 := []byte(composeOMPCatalogML2)
	cases := []struct {
		name    string
		fixture func(*testing.T) string
		catalog []byte
		lines   []string
		header  string
	}{
		{"(a) O3 first form", composeO3, ompML2, []string{"2\n", "4\n", "b\n", "b\n", "\n"}, "OMP"},
		{"(c) RV first form", tuiuxReviewRV, []byte(reviewCatalogCATG), []string{"2\n", "5\n", "b\n", "b\n", "\n"}, "멀티프로바이더 리뷰"},
		{"(d) RV second form", tuiuxReviewRV, []byte(reviewCatalogCATG), []string{"2\n", "5\n", "\n", "b\n", "b\n", "\n"}, "멀티프로바이더 리뷰"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newTUIUXEnv(t, true, "")
			out, activations := tuiuxOMPRun(t, tc.fixture(t), tc.catalog, tc.lines...)

			assert.NotContains(t, out, "경고:")
			assert.Equal(t, 2, strings.Count(out, "어느 도구를 바꿀까요? \n"), out)
			assert.Contains(t, out, "quality · 모델 구성 › 구성 편집 › "+tc.header+"\n")
			assert.Zero(t, activations.calls)
			if strings.HasPrefix(tc.name, "(b) ") {
				list := strings.Index(out, "에이전트를 고르세요 \n")
				model := strings.Index(out, "\ntask (")
				tab := strings.LastIndex(out, "어느 도구를 바꿀까요? \n")
				require.True(t, list >= 0 && list < model && model < tab, out)
				assert.Equal(t, "\nquality · 모델 구성 › 구성 편집\n"+tuiuxFooter+"\n",
					out[strings.LastIndex(out[:tab], "\nquality · "):tab], "the tool tab comes right after the model form")
			}
		})
	}
	t.Run("shown-again tool tab selects the tab", func(t *testing.T) {
		newTUIUXEnv(t, true, "")
		out, _ := tuiuxOMPRun(t, composeO3(t), ompML2, "2\n", "4\n", "b\n", "\n", "b\n", "b\n", "\n")
		assert.Equal(t, 2, strings.Count(out, "quality · 모델 구성 › 구성 편집 › OMP\n"), "Enter on the tool tab opened OMP again")
	})
}

// tuiuxReviewRV is RV: R3, whose review tab lists three OMP providers.
func tuiuxReviewRV(t *testing.T) string {
	t.Helper()
	return writeReviewR3(t)
}

// The refresh path: 최신 기본값으로 갱신's confirmation and the 모든 도구 같게
// pass it opens carry the refresh in their path; Esc on the confirmation
// keeps editing, and Esc on the method returns to the menu, since no tool
// tab was asked, with nothing written.
func TestComposeNavBack_RefreshPathHeadersAndBack(t *testing.T) {
	newTUIUXEnv(t, true, "")
	dir, _ := composeSessionProject(t, "R")
	before := tuiuxNPD(t, dir)
	useNavComposeForms(t)

	out, err := runQualityCompose(t, dir, formInput("2\n", "b\n", "1\n", "b\n", "b\n", "\n"))

	require.NoError(t, err, out)
	assert.Equal(t, []string{
		"quality · 모델 구성",
		"quality · 모델 구성 › 최신 기본값으로 갱신",
		"quality · 모델 구성 › 최신 기본값으로 갱신 › 모든 도구 같게",
		"quality · 모델 구성 › 최신 기본값으로 갱신 › 모든 도구 같게 › 역할 그룹",
		"quality · 모델 구성 › 최신 기본값으로 갱신 › 모든 도구 같게",
		"quality · 모델 구성",
	}, tuiuxHeaders(out, "quality · "))
	assert.Equal(t, 2, strings.Count(out, "구성을 바꿀까요? \n"))
	assert.NotContains(t, out, composeTextKO.Cancelled)
	assert.Equal(t, before, tuiuxNPD(t, dir))
}
