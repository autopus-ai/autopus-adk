package cli

// SPEC-REVMODEL-001 T7 (REQ-RVM-13; acceptance S10): init applies the
// wizard's Review Gate answer and the detected providers before its compose
// step, so a review tab edit goes into the config init saves and no later
// init step overwrites it.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// stubReviewInitWizard answers the wizard with the role-groups choice, so
// the compose forms open, and reviewGate as its Review Gate answer.
func stubReviewInitWizard(t *testing.T, reviewGate bool) {
	t.Helper()
	original := runInitWizard
	t.Cleanup(func() { runInitWizard = original })
	runInitWizard = func(tui.InitWizardOpts) (*tui.InitWizardResult, error) {
		return &tui.InitWizardResult{
			CommentsLang: "en", CommitsLang: "ko", AILang: "ko", Quality: tui.QualityRoleGroups,
			Methodology: "tdd", UsageProfile: "developer", ReviewGate: reviewGate,
		}, nil
	}
}

// newReviewInitRun is init's compose environment with claude, codex, and
// agy on PATH, so init detects the providers claude, codex, and gemini.
func newReviewInitRun(t *testing.T, reviewGate bool) {
	t.Helper()
	newComposeUpdateRun(t, true, "")
	stubReviewInitWizard(t, reviewGate)
	useAccessibleComposeForms(t)
	bin := t.TempDir()
	for _, binary := range []string{"claude", "agy"} {
		setFakeProviderOnPath(t, bin, binary)
	}
}

func reviewInitSaved(t *testing.T, dir string) *config.HarnessConfig {
	t.Helper()
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	return cfg
}

const reviewInitTab = "어느 도구를 바꿀까요? \n1. 모든 도구 같게\n2. Claude Code\n3. Codex\n"

func TestInitComposeReview_S10TabEditSurvivesInit(t *testing.T) {
	t.Run("(a) gemini off", func(t *testing.T) {
		newReviewInitRun(t, true)
		// 구성 편집, 멀티프로바이더 리뷰, claude·codex Enter, gemini 끄기, claude·codex
		// model Enter, judge Enter, 확인 호출 Enter, 적용, 유지.
		dir, out, err := runComposeInit(t, formInput("2\n", "4\n", "\n", "\n", "2\n", "\n", "\n", "\n", "\n", "2\n", "\n"))

		require.NoError(t, err, out)
		assert.Contains(t, out, reviewInitTab+"4. 멀티프로바이더 리뷰\n")
		assert.Contains(t, out, "claude model \n1. 그대로 — claude-opus-5-5 · max\n")
		assert.Contains(t, out, "구성을 적용했습니다.\n")
		saved := reviewInitSaved(t, dir)
		assert.True(t, saved.Spec.ReviewGate.Enabled)
		assert.Equal(t, []string{"claude", "codex"}, saved.Spec.ReviewGate.Providers)
		for name, command := range saved.Orchestra.Commands {
			assert.Contains(t, command.Providers, "gemini", name)
		}
	})
	t.Run("(b) tab not opened", func(t *testing.T) {
		newReviewInitRun(t, true)
		dir, out, err := runComposeInit(t, formInput("\n"))

		require.NoError(t, err, out)
		assert.Equal(t, []string{"claude", "codex", "gemini"}, reviewInitSaved(t, dir).Spec.ReviewGate.Providers)
	})
	t.Run("(c) review gate off", func(t *testing.T) {
		newReviewInitRun(t, false)
		_, out, err := runComposeInit(t, formInput("2\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n"))

		require.NoError(t, err, out)
		assert.Contains(t, out, reviewInitTab+"Enter a number between 1 and 3: ")
		assert.NotContains(t, out, "멀티프로바이더 리뷰")
		assert.False(t, strings.Contains(out, "리뷰 참여"))
	})
}
