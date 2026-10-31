package cli

// SPEC-TUIUX-001 T1 (REQ-TUX-07, REQ-TUX-15; acceptance S6, S15): where no
// form opens, the stdout and the normalized project digest stay those of the
// base, the same tree without any TUIUX source change. The goldens under
// evidence/base-goldens were written at b08ea644, the branch base of the
// implementation (8ec13935 with the merged SPEC-REVMODEL-001 implementation).
// The NPD files were written again on the v0.50.126 release tree without
// TUIUX: SPEC-TEAMAUTO-001 and its team fixes changed the generated auto-go
// and agent-teams skills and the deferred-tools rule, the agy fix changed the
// reverse-engineer agent, the REA fix raised the startup timeout of the Codex
// REA entry in .codex/config.toml, and the permission ledger fix added a
// managed .gitignore pattern. Only those files and the manifest, journal, and
// backup digests that follow from them changed, and the stdout goldens are
// still those of b08ea644 (evidence/base-goldens-recapture.md). Later NPD
// re-records for generated-file changes are listed in its numbered sections;
// section 13 adds the review hint line to quality-no-tty.stdout.
// TUIUX_WRITE_GOLDENS=1 writes them again: every flow then runs twice more
// than a second apart, and any difference between the two runs, such as a
// time field outside the NPD normalization, fails the write. It runs on a
// tree without TUIUX, never on the tree under test.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tuiuxGoldenDir = "../../.autopus/specs/SPEC-TUIUX-001/evidence/base-goldens"

// tuiuxBaseFlow is one recorded run: its stdout with the project written
// <DIR>, its NPD, and the form runs it made.
type tuiuxBaseFlow struct {
	name string
	// stdout reports whether the stdout is part of the golden. The IN flow
	// opens the init review after the base commit, so only its NPD is.
	stdout bool
	run    func(t *testing.T) (out, npd string, forms int)
}

func tuiuxBaseFlows() []tuiuxBaseFlow {
	return []tuiuxBaseFlow{
		{"init-yes", true, func(t *testing.T) (string, string, int) {
			newTUIUXEnv(t, true, "")
			dir := tuiuxProject(t)
			forms := countComposeFormRuns(t)
			out, err := runTUIUXInit(t, dir, strings.NewReader(""), "--yes")
			require.NoError(t, err, out)
			return out, tuiuxNPD(t, dir), *forms
		}},
		{"update-yes", true, func(t *testing.T) (string, string, int) {
			newTUIUXEnv(t, true, "")
			dir := tuiuxInitYes(t)
			forms := countComposeFormRuns(t)
			out, err := runTUIUXCommand(t, dir, strings.NewReader(""), "update", "--dir", dir, "--yes")
			require.NoError(t, err, out)
			return out, tuiuxNPD(t, dir), *forms
		}},
		{"update-ci", true, func(t *testing.T) (string, string, int) {
			newTUIUXEnv(t, true, "true")
			dir := tuiuxInitYes(t)
			forms := countComposeFormRuns(t)
			out, err := runTUIUXCommand(t, dir, strings.NewReader(""), "update", "--dir", dir)
			require.NoError(t, err, out)
			return out, tuiuxNPD(t, dir), *forms
		}},
		{"quality-line-b", true, func(t *testing.T) (string, string, int) {
			newTUIUXEnv(t, true, "")
			dir := tuiuxFixtureD(t)
			forms := countComposeFormRuns(t)
			out, err := runQualityCompose(t, dir, strings.NewReader("b\n"))
			require.NoError(t, err, out)
			return strings.ReplaceAll(out, dir, "<DIR>"), tuiuxNPD(t, dir), *forms
		}},
		{"quality-no-tty", true, func(t *testing.T) (string, string, int) {
			newTUIUXEnv(t, false, "")
			dir := tuiuxFixtureD(t)
			forms := countComposeFormRuns(t)
			out, err := runQualityCompose(t, dir, nil)
			require.Error(t, err)
			return strings.ReplaceAll(out, dir, "<DIR>") + "error: " + err.Error() + "\n", tuiuxNPD(t, dir), *forms
		}},
		{"init-pipe", true, func(t *testing.T) (string, string, int) {
			newTUIUXEnv(t, true, "")
			calls := stubTUIUXWizard(t, tuiuxW())
			forms := countComposeFormRuns(t)
			stubTUIUXFormsProbe(t, false)
			dir := tuiuxProject(t)
			out, err := runTUIUXInit(t, dir, strings.NewReader(""))
			require.NoError(t, err, out)
			require.Len(t, *calls, 1)
			return out, tuiuxNPD(t, dir), *forms
		}},
		{"init-in", false, func(t *testing.T) (string, string, int) {
			newTUIUXEnv(t, true, "")
			stubTUIUXWizard(t, tuiuxW())
			forms := useAccessibleComposeForms(t)
			dir := tuiuxProject(t)
			out, err := runTUIUXInit(t, dir, formInput("\n"))
			require.NoError(t, err, out)
			return out, tuiuxNPD(t, dir), *forms
		}},
	}
}

// tuiuxInitYes makes project p with `auto init --yes`, the starting point
// of the update flows.
func tuiuxInitYes(t *testing.T) string {
	t.Helper()
	dir := tuiuxProject(t)
	out, err := runTUIUXInit(t, dir, strings.NewReader(""), "--yes")
	require.NoError(t, err, out)
	return dir
}

// tuiuxGolden reads one base golden file.
func tuiuxGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tuiuxGoldenDir, name))
	require.NoError(t, err)
	return string(data)
}

// tuiuxFrameStrings are the frame and review strings no place without a form
// may print (S15).
var tuiuxFrameStrings = []string{
	"지금까지 고른 값", "Chosen so far", "방향키 이동", "Arrows move", "뒤로: b", "back: b",
	" · 모델 구성", " · Model composition", "적용할 설정",
}

func TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput(t *testing.T) {
	if os.Getenv("TUIUX_WRITE_GOLDENS") == "1" {
		writeTUIUXBaseGoldens(t)
		return
	}
	for _, flow := range tuiuxBaseFlows() {
		t.Run(flow.name, func(t *testing.T) {
			out, npd, forms := flow.run(t)

			assert.Equal(t, tuiuxGolden(t, flow.name+".npd"), npd)
			if !flow.stdout {
				return
			}
			assert.Equal(t, tuiuxGolden(t, flow.name+".stdout"), out)
			assert.Zero(t, forms)
			for _, frame := range tuiuxFrameStrings {
				assert.NotContains(t, out, frame)
			}
		})
	}
}

// writeTUIUXBaseGoldens runs every flow twice, more than a second apart,
// requires both runs to agree, and writes the goldens.
func writeTUIUXBaseGoldens(t *testing.T) {
	require.NoError(t, os.MkdirAll(tuiuxGoldenDir, 0o755))
	for _, flow := range tuiuxBaseFlows() {
		var outs, npds [2]string
		for i := range outs {
			t.Run(flow.name, func(t *testing.T) { outs[i], npds[i], _ = flow.run(t) })
			if i == 0 {
				time.Sleep(1100 * time.Millisecond)
			}
		}
		require.Equal(t, npds[0], npds[1], flow.name)
		require.Equal(t, outs[0], outs[1], flow.name)
		require.NoError(t, os.WriteFile(filepath.Join(tuiuxGoldenDir, flow.name+".npd"), []byte(npds[0]), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tuiuxGoldenDir, flow.name+".stdout"), []byte(outs[0]), 0o644))
	}
}
