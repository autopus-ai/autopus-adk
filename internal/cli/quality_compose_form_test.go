package cli

// SPEC-MODELUX-002 T1 (REQ-MUX2-01, REQ-MUX2-02, REQ-MUX2-17; acceptance S1,
// S15): the form seams, the Select and Confirm builders, and the source
// rules every composition form keeps.

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

func newFormTestCommand(input string) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(formInput(input))
	return cmd, &out
}

// The Input Mode Contract: forms only for the process stdin with stdin and
// stdout both terminals; a reader set on the command, a stdout that is not
// a terminal, and a false TTY probe take the line driver.
func TestComposeFormsProbe_FormsOnlyForTheProcessTerminal(t *testing.T) {
	originalTTY, originalOut := composeTTYProbe, composeOutputTerminal
	t.Cleanup(func() { composeTTYProbe, composeOutputTerminal = originalTTY, originalOut })
	cases := []struct {
		name          string
		setIn         bool
		tty, terminal bool
		want          bool
	}{
		{"process stdin and stdout are terminals", false, true, true, true},
		{"a reader set on the command", true, true, true, false},
		{"stdout is not a terminal", false, true, false, false},
		{"stdin is not a terminal", false, false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			composeTTYProbe = func() bool { return tc.tty }
			composeOutputTerminal = func(w io.Writer) bool { return tc.terminal }
			cmd := &cobra.Command{}
			if tc.setIn {
				cmd.SetIn(strings.NewReader(""))
			}
			assert.Equal(t, tc.want, composeFormsProbe(cmd))
		})
	}
}

func TestComposeOutputTerminal_PipeAndBufferAreNotTerminals(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })

	assert.False(t, composeOutputTerminal(writer))
	assert.False(t, composeOutputTerminal(&bytes.Buffer{}))
}

// The wizard's Windows guard: a legacy console sets neither variable.
func TestFormsSupported_WindowsNeedsWindowsTerminalOrTermProgram(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	assert.False(t, tui.FormsSupported("windows", env(nil)))
	assert.True(t, tui.FormsSupported("windows", env(map[string]string{"WT_SESSION": "1"})))
	assert.True(t, tui.FormsSupported("windows", env(map[string]string{"TERM_PROGRAM": "vscode"})))
	assert.True(t, tui.FormsSupported("darwin", env(nil)))
}

// The default runner binds the form to the command's streams: huh's NewForm
// writes to os.Stderr otherwise, and `auto update 2>err.log` would lose it.
func TestComposeRunForm_DefaultBindsTheCommandStreams(t *testing.T) {
	cmd, out := newFormTestCommand("2\n")
	value := "a"
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("pick").Options(huh.NewOptions("a", "b")...).Value(&value),
	)).WithAccessible(true)

	require.NoError(t, composeRunForm(cmd, form))

	assert.Equal(t, "b", value)
	assert.Contains(t, out.String(), "2. b")
}

func TestComposeSelect_BindsTheChoiceAndEnterKeepsTheDefault(t *testing.T) {
	useAccessibleComposeForms(t)
	options := []composeOption[string]{{"keep", "k"}, {"edit", "e"}, {"preset", "p"}}

	cmd, out := newFormTestCommand("\n3\n")
	first, err := composeSelect(cmd, "Change?", "", options, "k")
	require.NoError(t, err)
	second, err := composeSelect(cmd, "Change?", "", options, "k")
	require.NoError(t, err)

	assert.Equal(t, "k", first)
	assert.Equal(t, "p", second)
	assert.Contains(t, out.String(), "1. keep\n2. edit\n3. preset\n")
}

// Every title and label is terminal-safe: text from autopus.yaml or a
// catalog cannot drive the terminal through a form (S15).
func TestComposeSelect_TitleAndLabelsAreTerminalSafe(t *testing.T) {
	useAccessibleComposeForms(t)
	cmd, out := newFormTestCommand("\n")

	_, err := composeSelect(cmd, "pick \x1b]11;?\a", "", []composeOption[int]{{"fast\u202e\u200b", 0}}, 0)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "fast\\u202e\\u200b")
	assert.Contains(t, out.String(), `pick \x1b]11;?\a`)
	assertNoTerminalControls(t, out.String())
}

func TestComposeConfirm_DefaultsToNo(t *testing.T) {
	useAccessibleComposeForms(t)
	cmd, _ := newFormTestCommand("\ny\n")

	first, err := composeConfirm(cmd, "Follow?")
	require.NoError(t, err)
	second, err := composeConfirm(cmd, "Follow?")
	require.NoError(t, err)

	assert.False(t, first)
	assert.True(t, second)
}

// Ctrl+C in any form is the interruption the step ends on (REQ-MUX2-14).
func TestComposeForms_UserAbortIsTheInterruption(t *testing.T) {
	abortComposeFormAt(t, 1)
	cmd, _ := newFormTestCommand("")
	_, err := composeSelect(cmd, "pick", "", []composeOption[int]{{"a", 0}}, 0)
	assert.True(t, errors.Is(err, errComposeInterrupted), "%v", err)

	abortComposeFormAt(t, 1)
	_, err = composeConfirm(cmd, "Follow?")
	assert.True(t, errors.Is(err, errComposeInterrupted), "%v", err)
}

// One builder makes every form directly under internal/cli, and nothing
// under internal/cli sends a terminal query of its own (S1, S15).
func TestComposeForms_OneBuilderAndNoTerminalQueries(t *testing.T) {
	var builders []string
	queries := 0
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ParseComments); err != nil {
			return err
		}
		source := string(data)
		if filepath.Dir(path) == "." {
			for range strings.Count(source, "huh.NewForm(") {
				builders = append(builders, path)
			}
		}
		for _, call := range []string{"HasDarkBackground", "BackgroundColor(", "termenv.NewOutput"} {
			queries += strings.Count(source, call)
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"quality_compose_form.go"}, builders)
	assert.Zero(t, queries)
}
