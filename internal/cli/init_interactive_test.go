package cli

// SPEC-TUIUX-001 T9, probe A3 (REQ-TUX-13, REQ-TUX-15; acceptance S7, S14
// (c)): init's loop keeps the composition when the review's 뒤로 resumes the
// stage, drops it when the stage's menu goes back to the wizard, and
// generates from the snapshot of the last stage entry. Each run compares
// with a control run of the same final answers, and a negative control with
// one catalog shows the snapshot oracle tells the two catalogs apart.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

// tuiuxCatalogs stubs the run's Codex catalog probe with acceptance
// catalogs in call order, the last one again once they run out.
func tuiuxCatalogs(t *testing.T, catalogs ...string) *int {
	t.Helper()
	data := make([][]byte, len(catalogs))
	for i, catalog := range catalogs {
		raw, err := os.ReadFile(filepath.Join(composeTableTestdata, strings.ToLower(catalog)+".json"))
		require.NoError(t, err)
		data[i] = raw
	}
	original := composeCodexCatalogProbe
	t.Cleanup(func() { composeCodexCatalogProbe = original })
	calls := new(int)
	composeCodexCatalogProbe = func(context.Context, string, time.Duration) ([]byte, error) {
		*calls++
		return data[min(*calls, len(data))-1], nil
	}
	return calls
}

// tuiuxRun is one IN run: its output, NPD, project, and wizard calls.
type tuiuxRun struct {
	out, npd, dir string
	calls         *[]tui.InitWizardOpts
}

// runTUIUXIN runs IN with the wizard seam answering results in turn and
// every form through the accessible helper.
func runTUIUXIN(t *testing.T, results []tui.InitWizardResult, lines ...string) tuiuxRun {
	t.Helper()
	calls := stubTUIUXWizard(t, results...)
	useNavComposeForms(t)
	dir := tuiuxProject(t)
	out, err := runTUIUXInit(t, dir, formInput(lines...))
	require.NoError(t, err, out)
	return tuiuxRun{out: out, npd: tuiuxNPD(t, dir), dir: dir, calls: calls}
}

// tuiuxWith returns W with its quality and methodology answers replaced.
func tuiuxWith(quality, methodology string) tui.InitWizardResult {
	w := tuiuxW()
	w.Quality, w.Methodology = quality, methodology
	return w
}

const (
	tuiuxReview     = "init · 확인"
	tuiuxDesignStep = "[2/5] Design Context"
)

// S7 (a): the review's 뒤로 after a stage that asked no form resumes the
// wizard at its last step; the run writes what W_none alone writes.
func TestInitInteractive_S7aReviewBackResumesTheWizard(t *testing.T) {
	newTUIUXEnv(t, true, "")
	run := runTUIUXIN(t, []tui.InitWizardResult{tuiuxW(), tuiuxWith("balanced", "none")}, "2\n", "\n")
	control := runTUIUXIN(t, []tui.InitWizardResult{tuiuxWith("balanced", "none")}, "\n")

	require.Len(t, *run.calls, 2)
	assert.Nil(t, (*run.calls)[0].Resume)
	assert.Equal(t, tuiuxW(), *(*run.calls)[1].Resume)
	second := run.out[strings.LastIndex(run.out, "적용할 설정\n"):]
	assert.Contains(t, second, "\n  방법론: None\n")
	assert.Equal(t, 2, strings.Count(run.out, tuiuxReview+"\n"))
	assert.Equal(t, 1, strings.Count(run.out, tuiuxDesignStep))
	assert.Equal(t, control.npd, run.npd)
}

// tuiuxIR is IR's composition answers on the Codex tab: 설계·판단 on opus
// (option 3 under both catalogs) with effort low, every later form kept,
// then 적용.
var tuiuxIRCodexOpusLow = tuiuxLines([]string{"2\n", "3\n", "1\n", "3\n", "2\n"}, tuiuxEnters(6), []string{"2\n"})

// S7 (b), (b′): the review's 뒤로 resumes the stage with the composition
// kept; the second entry's snapshot is the one the run generates with.
func TestInitInteractive_S7bReviewBackKeepsTheComposition(t *testing.T) {
	wrg := tuiuxWith(tui.QualityRoleGroups, "tdd")
	lines := tuiuxLines(tuiuxIRCodexOpusLow, []string{"\n", "2\n", "\n", "\n"}) // 유지, 뒤로, 유지, 적용
	run := func(t *testing.T, catalogs ...string) tuiuxRun {
		newTUIUXEnv(t, true, "")
		tuiuxCatalogs(t, catalogs...)
		return runTUIUXIN(t, []tui.InitWizardResult{wrg}, lines...)
	}
	t.Run("(b) CAT-A then CAT-B", func(t *testing.T) {
		got := run(t, "CAT-A", "CAT-B")
		newTUIUXEnv(t, true, "")
		tuiuxCatalogs(t, "CAT-B")
		control := runTUIUXIN(t, []tui.InitWizardResult{wrg}, tuiuxLines(tuiuxIRCodexOpusLow, []string{"\n", "\n"})...)

		require.Len(t, *got.calls, 1)
		assert.Equal(t, 3, strings.Count(got.out, "구성을 바꿀까요? \n"))
		assert.Equal(t, 2, strings.Count(got.out, tuiuxReview+"\n"))
		for _, block := range strings.Split(got.out, "적용할 설정\n")[1:] {
			assert.Contains(t, block, "\n  quality.providers: codex=codex-custom\n")
			assert.Contains(t, block, "\n  quality.presets.codex-custom.efforts: architect=low, planner=low, security-auditor=low, spec-writer=low\n")
		}
		preset := composeBindLoad(t, got.dir).Quality.Presets["codex-custom"]
		for _, agent := range composeDesignAgents {
			assert.Equal(t, "opus", preset.Agents[agent], agent)
			assert.Equal(t, "low", preset.Efforts[agent], agent)
		}
		assert.Equal(t, control.npd, got.npd)
		planner := readComposeCodexProfile(t, got.dir, "planner")
		assert.Equal(t, "low", planner.Effort)
		assert.Equal(t, readComposeCodexProfile(t, control.dir, "planner"), planner)
		// reverse-engineer runs the Sol rung, which CAT-B substitutes.
		analysis := readComposeCodexProfile(t, got.dir, "reverse-engineer")
		assert.NotEqual(t, "gpt-6.1-sol", analysis.Model)
		assert.Equal(t, readComposeCodexProfile(t, control.dir, "reverse-engineer"), analysis)
		assert.Equal(t, 1, strings.Count(got.out, tuiuxDesignStep))
	})
	t.Run("(b′) CAT-A twice", func(t *testing.T) {
		got := run(t, "CAT-A")
		assert.Equal(t, "gpt-6.1-sol", readComposeCodexProfile(t, got.dir, "reverse-engineer").Model)
	})
}

// S14 (c), (c′): the stage's menu going back drops the composition; the
// resumed wizard and a fresh stage write only the second entry's answers,
// from the second entry's snapshot.
func TestInitInteractive_S14cMenuBackDropsTheComposition(t *testing.T) {
	wrg := tuiuxWith(tui.QualityRoleGroups, "tdd")
	// First entry: Claude Code, 설계·판단 kept, its effort low, the rest
	// kept, 적용, then the menu's 뒤로.
	first := tuiuxLines([]string{"2\n", "2\n", "1\n", "\n", "2\n"}, tuiuxEnters(6), []string{"2\n", "b\n"})
	// Second entry: Codex, 설계·판단 on fable, the rest kept, 적용; codex-custom
	// keeps reverse-engineer on the Sol ladder.
	second := tuiuxLines([]string{"2\n", "3\n", "1\n", "2\n"}, tuiuxEnters(7), []string{"2\n", "\n", "\n"})
	run := func(t *testing.T, catalogs ...string) tuiuxRun {
		newTUIUXEnv(t, true, "")
		tuiuxCatalogs(t, catalogs...)
		return runTUIUXIN(t, []tui.InitWizardResult{wrg, wrg}, tuiuxLines(first, second)...)
	}
	t.Run("(c) CAT-A then CAT-B", func(t *testing.T) {
		got := run(t, "CAT-A", "CAT-B")
		newTUIUXEnv(t, true, "")
		tuiuxCatalogs(t, "CAT-B")
		control := runTUIUXIN(t, []tui.InitWizardResult{wrg}, second...)

		require.Len(t, *got.calls, 2)
		assert.Equal(t, wrg, *(*got.calls)[1].Resume)
		// The table drawn after the first entry's 적용 binds the Claude Code
		// column to claude-custom with low cells; the second entry's first
		// table is back on balanced. (acceptance.md expects a `*` cell here,
		// which no redrawn table prints: the code fact is the binding header.)
		tables := strings.Split(got.out, "에이전트 모델 구성 ─")
		require.Len(t, tables, 5, "two tables per entry: at the start and after its 적용")
		assert.Contains(t, tables[2], "Claude Code (개별 설정: claude-custom)")
		assert.Regexp(t, `(?m)^설계·판단\s+planner\s+opus\s+opus-5-5 · low\s`, tables[2])
		assert.NotContains(t, tables[3], "claude-custom")
		assert.Regexp(t, `(?m)^설계·판단\s+planner\s+opus\s+opus-5-5 · max\s`, tables[3])
		data, err := os.ReadFile(filepath.Join(got.dir, "autopus.yaml"))
		require.NoError(t, err)
		assert.NotContains(t, string(data), "claude-custom")
		assert.NotContains(t, string(data), "efforts")
		assert.Equal(t, map[string]string{"codex": "codex-custom"}, composeBindLoad(t, got.dir).Quality.Providers)
		block := got.out[strings.Index(got.out, "적용할 설정\n"):strings.Index(got.out, tuiuxReview+"\n")]
		assert.NotContains(t, block, "claude-custom")
		assert.NotContains(t, block, "efforts")
		assert.Contains(t, block, "\n  quality.providers: codex=codex-custom\n")
		assert.Equal(t, control.npd, got.npd)
		analysis := readComposeCodexProfile(t, got.dir, "reverse-engineer")
		assert.NotEqual(t, "gpt-6.1-sol", analysis.Model)
		assert.Equal(t, readComposeCodexProfile(t, control.dir, "reverse-engineer"), analysis)
	})
	t.Run("(c′) CAT-A twice", func(t *testing.T) {
		got := run(t, "CAT-A")
		assert.Equal(t, "gpt-6.1-sol", readComposeCodexProfile(t, got.dir, "reverse-engineer").Model)
	})
}
