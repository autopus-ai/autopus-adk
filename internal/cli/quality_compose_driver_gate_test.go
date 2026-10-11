package cli

import (
	"bytes"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

const driverNoticeBoth = "참고: ultra, balanced 프리셋이 이전 버전 기본값입니다. " +
	"대화형 auto update 또는 auto quality에서 갱신할 수 있습니다.\n"

// stubDriverProbes replaces the TTY probe and the CI variable the step reads.
func stubDriverProbes(t *testing.T, tty bool, ci string) {
	t.Helper()
	originalTTY, originalCI := composeTTYProbe, composeCIEnv
	t.Cleanup(func() { composeTTYProbe, composeCIEnv = originalTTY, originalCI })
	composeTTYProbe = func() bool { return tty }
	composeCIEnv = func() string { return ci }
}

// TestComposeCITruthy pins A4: CI is truthy when it is not empty and, ignoring
// case, neither 0 nor false.
func TestComposeCITruthy(t *testing.T) {
	cases := map[string]bool{
		"": false, "  ": false, "0": false, "false": false, "FALSE": false, "False": false,
		"true": true, "TRUE": true, "1": true, "yes": true,
	}
	for value, want := range cases {
		assert.Equal(t, want, composeCITruthy(value), "CI=%q", value)
	}
}

// TestComposeGate_Mode pins the Update Prompt Contract rows.
func TestComposeGate_Mode(t *testing.T) {
	cases := []struct {
		name string
		gate composeGate
		tty  bool
		ci   string
		want composeMode
	}{
		{"interactive terminal", composeGate{}, true, "", composeModeAsk},
		{"CI=false stays interactive", composeGate{}, true, "false", composeModeAsk},
		{"CI=0 stays interactive", composeGate{}, true, "0", composeModeAsk},
		{"not a terminal", composeGate{}, false, "", composeModeNotice},
		{"--yes", composeGate{Yes: true}, true, "", composeModeNotice},
		{"--check", composeGate{Check: true}, true, "", composeModeNotice},
		{"--workspace", composeGate{Workspace: true}, true, "", composeModeNotice},
		{"CI=true", composeGate{}, true, "true", composeModeNotice},
		{"CI=1", composeGate{}, true, "1", composeModeNotice},
		{"preview", composeGate{Preview: true}, true, "", composeModeSilent},
		{"preview wins over every other flag", composeGate{Preview: true, Yes: true}, false, "true", composeModeSilent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubDriverProbes(t, tc.tty, tc.ci)
			assert.Equal(t, tc.want, tc.gate.mode())
		})
	}
}

// TestRunComposeStep_NonInteractiveNeverReadsStdin pins REQ-MUX-14: no read,
// no table, no catalog probe, and at most one notice line outside preview.
func TestRunComposeStep_NonInteractiveNeverReadsStdin(t *testing.T) {
	cases := []struct {
		name string
		gate composeGate
		tty  bool
		ci   string
		want string
	}{
		{"not a terminal", composeGate{}, false, "", driverNoticeBoth},
		{"--yes", composeGate{Yes: true}, true, "", driverNoticeBoth},
		{"--check", composeGate{Check: true}, true, "", driverNoticeBoth},
		{"--workspace", composeGate{Workspace: true}, true, "", driverNoticeBoth},
		{"CI=true", composeGate{}, true, "true", driverNoticeBoth},
		{"CI=1", composeGate{}, true, "1", driverNoticeBoth},
		{"preview prints nothing", composeGate{Preview: true}, true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubDriverProbes(t, tc.tty, tc.ci)
			probes := stubComposeCatalogProbe(t, composeCatalogA, nil)
			session, table, planner := newDriverSession(driverHistoricalConfig("claude-code", "codex"))
			cmd, out := newDriverCommand(driverFailingReader{t})

			result, err := runComposeStep(cmd, tc.gate, session)

			require.NoError(t, err)
			assert.Equal(t, tc.want, out.String())
			assert.Equal(t, composeDriverKept, result.Outcome)
			assert.Equal(t, driftGenerationSnapshot{}, result.Snapshot, "generation must probe for itself")
			assert.Empty(t, *probes)
			assert.Zero(t, table.draws())
			assert.Empty(t, planner.edits)
		})
	}
}

// TestRunComposeStep_NonInteractiveWithoutHistoricalPrintsNothing keeps the
// notice off a project whose built-in presets are current.
func TestRunComposeStep_NonInteractiveWithoutHistoricalPrintsNothing(t *testing.T) {
	stubDriverProbes(t, false, "")
	session, _, _ := newDriverSession(driverConfig("claude-code"))
	session.Text = composeText{} // the locale default must not matter here
	cmd, out := newDriverCommand(driverFailingReader{t})

	_, err := runComposeStep(cmd, composeGate{}, session)

	require.NoError(t, err)
	assert.Empty(t, out.String())
}

// TestRunComposeStep_InteractiveRunsTheSession shows the table, the GAP 1
// status, and the one question with `u` while a historical preset exists.
func TestRunComposeStep_InteractiveRunsTheSession(t *testing.T) {
	stubDriverProbes(t, true, "false")
	probes := stubComposeCatalogProbe(t, composeCatalogA, nil)
	session, table, _ := newDriverSession(driverHistoricalConfig("claude-code", "codex"))
	cmd, out := newDriverCommand(strings.NewReader("\n"))

	result, err := runComposeStep(cmd, composeGate{}, session)

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.Len(t, *probes, 1)
	assert.Equal(t, 1, table.draws())
	assert.Contains(t, out.String(),
		"quality.default = ultra   ultra: 이전 버전 기본값 (갱신 가능)   balanced: 이전 버전 기본값 (갱신 가능)\n")
	assert.Contains(t, out.String(),
		"구성을 바꿀까요? [Enter] 유지 · u) 최신 기본값으로 갱신 · g) 역할 그룹 · a) 고급\n")
}

// TestWriteComposeNotice_NamesOnlyHistoricalPresets names the historical
// presets, ultra first, and stays silent when there are none.
func TestWriteComposeNotice_NamesOnlyHistoricalPresets(t *testing.T) {
	ko := composeTextFor("ko")
	quality := driverConfig().Quality
	quality.Presets = maps.Clone(quality.Presets)
	quality.Presets["balanced"] = config.QualityPreset{Agents: maps.Clone(driverHB2)}
	var out bytes.Buffer

	writeComposeNotice(&out, quality, ko)
	assert.Equal(t, "참고: balanced 프리셋이 이전 버전 기본값입니다. "+
		"대화형 auto update 또는 auto quality에서 갱신할 수 있습니다.\n", out.String())

	out.Reset()
	writeComposeNotice(&out, driverConfig().Quality, ko)
	assert.Empty(t, out.String())

	out.Reset()
	writeComposeNotice(&out, driverHistoricalConfig().Quality, composeTextFor("en"))
	assert.Equal(t, "Note: previous-version default presets: ultra, balanced. "+
		"Update them from an interactive auto update or auto quality.\n", out.String())
}

// TestComposeDriverStatus_LabelsWrittenBuiltinPresets pins the GAP 1 labels of
// REQ-MUX-10; an absent preset carries no label.
func TestComposeDriverStatus_LabelsWrittenBuiltinPresets(t *testing.T) {
	ko := composeTextFor("ko")
	assert.Equal(t,
		"quality.default = ultra   ultra: 이전 버전 기본값 (갱신 가능)   balanced: 이전 버전 기본값 (갱신 가능)",
		composeDriverStatus(driverHistoricalConfig().Quality, ko))
	assert.Equal(t, "quality.default = balanced   ultra: 현재 기본값   balanced: 현재 기본값",
		composeDriverStatus(driverConfig().Quality, ko))

	// Fixture X: an invalid tier makes ultra customized, 12 agents off the default.
	x := driverHistoricalConfig().Quality
	ultra := maps.Clone(driverHU1)
	ultra["tester"] = "gpt"
	x.Presets["ultra"] = config.QualityPreset{Agents: ultra}
	delete(x.Presets, "balanced")
	assert.Equal(t, "quality.default = ultra   ultra: 사용자 수정 (12개 다름)", composeDriverStatus(x, ko))
	assert.Equal(t, "quality.default = ultra   ultra: customized (12 differ)",
		composeDriverStatus(x, composeTextFor("en")))
}

// TestComposeDriverStatus_EscapesControlCharacters keeps a YAML preset name
// from writing terminal control sequences.
func TestComposeDriverStatus_EscapesControlCharacters(t *testing.T) {
	quality := driverConfig().Quality
	quality.Default = "bad\x1b[31mname\n"

	status := composeDriverStatus(quality, composeTextFor("ko"))

	assert.True(t, strings.HasPrefix(status, `quality.default = bad\x1b[31mname\n   `), status)
	assert.NotContains(t, status, "\x1b")
	assert.NotContains(t, status, "\n")
}

// TestComposeDriverSessionShow_DrawsWithoutReadingStdin is the init path for
// balanced or ultra: the table only, with the snapshot generation will read.
func TestComposeDriverSessionShow_DrawsWithoutReadingStdin(t *testing.T) {
	probes := stubComposeCatalogProbe(t, composeCatalogA, nil)
	session, table, _ := newDriverSession(driverConfig("claude-code", "codex"))
	cmd, out := newDriverCommand(driverFailingReader{t})

	snapshot, err := session.show(cmd)

	require.NoError(t, err)
	assert.Len(t, *probes, 1)
	assert.Equal(t, []byte(composeCatalogA), snapshot.codexCatalog)
	assert.True(t, snapshot.codexCatalogFixed)
	assert.Equal(t, []driftGenerationSnapshot{snapshot}, table.tables)
	assert.Contains(t, out.String(), "에이전트 모델 구성")
	assert.NotContains(t, out.String(), "구성을 바꿀까요?")
}

// TestComposeSeams_DefaultToTheRealProbes pins the REQ-MUX-17 seam defaults.
func TestComposeSeams_DefaultToTheRealProbes(t *testing.T) {
	assert.Equal(t, reflect.ValueOf(isStdinTTY).Pointer(), reflect.ValueOf(composeTTYProbe).Pointer())
	assert.Equal(t, reflect.ValueOf(tui.RunInitWizard).Pointer(), reflect.ValueOf(runInitWizard).Pointer())
	assert.Equal(t, reflect.ValueOf(tui.DetectSystemLang).Pointer(), reflect.ValueOf(composeLanguage).Pointer())
	t.Setenv("CI", "true")
	assert.Equal(t, "true", composeCIEnv())
}
