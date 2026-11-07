package cli

// SPEC-TUIUX-001 T9 (REQ-TUX-05, 11, 12, 17; acceptance S6, S8, S9, S15 (h)):
// before any file is written, interactive init on the form path shows the
// answers, every setting key they change, and the REA decision, and asks
// 적용, 뒤로, or 취소. 취소 and Ctrl+C leave the project as it was.

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// S6: the review block, its frame and select, nothing written while it
// runs, and the files of the base commit's IN run once it applies.
func TestInitReview_S6ShowsTheSettingsBeforeAnyWrite(t *testing.T) {
	newTUIUXEnv(t, true, "")
	stubTUIUXWizard(t, tuiuxW())
	dir := tuiuxProject(t)
	var existed []bool
	useNavComposeFormsWith(t, func(_ int, frame tui.NavFrame) error {
		if frame.Header == tuiuxReview {
			for _, name := range []string{"DESIGN.md", "autopus.yaml"} {
				_, err := os.Stat(filepath.Join(dir, name))
				existed = append(existed, err == nil)
			}
		}
		return nil
	})

	out, err := runTUIUXInit(t, dir, formInput("\n"))

	require.NoError(t, err, out)
	assert.Contains(t, out, strings.Join([]string{
		"적용할 설정", " 답한 값", "  사용 프로필: Developer", "  언어 설정: 주석 en · 커밋 ko · AI 응답 ko",
		"  품질 모드: Balanced", "  리뷰 게이트: 끔", "  REA 역공학 MCP: 켬", "  방법론: TDD",
		" 바뀌는 설정", "  language: ai_responses=ko, comments=en, commits=ko",
		// New configs default to methodology none, so a TDD answer is a change.
		"  methodology: enforce=true, mode=tdd", "  spec.review_gate: enabled=false",
		"  usage_profile: developer", " REA", "  REA MCP: 켬 · 등록", tuiuxReview, tuiuxFooter,
		"이대로 진행할까요? ", "1. 적용", "2. 뒤로", "3. 취소",
	}, "\n")+"\n")
	assert.Equal(t, []bool{false, false}, existed, "DESIGN.md and autopus.yaml do not exist while the review runs")
	assert.Less(t, strings.Index(out, "적용할 설정\n"), strings.Index(out, tuiuxDesignStep))
	assert.Equal(t, tuiuxGolden(t, "init-in.npd"), tuiuxNPD(t, dir))
}

// tuiuxPathState is every path under dir with its bytes' SHA-256 and mtime.
func tuiuxPathState(t *testing.T, dir string) map[string]string {
	t.Helper()
	state := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		sum := ""
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		state[path] = info.ModTime().Format(time.RFC3339Nano) + " " + sum
		return nil
	}))
	return state
}

// S8: 취소 on the review and Ctrl+C there print the cancel line, end init
// without an error, and leave every path as it was, also when autopus.yaml
// holds a platform name config.Load would rewrite.
func TestInitReview_S8CancelWritesNothing(t *testing.T) {
	fixtures := map[string]func(t *testing.T) string{
		"(a) empty": tuiuxProject,
		"(b) old platform name": func(t *testing.T) string {
			dir := tuiuxInitYes(t)
			path := filepath.Join(dir, "autopus.yaml")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(data), "    - codex\n")
			require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), "    - codex\n", "    - gemini\n", 1)), 0o644))
			past := time.Now().Add(-time.Hour)
			require.NoError(t, filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				return os.Chtimes(path, past, past)
			}))
			return dir
		},
	}
	for name, fixture := range fixtures {
		for _, how := range []string{"취소", "Ctrl+C"} {
			t.Run(name+" "+how, func(t *testing.T) {
				newTUIUXEnv(t, true, "")
				dir := fixture(t)
				before := tuiuxPathState(t, dir)
				stubTUIUXWizard(t, tuiuxW())
				useNavComposeFormsWith(t, func(_ int, frame tui.NavFrame) error {
					if how == "Ctrl+C" && frame.Header == tuiuxReview {
						return huh.ErrUserAborted
					}
					return nil
				})

				out, err := runTUIUXInit(t, dir, formInput("3\n"))

				require.NoError(t, err, out)
				assert.True(t, strings.HasSuffix(out, "\n  init cancelled.\n"), out)
				assert.Equal(t, before, tuiuxPathState(t, dir))
				if name == "(a) empty" {
					assert.Empty(t, before)
				} else {
					data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
					require.NoError(t, err)
					assert.Contains(t, string(data), "    - gemini\n")
				}
			})
		}
	}
}

// S9, review part, and REQ-TUX-17: the review follows the session language.
func TestInitReview_S9EnglishReview(t *testing.T) {
	newTUIUXEnv(t, true, "")
	composeLanguage = func() string { return "en" }
	run := runTUIUXIN(t, []tui.InitWizardResult{tuiuxW()}, "\n")

	assert.Contains(t, run.out, "Settings to apply\n Answers\n  Usage Profile: Developer\n")
	assert.Contains(t, run.out, "\n Changed settings\n  language: ai_responses=ko, comments=en, commits=ko\n")
	assert.Contains(t, run.out, "\n REA\n  REA MCP: on · add\ninit · Review\nType an answer and press Enter · back: b\nProceed with these settings? \n1. apply\n2. back\n3. cancel\n")
}

// S15 (h): init's form gate does not read CI, so a truthy CI still asks
// the wizard and the review.
func TestInitReview_S15hCIStillAsks(t *testing.T) {
	newTUIUXEnv(t, true, "true")
	run := runTUIUXIN(t, []tui.InitWizardResult{tuiuxW()}, "\n")

	assert.Len(t, *run.calls, 1)
	assert.Contains(t, run.out, tuiuxReview+"\n")
}

// The review's back and Esc on a stage that asked no form go to the
// wizard; a form error other than Ctrl+C ends init with nothing written.
func TestInitReview_FormErrorEndsInitWithoutWrites(t *testing.T) {
	newTUIUXEnv(t, true, "")
	stubTUIUXWizard(t, tuiuxW())
	dir := tuiuxProject(t)
	useNavComposeFormsWith(t, func(_ int, frame tui.NavFrame) error {
		if frame.Header == tuiuxReview {
			return os.ErrClosed
		}
		return nil
	})

	out, err := runTUIUXInit(t, dir, formInput())

	require.ErrorIs(t, err, os.ErrClosed)
	assert.Contains(t, err.Error(), "init review")
	assert.Empty(t, tuiuxPathState(t, dir), out)
}

// The REA line reads the run's decision and the action decide set.
func TestInitReview_ReaDecisionAndAction(t *testing.T) {
	text := initReviewTextFor("ko")
	cfg := tuiuxConfigWith(config.ReaMCPRemove)
	run := &initInteractive{cfg: cfg, rea: &initRea{reaRun: &reaRun{state: reaOff}}}
	assert.Equal(t, "끔", run.reaDecision(text))
	assert.Equal(t, "제거", run.reaAction(text))
	run.cfg.Runtime.ReaMCP = config.ReaMCPKeep
	run.rea.state = reaOn
	assert.Equal(t, "켬", run.reaDecision(text))
	assert.Equal(t, "그대로", run.reaAction(text))
	assert.Equal(t, "keep", run.reaAction(initReviewTextFor("en")))
}

func tuiuxConfigWith(action config.ReaMCPAction) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("p")
	cfg.Runtime.ReaMCP = action
	return cfg
}

// A piped or TERM=dumb review whose input ends after an invalid answer is a
// cancel: the existing cancel line, no error, and nothing written.
func TestInitReview_InvalidAnswerThenEndCancels(t *testing.T) {
	newTUIUXEnv(t, true, "")
	run := runTUIUXIN(t, []tui.InitWizardResult{tuiuxW()}, "9\n")

	assert.True(t, strings.HasSuffix(run.out, "\n  init cancelled.\n"), run.out)
	assert.Empty(t, tuiuxPathState(t, run.dir))
}
