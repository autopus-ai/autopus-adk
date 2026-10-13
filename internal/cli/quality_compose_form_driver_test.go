package cli

// SPEC-MODELUX-002 T5 (REQ-MUX2-03, REQ-MUX2-04, REQ-MUX2-09, REQ-MUX2-13;
// acceptance S2, S3): the form flow over a real session: the menu after
// every table, the tool tab, the default preset select, one catalog probe
// per run, and a refused write that keeps the baseline.

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// newComposeFormSession is a real file session over dir in Korean.
func newComposeFormSession(t *testing.T, dir string) composeDriverSession {
	t.Helper()
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	return newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)
}

// runComposeForms runs session's form flow over the scripted answers with
// accessible forms.
func runComposeForms(t *testing.T, session composeDriverSession, options composeFormOptions, lines ...string) (composeDriverResult, string, error) {
	t.Helper()
	useAccessibleComposeForms(t)
	cmd, out := newComposeTestCommand()
	cmd.SetIn(formInput(lines...))
	result, err := session.runForms(cmd, options)
	return result, out.String(), err
}

const (
	composeFormMenuQuality = "구성을 바꿀까요? \n1. 유지\n2. 구성 편집\n3. 기본 프리셋 고르기\n"
	composeFormMenuR       = "구성을 바꿀까요? \n1. 유지\n2. 최신 기본값으로 갱신\n3. 구성 편집\n"
	composeFormTabD        = "어느 도구를 바꿀까요? \n1. 모든 도구 같게\n2. Claude Code\n3. Codex\n"
)

// S2: the menu after the table; Enter keeps, writes nothing, and probes the
// catalog once. Without a historical preset there is no refresh.
func TestComposeForms_EnterKeepsAndWritesNothing(t *testing.T) {
	probes := composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)

	result, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{Presets: true}, "\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.True(t, result.Snapshot.codexCatalogFixed)
	assert.Contains(t, out, composeViewStatusD+"\n"+composeSessionGolden(t, "table-D-CAT-A.golden"))
	assert.Contains(t, out, composeFormMenuQuality)
	assert.Len(t, *probes, 1)
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// S2: a historical built-in offers the refresh; `auto update` has no
// default preset choice.
func TestComposeForms_HistoricalPresetOffersTheRefresh(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir, _ := composeSessionProject(t, "R")

	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{}, "\n")

	require.NoError(t, err)
	assert.Contains(t, out, composeFormMenuR)
}

// S2: a Codex tab edit applied from the menu, the table drawn again with
// the binding just written, and the menu once more; one probe for the run.
func TestComposeForms_AppliedEditRedrawsTheTableAndAsksAgain(t *testing.T) {
	probes := composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	session := newComposeFormSession(t, dir)

	result, out, err := runComposeForms(t, session, composeFormOptions{Presets: true},
		"2\n", "3\n", "\n", "3\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "2\n", "\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverApplied, result.Outcome)
	assert.Contains(t, out, composeFormTabD)
	assert.Contains(t, out, "구성을 적용했습니다.\n")
	assert.Equal(t, 2, strings.Count(out, composeFormMenuQuality))
	redrawn := out[strings.LastIndex(out, composeViewStatusD):]
	assert.Contains(t, composeTableHeader(t, redrawn, composeViewStatusD), "Codex (개별 설정: codex-custom)")
	assert.Equal(t, "codex-custom", session.Config.Quality.Providers["codex"])
	assert.Len(t, *probes, 1)
}

// S2: the default preset select writes only the quality.default line; a
// refused write leaves one warning line and the baseline as it was.
func TestComposeForms_DefaultPresetWritesTheDefaultLine(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := readBlockFixture(t, dir)

	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{Presets: true}, "3\n", "1\n", "\n")

	require.NoError(t, err)
	assert.Contains(t, out, "1. ultra — "+config.DefaultFullConfig("").Quality.Presets["ultra"].Description+"\n")
	assert.Contains(t, out, "2. balanced — "+config.DefaultFullConfig("").Quality.Presets["balanced"].Description+" (현재)\n")
	assert.Contains(t, out, "quality.default = ultra\n")
	assert.Equal(t, strings.Replace(before, "    default: balanced\n", "    default: ultra\n", 1), readBlockFixture(t, dir))
}

func TestComposeForms_RefusedDefaultWriteKeepsTheBaseline(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	session := newComposeFormSession(t, dir)

	_, out, err := runComposeForms(t, session, composeFormOptions{Presets: true}, "3\n", "1\n", "\n")

	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(out, "  경고: "), out)
	assert.NotContains(t, out, "quality.default = ultra\n")
	assert.Equal(t, before, composeSessionHash(t, dir))
	assert.Equal(t, "balanced", session.Config.Quality.Default)
	tables := strings.Split(out, composeViewStatusD+"\n")
	require.Len(t, tables, 3)
	assert.Equal(t, tables[1][:strings.Index(tables[1], "구성을 바꿀까요?")], tables[2][:strings.Index(tables[2], "구성을 바꿀까요?")])
}

// S3: the tool tab lists the installed tools with per-agent models; a
// project with one tool lists two options and one with none skips the tab.
func TestComposeForms_ToolTabListsTheInstalledTools(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	cases := []struct {
		platforms []string
		want      string
	}{
		{[]string{"claude-code", "codex"}, composeFormTabD},
		{[]string{"claude-code"}, "어느 도구를 바꿀까요? \n1. 모든 도구 같게\n2. Claude Code\n"},
		{[]string{"opencode", "antigravity-cli"}, ""},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.platforms, ","), func(t *testing.T) {
			dir := writeComposeFixture(t, tc.platforms...)
			_, out, err := runComposeForms(t, newComposeFormSession(t, dir), composeFormOptions{}, "2\n", "\n", "\n")
			require.NoError(t, err)
			assert.NotRegexp(t, `(?m)^\d+\. (OpenCode|Antigravity)`, out, "no tab for a tool without per-agent models")
			if tc.want == "" {
				assert.NotContains(t, out, "어느 도구를 바꿀까요?")
				assert.Contains(t, out, "어떻게 고칠까요? \n1. 역할 그룹\n2. 에이전트별\n")
				return
			}
			assert.Contains(t, out, tc.want)
		})
	}
}

// S15 through the form flow: a preset name and description from
// autopus.yaml reach the status line, the menu screen, and the default
// preset select only in their terminalSafe spelling.
func TestComposeForms_PresetTextFromTheFileIsTerminalSafe(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	name := "fast\u202e\u200b"
	cfg.Quality.Presets[name] = config.QualityPreset{Description: "note \x1b]0;owned\a", Agents: map[string]string{"planner": "haiku"}}
	cfg.Quality.Default = name
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)

	_, out, err := runComposeForms(t, session, composeFormOptions{Presets: true}, "3\n", "\n", "\n")

	require.NoError(t, err)
	assert.Contains(t, out, "quality.default = "+terminalSafe(name))
	assert.Contains(t, out, terminalSafe(name)+" — note \\x1b]0;owned\\a (현재)\n")
	assertNoTerminalControls(t, out)
}
