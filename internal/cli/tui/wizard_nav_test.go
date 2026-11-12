package tui_test

// SPEC-TUIUX-001 T5 (REQ-TUX-02, 03, 04, 08, 10, 16, 17; acceptance S1–S3,
// S5, S9, S16, S21): the wizard runs its steps as one navigation flow
// through InitWizardOpts.Runner, here the accessible helper over scripted
// lines.

import (
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

// wizardWZ6 is WZ6: Claude Code and Codex with two providers, six steps.
func wizardWZ6() tui.InitWizardOpts {
	return tui.InitWizardOpts{Platforms: []string{"claude-code", "codex"}, Providers: []string{"claude", "codex"}}
}

// wizardLocale fixes the system language the wizard reads.
func wizardLocale(t *testing.T, lang string) {
	t.Helper()
	t.Setenv("LANG", map[string]string{"ko": "ko_KR.UTF-8", "en": "en_US.UTF-8"}[lang])
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE"} {
		t.Setenv(key, "")
	}
}

// runWizardNav runs the wizard with the accessible helper over lines, one
// byte per read, and returns the result and everything it printed.
func runWizardNav(t *testing.T, opts tui.InitWizardOpts, lines ...string) (*tui.InitWizardResult, string) {
	t.Helper()
	in := iotest.OneByteReader(strings.NewReader(strings.Join(lines, "")))
	var out strings.Builder
	opts.Runner = func(form *huh.Form, frame tui.NavFrame) error {
		return tui.RunNavAccessible(form, frame, in, &out)
	}
	result, err := tui.RunInitWizard(opts)
	require.NoError(t, err)
	return result, out.String()
}

func reaOn(on bool) *bool { return &on }

// linesAfter returns the n lines after the occurrence-th line equal to line,
// counted from 1.
func linesAfter(t *testing.T, out, line string, occurrence, n int) []string {
	t.Helper()
	lines := strings.Split(out, "\n")
	seen := 0
	for i, candidate := range lines {
		if candidate != line {
			continue
		}
		if seen++; seen == occurrence {
			require.LessOrEqual(t, i+1+n, len(lines))
			return lines[i+1 : i+1+n]
		}
	}
	t.Fatalf("line %q appears %d times, not %d:\n%s", line, seen, occurrence, out)
	return nil
}

func countLines(out, line string) int {
	return strings.Count("\n"+out, "\n"+line+"\n")
}

const (
	wizardFooterKO      = "답을 입력하고 Enter를 누르세요 · 뒤로: b"
	wizardFooterFirstKO = "답을 입력하고 Enter를 누르세요"
)

// S1: back from the quality step restores the language answers, which the
// shown-again step preselects, and the result equals the run without back.
func TestWizardNav_S1BackPreselectsAndKeepsTheValues(t *testing.T) {
	wizardLocale(t, "ko")
	result, out := runWizardNav(t, wizardWZ6(), "2\n", "2\n", "1\n", "3\n", "b\n", "\n", "\n", "\n", "1\n", "n\n", "\n", "2\n")
	control, _ := runWizardNav(t, wizardWZ6(), "2\n", "2\n", "1\n", "3\n", "1\n", "n\n", "\n", "2\n")

	assert.Equal(t, &tui.InitWizardResult{
		UsageProfile: "fullstack", CommentsLang: "ko", CommitsLang: "en", AILang: "ja",
		Quality: "ultra", ReviewGate: false, Rea: reaOn(false), Methodology: "none",
	}, result)
	assert.Equal(t, control, result)
	for header, want := range map[string]int{
		"init · 1/6 사용 프로필": 1, "init · 2/6 언어 설정": 2, "init · 3/6 품질 모드": 2, "init · 4/6 리뷰 게이트": 1,
	} {
		assert.Equal(t, want, countLines(out, header), header)
	}
	assert.Equal(t, []string{"지금까지 고른 값", "  사용 프로필: Fullstack", wizardFooterKO},
		linesAfter(t, out, "init · 2/6 언어 설정", 2, 3))
	assert.Equal(t, []string{
		"지금까지 고른 값", "  사용 프로필: Fullstack", "  언어 설정: 주석 ko · 커밋 en · AI 응답 ja", "  품질 모드: Ultra", wizardFooterKO,
	}, linesAfter(t, out, "init · 4/6 리뷰 게이트", 1, 5))
}

// S2: back on the language step's second select drops its first answer.
func TestWizardNav_S2BackDropsThePartialStep(t *testing.T) {
	wizardLocale(t, "ko")
	result, out := runWizardNav(t, wizardWZ6(), "2\n", "4\n", "b\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n")

	assert.Equal(t, &tui.InitWizardResult{
		UsageProfile: "fullstack", CommentsLang: "en", CommitsLang: "ko", AILang: "ko",
		Quality: "balanced", ReviewGate: true, Rea: reaOn(false), Methodology: "tdd",
	}, result)
	assert.Equal(t, 2, countLines(out, "init · 1/6 사용 프로필"))
	for occurrence := 1; occurrence <= 2; occurrence++ {
		assert.Equal(t, []string{wizardFooterFirstKO}, linesAfter(t, out, "init · 1/6 사용 프로필", occurrence, 1))
	}
}

// S3, wizard part: back on the first step stays with one notice.
func TestWizardNav_S3FirstStepNotice(t *testing.T) {
	for lang, notice := range map[string]string{
		"ko": "첫 단계입니다. 취소하려면 Ctrl+C를 누르세요.",
		"en": "This is the first step; press Ctrl+C to cancel.",
	} {
		t.Run(lang, func(t *testing.T) {
			wizardLocale(t, lang)
			result, out := runWizardNav(t, wizardWZ6(), "b\n", "1\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n")

			assert.Equal(t, 1, strings.Count(out, notice), out)
			header := map[string]string{"ko": "init · 1/6 사용 프로필", "en": "init · 1/6 Usage Profile"}[lang]
			assert.Equal(t, 1, countLines(out, header))
			assert.Equal(t, &tui.InitWizardResult{
				UsageProfile: "developer", CommentsLang: "en", CommitsLang: lang, AILang: lang,
				Quality: "balanced", ReviewGate: true, Rea: reaOn(false), Methodology: "tdd",
			}, result)
		})
	}
}

// wizardR is S5's resume result.
func wizardR() *tui.InitWizardResult {
	return &tui.InitWizardResult{
		UsageProfile: "fullstack", CommentsLang: "ko", CommitsLang: "en", AILang: "ja",
		Quality: "ultra", ReviewGate: false, Rea: reaOn(false), Methodology: "none",
	}
}

// S5: Resume starts at the last step with every earlier answer summarized.
func TestWizardNav_S5ResumeStartsAtTheLastStep(t *testing.T) {
	wizardLocale(t, "ko")
	opts := wizardWZ6()
	opts.Resume = wizardR()

	result, out := runWizardNav(t, opts, "\n")
	assert.Equal(t, wizardR(), result)
	assert.True(t, strings.HasPrefix(out, "init · 6/6 방법론\n"), out)
	assert.Equal(t, []string{
		"지금까지 고른 값", "  사용 프로필: Fullstack", "  언어 설정: 주석 ko · 커밋 en · AI 응답 ja", "  품질 모드: Ultra",
		"  리뷰 게이트: 끔", "  REA 역공학 MCP: 끔", wizardFooterKO,
	}, linesAfter(t, out, "init · 6/6 방법론", 1, 7))

	result, out = runWizardNav(t, opts, "b\n", "\n", "\n")
	assert.Equal(t, wizardR(), result)
	rea := strings.Index(out, "init · 5/6 REA 역공학 MCP\n")
	require.Positive(t, rea, out)
	assert.Contains(t, out[rea:], "init · 6/6 방법론\n")
	assert.Equal(t, 2, countLines(out, "init · 6/6 방법론"))
}

// S9, wizard part: the header's n/N is the form title's [n/N].
func TestWizardNav_S9HeaderMatchesTheTitle(t *testing.T) {
	wz5 := wizardWZ6()
	wz5.ExistingProfile = "developer"
	for _, tc := range []struct {
		name, lang     string
		opts           tui.InitWizardOpts
		header, title  string
		firstAfter     string
		firstHeaderKey string
	}{
		{"WZ6 ko", "ko", wizardWZ6(), "init · 4/6 리뷰 게이트", "[4/6] Review Gate", wizardFooterFirstKO, "init · 1/6 사용 프로필"},
		{"WZ6 en", "en", wizardWZ6(), "init · 4/6 Review Gate", "[4/6] Review Gate", "Type an answer and press Enter", "init · 1/6 Usage Profile"},
		{"WZ5 ko", "ko", wz5, "init · 3/5 리뷰 게이트", "[3/5] Review Gate", wizardFooterFirstKO, "init · 1/5 언어 설정"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wizardLocale(t, tc.lang)
			_, out := runWizardNav(t, tc.opts)

			header := strings.Index(out, tc.header+"\n")
			require.GreaterOrEqual(t, header, 0, out)
			assert.Contains(t, out[header:], tc.title)
			assert.Equal(t, []string{tc.firstAfter}, linesAfter(t, out, tc.firstHeaderKey, 1, 1))
			back := map[string]string{"ko": wizardFooterKO, "en": "Type an answer and press Enter · back: b"}[tc.lang]
			assert.Contains(t, out, tc.header+"\n")
			assert.Contains(t, linesAfter(t, out, tc.header, 1, 6), back)
		})
	}
}

// S16, wizard part: Ctrl+C in a step cancels the wizard as before.
func TestWizardNav_S16InterruptCancels(t *testing.T) {
	wizardLocale(t, "ko")
	opts := wizardWZ6()
	forms := 0
	opts.Runner = func(form *huh.Form, frame tui.NavFrame) error {
		if forms++; forms == 3 {
			return huh.ErrUserAborted
		}
		return tui.RunNavAccessible(form, frame, strings.NewReader("\n"), &strings.Builder{})
	}
	result, err := tui.RunInitWizard(opts)
	require.NoError(t, err)
	assert.True(t, result.Cancelled)
}

// S21, wizard part: the default runner keeps huh's streams.
func TestWizardNav_S21DefaultStreams(t *testing.T) {
	in, out := tui.TestWizardStreams(true)
	assert.Equal(t, os.Stdin, in)
	assert.Equal(t, os.Stdout, out)
	in, out = tui.TestWizardStreams(false)
	assert.Equal(t, os.Stdin, in)
	assert.Equal(t, os.Stderr, out)
}

// The init review's answers come from the steps the wizard asked.
func TestWizardAnswerLines_StepsTheWizardAsked(t *testing.T) {
	w := &tui.InitWizardResult{
		UsageProfile: "developer", CommentsLang: "en", CommitsLang: "ko", AILang: "ko",
		Quality: "balanced", ReviewGate: false, Rea: reaOn(false), Methodology: "tdd",
	}
	assert.Equal(t, []string{
		"  사용 프로필: Developer", "  언어 설정: 주석 en · 커밋 ko · AI 응답 ko", "  품질 모드: Balanced",
		"  리뷰 게이트: 끔", "  REA 역공학 MCP: 끔", "  방법론: TDD",
	}, tui.WizardAnswerLines(wizardWZ6(), w, "ko"))

	w.Quality = tui.QualityRoleGroups
	opts := tui.InitWizardOpts{ExistingProfile: "developer", NoReviewGate: true, Platforms: []string{"opencode"}}
	assert.Equal(t, []string{
		"  Language Settings: comments en · commits ko · AI responses ko", "  Quality Mode: Advanced", "  Methodology: TDD",
	}, tui.WizardAnswerLines(opts, w, "en"))
}

// The summary values are the step options' labels before " — ", and the
// wizard asks exactly the steps buildStepList builds.
func TestWizardNav_SummaryValuesAreTheOptionLabels(t *testing.T) {
	wizardLocale(t, "en")
	for _, profile := range []string{"developer", "fullstack"} {
		for _, methodology := range []string{"tdd", "none"} {
			r := &tui.InitWizardResult{UsageProfile: profile, Methodology: methodology, Quality: "ultra", Rea: reaOn(false)}
			lines := tui.WizardAnswerLines(wizardWZ6(), r, "en")
			require.Len(t, lines, len(tui.TestBuildStepList(wizardWZ6())))
			for i, form := range []*huh.Form{tui.TestBuildProfileStep(1, 6, r), tui.TestBuildQualityStep(3, 6, r, map[string]string{"ultra": "Ultra — u", "balanced": "Balanced — b"}), tui.TestBuildMethodologyStep(6, 6, r)} {
				var out strings.Builder
				require.NoError(t, form.WithAccessible(true).WithInput(strings.NewReader("\n")).WithOutput(&out).Run())
				line := lines[[]int{0, 2, 5}[i]]
				value := line[strings.Index(line, ": ")+2:]
				assert.Contains(t, out.String(), ". "+value+" — ", "%q among the options", value)
			}
		}
	}
}

// The step kinds the loop names are the steps buildStepList builds.
func TestWizardStepKinds_MatchTheBuiltSteps(t *testing.T) {
	no := false
	for _, opts := range []tui.InitWizardOpts{
		{}, wizardWZ6(), {ExistingProfile: "developer"}, {Quality: "ultra"}, {NoReviewGate: true},
		{Platforms: []string{"codex"}, Rea: &no}, {Platforms: []string{"omp", "opencode"}},
		{ExistingProfile: "fullstack", Quality: "balanced", NoReviewGate: true, Platforms: []string{"antigravity-cli"}},
	} {
		assert.Len(t, tui.TestWizardStepKinds(opts), len(tui.TestBuildStepList(opts)), "%+v", opts)
	}
}
