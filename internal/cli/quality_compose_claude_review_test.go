package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The Claude review provider model and the Claude Code preset are separate
// settings; these are the lines that say so, as the operator reads them.
const (
	claudeReviewLineFable = "리뷰 provider claude 모델 = claude-fable-5-1 " +
		"(Claude Code 프리셋과 별개 설정 · 변경: auto quality review model claude <model>)"
	claudeReviewWarnCustom = "주의: Claude Code 프리셋 claude-custom의 최상위 등급은 opus인데 " +
		"리뷰 provider claude 모델은 claude-fable-5-1입니다. 별개 설정이며 auto quality review model claude <model>로 바꿉니다."
	claudeReviewWarnFormer = "주의: Claude Code 프리셋 balanced의 최상위 등급은 fable인데 " +
		"리뷰 provider claude 모델은 opus입니다. 별개 설정이며 auto quality review model claude <model>로 바꿉니다."
	claudeFormerDefaultNotice = "참고: claude 리뷰 provider 모델이 이전 버전 기본값 opus입니다. " +
		"현재 기본값은 claude-fable-5-1이며 auto quality review model claude <model>로 바꿀 수 있습니다."
)

var claudeFormerDefaultArgs = []string{"--print", "--model", "opus", "--effort", "max"}

// claudeAllOpusConfig is the reported setup: quality.providers.claude names
// a custom preset with every agent on opus, while the review provider keeps
// the shipped claude-fable-5-1.
func claudeAllOpusConfig() *config.HarnessConfig {
	cfg := config.DefaultFullConfig("p")
	agents := map[string]string{}
	for _, name := range config.CanonicalAgentNames() {
		agents[name] = "opus"
	}
	cfg.Quality.Presets["claude-custom"] = config.QualityPreset{Agents: agents}
	cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: "claude-custom"}
	return cfg
}

func withClaudeArgs(cfg *config.HarnessConfig, args []string) *config.HarnessConfig {
	entry := cfg.Orchestra.Providers["claude"]
	entry.Args = append([]string(nil), args...)
	cfg.Orchestra.Providers["claude"] = entry
	return cfg
}

func TestClaudeReviewModelLine_NamesTheSeparateReviewModel(t *testing.T) {
	t.Parallel()
	text := composeTextFor("ko")
	disabled := config.DefaultFullConfig("p")
	disabled.Orchestra.Enabled = false
	routed := config.DefaultFullConfig("p")
	routed.Orchestra.Providers["claude"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: "anthropic/opus"}

	tests := []struct {
		name string
		cfg  *config.HarnessConfig
		want string
	}{
		{"shipped default agrees with the preset top tier", config.DefaultFullConfig("p"), claudeReviewLineFable},
		{"all-opus custom preset with the fable review model warns", claudeAllOpusConfig(), claudeReviewWarnCustom},
		{"former opus review model under balanced warns", withClaudeArgs(config.DefaultFullConfig("p"), claudeFormerDefaultArgs), claudeReviewWarnFormer},
		{"model-less print names no model", withClaudeArgs(config.DefaultFullConfig("p"), []string{"--print"}), ""},
		{"orchestra off has no review provider", disabled, ""},
		{"backend-routed provider has no CLI model", routed, ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, claudeReviewModelLine("", tt.cfg, text))
		})
	}
}

func TestClaudeReviewFormerDefaultNotice_NamesModelDefaultAndCommand(t *testing.T) {
	t.Parallel()
	text := composeTextFor("ko")

	var out bytes.Buffer
	writeClaudeReviewFormerDefaultNotice(&out, "", withClaudeArgs(config.DefaultFullConfig("p"), claudeFormerDefaultArgs), text)
	assert.Equal(t, claudeFormerDefaultNotice+"\n", out.String())

	for _, args := range [][]string{config.DefaultClaudeProviderEntry().Args, {"--print"}, {"--print", "--model", "opus"}} {
		out.Reset()
		writeClaudeReviewFormerDefaultNotice(&out, "", withClaudeArgs(config.DefaultFullConfig("p"), args), text)
		assert.Empty(t, out.String(), "%v", args)
	}
}

// A non-preview update over a former default keeps the stored opus argv and
// prints the notice once; a preview prints nothing of it.
func TestUpdate_FormerDefaultClaudeReviewModel_NoticesAndKeepsOpus(t *testing.T) {
	newComposeUpdateRun(t, false, "")
	dir, cfg := composeSessionProject(t, "D")
	require.NoError(t, config.Save(dir, withClaudeArgs(cfg, claudeFormerDefaultArgs)))

	preview, err := runComposeUpdate(t, driverFailingReader{t}, "--dir", dir, "--plan")
	require.NoError(t, err, preview)
	assert.NotContains(t, preview, claudeFormerDefaultNotice)

	out, err := runComposeUpdate(t, driverFailingReader{t}, "--dir", dir, "--yes")
	require.NoError(t, err, out)
	assert.Equal(t, 1, strings.Count(out, claudeFormerDefaultNotice), out)

	updated, err := config.Load(dir)
	require.NoError(t, err)
	// The model stays; only the #267 output-format piece is added.
	assert.Equal(t, append(append([]string(nil), claudeFormerDefaultArgs...), "--output-format", "json"), updated.Orchestra.Providers["claude"].Args)

	again, err := runComposeUpdate(t, driverFailingReader{t}, "--dir", dir, "--yes")
	require.NoError(t, err, again)
	assert.Equal(t, 1, strings.Count(again, claudeFormerDefaultNotice), "the notice repeats while the model stays")
}

// An interactive update draws the review model line under the table.
func TestUpdateCompose_InteractiveTableNamesTheClaudeReviewModel(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	dir, cfg := composeSessionProject(t, "D")
	require.NoError(t, config.Save(dir, cfg))

	out, err := runComposeUpdate(t, reaEnter(strings.NewReader("\n")), "--dir", dir)

	require.NoError(t, err, out)
	assert.Contains(t, out, "에이전트 모델 구성")
	assert.Equal(t, 1, strings.Count(out, claudeReviewLineFable+"\n"), out)
}

func TestQualityShow_NamesTheClaudeReviewModelAndWarnsOnMismatch(t *testing.T) {
	originalLanguage := composeLanguage
	t.Cleanup(func() { composeLanguage = originalLanguage })
	composeLanguage = func() string { return "ko" }

	for name, tc := range map[string]struct {
		cfg  *config.HarnessConfig
		want string
	}{
		"default":  {config.DefaultFullConfig("p"), claudeReviewLineFable},
		"mismatch": {claudeAllOpusConfig(), claudeReviewWarnCustom},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, config.Save(dir, tc.cfg))
			root := NewRootCmd()
			buf := &bytes.Buffer{}
			root.SetOut(buf)
			root.SetErr(buf)
			root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "show"})

			require.NoError(t, root.Execute())
			assert.Contains(t, buf.String(), "quality.effective.claude = ")
			assert.Equal(t, 1, strings.Count(buf.String(), tc.want+"\n"), buf.String())
		})
	}
}

// A model the file takes from the environment is never shown, as in the
// review tab (REQ-RVM-26): the line names nothing rather than the value.
func TestQualityShow_ClaudeReviewLineNeverShowsAnEnvironmentValue(t *testing.T) {
	t.Setenv("REVIEW_MODEL", "rvm-sentinel-7f3a9c")
	dir := writeReviewRL(t)
	replaceReviewFile(t, dir, "--model, claude-fable-5-1,", `--model, "${REVIEW_MODEL}",`)

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "show"})

	require.NoError(t, root.Execute())
	assert.NotContains(t, buf.String(), "rvm-sentinel-7f3a9c")
	assert.NotContains(t, buf.String(), "claude review provider model")
	assert.NotContains(t, buf.String(), "리뷰 provider claude 모델")
}
