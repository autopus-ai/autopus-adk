package cli

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// useAccessibleComposeForms makes the composition step take its form path
// and run every form as the real huh fields in accessible mode on the
// command's streams (SPEC-MODELUX-002 REQ-MUX2-17). It returns the number of
// forms run so far.
func useAccessibleComposeForms(t *testing.T) *int {
	t.Helper()
	return useComposeFormRunner(t, func(int) error { return nil })
}

// abortComposeFormAt is useAccessibleComposeForms whose nth form, counted
// from 1, returns huh.ErrUserAborted instead of running, as Ctrl+C does.
func abortComposeFormAt(t *testing.T, nth int) *int {
	t.Helper()
	return useComposeFormRunner(t, func(run int) error {
		if run == nth {
			return huh.ErrUserAborted
		}
		return nil
	})
}

func useComposeFormRunner(t *testing.T, before func(run int) error) *int {
	t.Helper()
	originalProbe, originalRun := composeFormsProbe, composeRunForm
	t.Cleanup(func() { composeFormsProbe, composeRunForm = originalProbe, originalRun })
	runs := new(int)
	composeFormsProbe = func(*cobra.Command) bool { return true }
	composeRunForm = func(cmd *cobra.Command, form *huh.Form) error {
		*runs++
		if err := before(*runs); err != nil {
			return err
		}
		return form.WithAccessible(true).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).Run()
	}
	return runs
}

// formInput feeds accessible forms one byte per Read: every accessible
// prompt builds its own Scanner, which would otherwise buffer the answers
// queued behind it (probe A2).
func formInput(lines ...string) io.Reader {
	return iotest.OneByteReader(strings.NewReader(strings.Join(lines, "")))
}
