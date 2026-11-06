package cli

// SPEC-REVMODEL-001 T14 (REQ-RVM-08, 23, 24; acceptance S8, S21, S23, S26):
// an agy provider offers the listed ids of its family, every CLI provider
// can switch to the OMP backend while omp attests its family, and without
// either a provider gets the reason line.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	reviewGeminiTail = "                output_format: text\n                timeout: 480\n"
	reviewAgyReason  = "gemini(agy)는 Autopus가 증명할 model 목록이 없어 model을 바꾸지 않습니다. OMP를 설치하면 OMP backend로 전환할 수 있습니다\n"
)

// S8: without omp, an agy provider whose list Autopus cannot read gets the
// reason line once per pass and no select; claude and codex keep theirs.
func TestReviewTab_S8AgyWithoutOMPGetsTheReasonLine(t *testing.T) {
	dir := writeReviewRL(t)
	before, err := config.Load(dir)
	require.NoError(t, err)
	// (1) participation ×3, claude Enter, codex Enter, judge Enter, check Enter.
	// (2) codex 끄기, the rest Enter, check Enter, 적용, 유지.
	first := []string{"\n", "\n", "\n", "\n", "\n", "\n", "\n"}
	second := []string{"\n", "2\n", "\n", "\n", "\n", "\n", "2\n", "\n"}

	run := runReviewCLIForms(t, dir, nil, reviewLines(reviewOpen, first, reviewOpen, second)...)

	require.NoError(t, run.err, run.out)
	passes := strings.Split(run.out, "어느 도구를 바꿀까요?")
	require.Len(t, passes, 3)
	assert.Equal(t, 1, strings.Count(passes[1], reviewAgyReason))
	assert.Contains(t, passes[1], "claude model \n")
	assert.Contains(t, passes[1], "codex model \n")
	assert.Contains(t, passes[1], "변경 없음\n")
	assert.NotContains(t, run.out, "gemini model \n")
	assert.NotContains(t, run.out, "OMP backend로 전환\n")
	assert.Regexp(t, `(?m)^claude\s+켜짐\s+claude claude-opus-5-5 · max\s+judge$`, passes[2])
	assert.Regexp(t, `(?m)^codex\s+꺼짐 \*\s+codex gpt-6-astra · max$`, passes[2])
	assert.Regexp(t, `(?m)^gemini\s+켜짐\s+agy$`, passes[2])
	after, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "gemini"}, after.Spec.ReviewGate.Providers)
	assert.True(t, reflect.DeepEqual(before.Orchestra.Providers, after.Orchestra.Providers))
}

// S21: with omp, agy switches to the OMP backend; only backend and model are
// added, and the assembled provider runs on OMP within the read-only policy.
func TestReviewTab_S21AgySwitchesToTheOMPBackend(t *testing.T) {
	dir := writeReviewRL(t)
	before := readReviewFile(t, dir)

	// participation ×3, claude Enter, codex Enter, gemini 전환 → flash → thinking Enter,
	// judge Enter, check Enter, 적용, 유지.
	run := runReviewCLIForms(t, dir, []string{"omp"}, reviewLines(reviewOpen,
		[]string{"\n", "\n", "\n", "\n", "\n", "2\n", "2\n", "\n", "\n", "\n", "2\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "gemini model \n1. 그대로 — CLI backend\n2. OMP backend로 전환\nEnter")
	assert.Contains(t, run.out, "gemini model \n1. google-antigravity/gemini-3.1-pro\n2. google-antigravity/gemini-3.8-flash\nEnter")
	assert.Contains(t, run.out, "gemini thinking (google-antigravity/gemini-3.8-flash) \n1. high\n2. low\n3. medium\nEnter")
	assert.Contains(t, run.out, "  orchestra.providers.gemini.backend 없음 → omp\n  orchestra.providers.gemini.model 없음 → google-antigravity/gemini-3.8-flash:high\n")
	assert.Equal(t, strings.Replace(before, reviewGeminiTail,
		reviewGeminiTail+"            backend: omp\n            model: google-antigravity/gemini-3.8-flash:high\n", 1), readReviewFile(t, dir))
	loaded := reviewLoaderValueKeepCwd(t, dir)
	set, err := assembleSpecReviewProviders(t.Context(), specReviewProviderRequest{Config: loaded, FlagProviders: []string{"gemini"}})
	require.NoError(t, err)
	require.Len(t, set.Providers, 1)
	assert.Equal(t, config.ProviderBackendOMP, set.Providers[0].Backend)
	assert.Equal(t, "google-antigravity/gemini-3.8-flash:high", set.Providers[0].Model)
}

// S23: the listed ids of the provider's family, grouped by effort suffix;
// an id is written only as listed, and an unreadable list leaves the keep
// option and the switch.
func TestReviewTab_S23AgyModelList(t *testing.T) {
	agyPath := []string{"agy", "omp"}
	t.Run("(1) one id per group", func(t *testing.T) {
		useAgyModels(t, reviewAgySample, nil)
		dir := writeReviewRL(t)
		run := runReviewCLIForms(t, dir, agyPath, reviewLines(reviewOpen,
			[]string{"\n", "\n", "\n", "\n", "\n", "3\n", "\n", "\n", "2\n", "\n"})...)
		require.NoError(t, run.err, run.out)
		assert.Contains(t, run.out, "gemini model \n1. 그대로 — agy\n2. gemini-3.8-flash\n3. gemini-4-argon\n4. OMP backend로 전환\nEnter")
		assert.NotContains(t, run.out, "gemini effort")
		assert.Equal(t, `args: [--print, "", --output-format, stream-json, --model, gemini-4-argon-medium]`, reviewArgsLine(t, readReviewFile(t, dir), "gemini"))
	})
	t.Run("(2) an effort per listed suffix", func(t *testing.T) {
		useAgyModels(t, reviewAgySample+"gemini-3.8-flash-low\tGemini 3.8 Flash (Low)\n", nil)
		dir := writeReviewRL(t)
		run := runReviewCLIForms(t, dir, agyPath, reviewLines(reviewOpen,
			[]string{"\n", "\n", "\n", "\n", "\n", "2\n", "2\n", "\n", "\n", "2\n", "\n"})...)
		require.NoError(t, run.err, run.out)
		assert.Contains(t, run.out, "gemini effort \n1. high — Gemini 3.8 Flash (High)\n2. low — Gemini 3.8 Flash (Low)\nEnter")
		assert.Equal(t, `args: [--print, "", --output-format, stream-json, --model, gemini-3.8-flash-low]`, reviewArgsLine(t, readReviewFile(t, dir), "gemini"))
	})
	for name, variant := range reviewAgyUnreadable {
		t.Run("(3) "+name, func(t *testing.T) {
			useAgyModels(t, variant.out, variant.err)
			dir := writeReviewRL(t)
			run := runReviewCLIForms(t, dir, agyPath, reviewLines(reviewOpen,
				[]string{"\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n"})...)
			require.NoError(t, run.err, run.out)
			assert.Contains(t, run.out, "gemini(agy) model 목록을 읽지 못해 agy model은 그대로입니다\n"+
				"gemini model \n1. 그대로 — agy\n2. OMP backend로 전환\nEnter")
			assert.NotContains(t, run.out, "\x1b")
		})
	}
}

// S26, agy half and the combined preview: an id off the list is an error,
// and the tab names exactly the changed flag values, provider by provider.
func TestReviewTab_S26CLIChangeLines(t *testing.T) {
	useAgyModels(t, reviewAgySample, nil)
	dir := writeReviewRL(t)
	sum := reviewFileSum(t, dir)

	// participation ×3, claude fable → xhigh, codex Enter, gemini gemini-4-argon, judge, check, 취소, 유지.
	run := runReviewCLIForms(t, dir, []string{"agy"}, reviewLines(reviewOpen,
		[]string{"\n", "\n", "\n", "2\n", "4\n", "\n", "3\n", "\n", "\n", "\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "바뀌는 설정:\n"+
		"  orchestra.providers.claude.args --model claude-opus-5-5 → claude-fable-5-1\n"+
		"  orchestra.providers.claude.args --effort max → xhigh\n"+
		"  orchestra.providers.gemini.args --model 없음 → gemini-4-argon-medium\n바뀐 model은")
	assert.Equal(t, sum, reviewFileSum(t, dir))

	cfg := reviewR3Config()
	reviewRLConfig(cfg)
	baseline := newReviewBaseline(cfg, nil)
	listed, _ := parseAgyModels([]byte(reviewAgySample))
	attested := newReviewAttestation()
	attested.CLI["gemini"] = agyCLIModels("gemini", listed)
	working := baseline.reviewComposition.clone()
	working.CLI["gemini"] = reviewCLIValue{Model: "gemini-9-foo-high"}
	_, err := planReviewComposition(baseline, working, attested, composeReviewTextKO)
	require.Error(t, err)
	assert.Equal(t, "gemini model gemini-9-foo-high는 이 provider의 선택지에 없습니다", err.Error())
}

// L2: an agy binary given as a path is never run on tab entry, whatever it
// is; the provider gets the reason line, and the OMP switch stays.
func TestReviewTab_AgyPathIsNeverRun(t *testing.T) {
	runs := useAgyModels(t, reviewAgySample, nil)
	tools := t.TempDir()
	agy := filepath.Join(tools, "agy")
	require.NoError(t, os.WriteFile(agy, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	dir := writeReviewRL(t)
	replaceReviewFile(t, dir, "            binary: agy\n", "            binary: "+agy+"\n")

	// participation ×3, claude Enter, codex Enter, gemini Enter, judge Enter, check Enter, the menu's default.
	run := runReviewCLIForms(t, dir, []string{"omp"}, reviewLines(reviewOpen,
		[]string{"\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Zero(t, *runs)
	assert.Contains(t, run.out, "gemini의 binary "+agy+"는 PATH에서 찾는 이름이 아니라 model 목록을 읽으려고 실행하지 않습니다\n"+
		"gemini model \n1. 그대로 — CLI backend\n2. OMP backend로 전환\nEnter")
}
