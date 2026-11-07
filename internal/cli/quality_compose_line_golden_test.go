package cli

// SPEC-MODELUX-002 S1 (REQ-MUX2-01, REQ-MUX2-15): scripted line input keeps
// the SPEC-MODELUX-001 line driver byte for byte. The goldens were first the
// outputs of HEAD 44773355 for the same inputs and were rewritten with
// -update-compose-table when the role groups shrank to the generated agents;
// the project directory is written <DIR>. The default form probe sees a
// reader set on the command and never runs a form.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countComposeFormRuns counts form runs without touching the form probe.
func countComposeFormRuns(t *testing.T) *int {
	t.Helper()
	original := composeRunForm
	t.Cleanup(func() { composeRunForm = original })
	runs := new(int)
	composeRunForm = func(*cobra.Command, *huh.Form, tui.NavFrame) error {
		*runs++
		return nil
	}
	return runs
}

func TestComposeLineInput_MatchesTheSPECMODELUX001Output(t *testing.T) {
	t.Run("update R enter", func(t *testing.T) {
		run := newComposeUpdateRun(t, true, "")
		dir := run.settled(t, "R")
		run.reaSettledOff(t, dir)
		runs := countComposeFormRuns(t)

		out, err := runComposeUpdate(t, strings.NewReader("\n"), "--dir", dir)

		require.NoError(t, err, out)
		assertComposeTableGolden(t, "line-update-R-enter.golden", []byte(strings.ReplaceAll(out, dir, "<DIR>")))
		assert.Zero(t, *runs)
	})
	for golden, input := range map[string]string{
		"line-quality-D-global.golden": "global\n1\n",
		"line-quality-D-groups.golden": "g\n4\n\nn\n",
	} {
		t.Run(golden, func(t *testing.T) {
			newComposeUpdateRun(t, true, "")
			dir, _ := composeSessionProject(t, "D")
			runs := countComposeFormRuns(t)

			out, err := runQualityCompose(t, dir, strings.NewReader(input))

			require.NoError(t, err, out)
			assertComposeTableGolden(t, golden, []byte(strings.ReplaceAll(out, dir, "<DIR>")))
			assert.Zero(t, *runs)
		})
	}
}

// A run that does not ask reads no input and runs no form (S1).
func TestComposeLineInput_NonInteractiveUpdateRunsNoForm(t *testing.T) {
	base := newComposeUpdateRun(t, true, "")
	settled := base.settled(t, "R")
	cases := []struct {
		name string
		tty  bool
		ci   string
		args []string
	}{
		{"not a terminal", false, "", nil},
		{"--yes", true, "", []string{"--yes"}},
		{"--check", true, "", []string{"--check"}},
		{"CI=true", true, "true", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base.rerun(t, tc.tty, tc.ci)
			runs := countComposeFormRuns(t)
			dir := cloneComposeProject(t, settled)

			out, err := runComposeUpdate(t, driverFailingReader{t}, append([]string{"--dir", dir}, tc.args...)...)

			require.NoError(t, err, out)
			assert.Zero(t, *runs)
		})
	}
}
