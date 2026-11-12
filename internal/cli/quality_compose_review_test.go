package cli

// SPEC-REVMODEL-001 T5 (REQ-RVM-01, 02, 04, 08, 09; acceptance S1, S2, S17):
// the review tab over a real file session with accessible forms: the last
// tool tab option, participation, models, judge, preview, live check select,
// and confirmation, in contract order.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The R3 answers of a session with the OMP tab: 구성 편집 opens the tool
// tab, whose fifth option is the review tab.
var (
	reviewOpen    = []string{"2\n", "5\n"}
	reviewTabR3   = "어느 도구를 바꿀까요? \n1. 모든 도구 같게\n2. Claude Code\n3. Codex\n4. OMP\n5. 멀티프로바이더 리뷰\n"
	reviewKeepOn  = "1. 그대로 — 켜짐\n2. 끄기\n"
	reviewKeepOff = "1. 그대로 — 꺼짐\n2. 켜기\n"
	reviewCheckR3 = "확인 호출 \n1. 확인 호출 없이 계속\n2. 확인 호출 — provider 3개에 요청 1회씩 (재시도 포함 최대 9회 model 호출)\n"
)

func reviewLines(groups ...[]string) []string {
	var lines []string
	for _, group := range groups {
		lines = append(lines, group...)
	}
	return lines
}

// reviewRun is one form run of the review tests: its output, the OMP
// catalog runner the tab read, FB, and the session's config.
type reviewRun struct {
	out    string
	runner *ompCLIFakeRunner
	smoke  *reviewSmokeBackend
	cfg    *config.HarnessConfig
	err    error
}

// runReviewForms runs a real Korean file session over dir with the OMP tab
// on runner and FB installed; runner nil answers CATG.
func runReviewForms(t *testing.T, dir string, runner *ompCLIFakeRunner, lines ...string) reviewRun {
	t.Helper()
	composeSessionCatalog(t, "CAT-A")
	if runner == nil {
		runner = &ompCLIFakeRunner{catalog: []byte(reviewCatalogCATG)}
	}
	smoke := useReviewSmokeBackend(t, "omp")
	deps := ompPlatformDependencies{newRunner: func() omp.OMPModelCatalogRunner { return runner }}
	session := newComposeFormSession(t, dir)
	_, out, err := runComposeForms(t, session, composeFormOptions{OMP: &composeOMPTab{deps: deps}}, lines...)
	return reviewRun{out: out, runner: runner, smoke: smoke, cfg: session.Config, err: err}
}

// reviewLoaderValue is the loader value: the config `auto spec review`
// loads in dir.
func reviewLoaderValue(t *testing.T, dir string) *config.HarnessConfig {
	t.Helper()
	t.Chdir(dir)
	cfg, err := loadHarnessConfigForFlags(globalFlags{})
	require.NoError(t, err)
	return cfg
}

// S1: turning gemini off changes the review list only; the commands keep
// every provider, and no live check request is made.
func TestReviewTab_S1TurnOffChangesTheReviewListOnly(t *testing.T) {
	dir := writeReviewR3(t)
	before := readReviewFile(t, dir)

	run := runReviewForms(t, dir, nil, reviewLines(reviewOpen,
		[]string{"\n", "\n", "2\n", "\n", "\n", "\n", "\n", "2\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, reviewTabR3)
	assert.Contains(t, run.out, "gemini 리뷰 참여 \n"+reviewKeepOn)
	assert.Contains(t, run.out, "바뀌는 설정:\n  spec.review_gate.providers [claude, codex, gemini] → [claude, codex]\n"+
		"꺼진 provider도 orchestra secure와 auto spec review --multi에서는 계속 돕니다\n확인 호출 \n")
	assert.NotContains(t, run.out, "gemini model")
	assert.Contains(t, run.out, "구성을 적용했습니다.\n")
	assert.Len(t, reviewFileDiff(t, before, readReviewFile(t, dir)), 1)
	orchestraSection := func(text string) string {
		return text[strings.Index(text, "\norchestra:\n"):strings.Index(text, "\nquality:\n")]
	}
	assert.Equal(t, orchestraSection(before), orchestraSection(readReviewFile(t, dir)))
	loaded := reviewLoaderValue(t, dir)
	assert.Equal(t, []string{"claude", "codex"}, loaded.Spec.ReviewGate.Providers)
	assert.Equal(t, "claude", loaded.Spec.ReviewGate.Judge)
	for _, command := range []string{"review", "secure"} {
		assert.Equal(t, []string{"claude", "codex", "gemini"}, loaded.Orchestra.Commands[command].Providers, command)
	}
	names, _ := selectSpecReviewProviderNames(loaded, nil, false)
	assert.Equal(t, []string{"claude", "codex"}, names)
	assert.Empty(t, run.smoke.recorded())
	assert.Equal(t, []string{"claude", "codex"}, run.cfg.Spec.ReviewGate.Providers, "the run's config follows the write")
}

// S2: the family filter, the thinking rule, the change lines, and the exact
// selector written and assembled.
func TestReviewTab_S2ModelAndThinkingWriteExactSelectors(t *testing.T) {
	dir := writeReviewR3(t)
	before := readReviewFile(t, dir)

	// participation Enter ×3, claude opus → Enter, codex luna → Enter,
	// gemini Enter, judge Enter, live check Enter, 적용, 유지.
	run := runReviewForms(t, dir, nil, reviewLines(reviewOpen,
		[]string{"\n", "\n", "\n", "3\n", "\n", "3\n", "\n", "\n", "\n", "\n", "2\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "codex model \n1. 그대로 — openai-codex/gpt-6-astra:max\n2. openai-codex/gpt-6-astra\n3. openai-codex/gpt-6-luna\nEnter")
	assert.NotContains(t, run.out, "example/foo-1")
	assert.Contains(t, run.out, "codex thinking (openai-codex/gpt-6-luna) \n1. high\n2. low\n3. medium\nEnter")
	assert.Contains(t, run.out, "claude thinking (anthropic/claude-opus-5-5) \n1. high\n2. max\nEnter")
	assert.Contains(t, run.out, "바뀌는 설정:\n"+
		"  orchestra.providers.claude.model anthropic/claude-fable-5-1:max → anthropic/claude-opus-5-5:max\n"+
		"  orchestra.providers.codex.model openai-codex/gpt-6-astra:max → openai-codex/gpt-6-luna:high\n"+
		"바뀐 model은 auto spec review와 orchestra 명령 모두에 쓰입니다\n")
	assert.Len(t, reviewFileDiff(t, before, readReviewFile(t, dir)), 2)
	loaded := reviewLoaderValue(t, dir)
	assert.Equal(t, "openai-codex/gpt-6-luna:high", loaded.Orchestra.Providers["codex"].Model)
	assert.Equal(t, "anthropic/claude-opus-5-5:max", loaded.Orchestra.Providers["claude"].Model)
	set, err := assembleSpecReviewProviders(t.Context(), specReviewProviderRequest{Config: loaded})
	require.NoError(t, err)
	models := map[string]string{}
	for _, provider := range set.Providers {
		models[provider.Name] = provider.Model
	}
	assert.Equal(t, "openai-codex/gpt-6-luna:high", models["codex"])
	assert.Len(t, run.runner.calls, 2, "one catalog read for the tab entry: version and models")
}

// S17: 계속 편집 asks again from the working value and still compares with
// the baseline; an edit back to the baseline is no change.
func TestReviewTab_S17EditAgainUsesTheWorkingValue(t *testing.T) {
	first := []string{"\n", "\n", "2\n", "\n", "\n", "\n", "\n", "3\n"}

	t.Run("apply the second pass", func(t *testing.T) {
		dir := writeReviewR3(t)
		run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, first,
			[]string{"\n", "\n", "\n", "\n", "\n", "\n", "\n", "2\n", "\n"})...)

		require.NoError(t, run.err, run.out)
		second := run.out[strings.Index(run.out, "3. 계속 편집\n"):]
		assert.Contains(t, second, "gemini 리뷰 참여 \n"+reviewKeepOff)
		assert.Contains(t, second, "  spec.review_gate.providers [claude, codex, gemini] → [claude, codex]\n")
		assert.Equal(t, []string{"claude", "codex"}, reviewLoaderValue(t, dir).Spec.ReviewGate.Providers)
	})
	t.Run("back to the baseline", func(t *testing.T) {
		dir := writeReviewR3(t)
		sum := reviewFileSum(t, dir)
		run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, first,
			[]string{"\n", "\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n"})...)

		require.NoError(t, run.err, run.out)
		second := run.out[strings.Index(run.out, "3. 계속 편집\n"):]
		assert.Contains(t, second, "변경 없음\n"+reviewCheckR3)
		assert.Equal(t, 1, strings.Count(run.out, "적용할까요?"), "no confirmation without a change")
		assert.Equal(t, sum, reviewFileSum(t, dir))
	})
}
