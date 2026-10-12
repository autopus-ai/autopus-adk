package cli

import (
	"errors"
	"io"
	"os"
	"runtime"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

// The composition form seams (SPEC-MODELUX-002 REQ-MUX2-17). Tests replace a
// seam and restore it with t.Cleanup: they run the real huh fields in
// accessible mode over scripted input while the rest of a command runs for
// real.
var (
	// composeFormsProbe decides the Input Mode Contract once the step asks:
	// huh forms when it reports true, the SPEC-MODELUX-001 line driver
	// otherwise.
	composeFormsProbe = composeFormsTerminal
	// composeRunForm runs one composition form on the command's streams.
	// huh's NewForm writes to os.Stderr unless told otherwise, so both
	// streams are bound explicitly and `auto update 2>err.log` keeps the
	// form on the terminal.
	composeRunForm = func(cmd *cobra.Command, form *huh.Form) error {
		return form.WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).Run()
	}
	// composeOutputTerminal reports whether w is a file attached to a
	// terminal.
	composeOutputTerminal = func(w io.Writer) bool {
		file, ok := w.(*os.File)
		return ok && term.IsTerminal(int(file.Fd()))
	}
)

// errComposeInterrupted is the step's answer to Ctrl+C in any of its forms
// (REQ-MUX2-14): nothing further is written, and each entry point ends its
// command its own way.
var errComposeInterrupted = errors.New("composition interrupted")

// composeFormsTerminal is the default Input Mode Contract decision: the
// command reads the process stdin, stdin and stdout are both terminals, and
// the terminal passes the init wizard's Windows guard. A reader set on the
// command (scripted input, tests), a piped stdout, and a legacy Windows
// console keep the line driver.
func composeFormsTerminal(cmd *cobra.Command) bool {
	in, ok := cmd.InOrStdin().(*os.File)
	return ok && in == os.Stdin && composeTTYProbe() &&
		composeOutputTerminal(cmd.OutOrStdout()) && tui.FormsSupported(runtime.GOOS, os.Getenv)
}

// composeOption is one choice of a composition select: the label the
// operator reads and the value the choice binds.
type composeOption[T comparable] struct {
	label string
	value T
}

// composeSelect asks one select question, one field to a form, and returns
// the chosen value; Enter keeps def, which should be one of the options'
// values. The title, the note, and every label pass through terminalSafe:
// huh draws them as they are, and preset names, descriptions, and catalog
// text come from files the operator may not have written (REQ-MUX2-02).
func composeSelect[T comparable](
	cmd *cobra.Command,
	title, note string,
	options []composeOption[T],
	def T,
) (T, error) {
	value := def
	choices := make([]huh.Option[T], 0, len(options))
	for _, option := range options {
		choices = append(choices, huh.NewOption(terminalSafe(option.label), option.value))
	}
	field := huh.NewSelect[T]().Title(terminalSafe(title)).Options(choices...).Value(&value)
	if note != "" {
		field = field.Description(terminalSafe(note))
	}
	if err := runComposeForm(cmd, field); err != nil {
		return def, err
	}
	return value, nil
}

// composeConfirm asks one yes/no question whose default is no.
func composeConfirm(cmd *cobra.Command, title string) (bool, error) {
	confirmed := false
	field := huh.NewConfirm().Title(terminalSafe(title)).Value(&confirmed)
	if err := runComposeForm(cmd, field); err != nil {
		return false, err
	}
	return confirmed, nil
}

// runComposeForm builds the one form of field with the wizard's theme and no
// fixed width, so huh uses the terminal's: a label naming real models is
// wider than the wizard's 50 cells. Ctrl+C becomes errComposeInterrupted.
func runComposeForm(cmd *cobra.Command, field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithTheme(tui.AutopusTheme())
	if err := composeRunForm(cmd, form); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errComposeInterrupted
		}
		return err
	}
	return nil
}
