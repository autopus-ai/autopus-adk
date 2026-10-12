package cli

// SPEC-MODELUX-002 probe A2 (plan.md Risk-First Integration Probe): the form
// test seam runs real huh fields in accessible mode over scripted input.
// huh v1.0.0 builds a new bufio.Scanner for every accessible prompt, so forms
// run back to back over one stream must each read exactly their own line.

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runProbeA2Forms runs four Selects, a Confirm, and a sixth Select that meets
// the end of input, each as its own accessible form over in.
func runProbeA2Forms(t *testing.T, in io.Reader) ([]string, bool, string) {
	t.Helper()
	var out bytes.Buffer
	run := func(field huh.Field) {
		form := huh.NewForm(huh.NewGroup(field)).WithAccessible(true).WithInput(in).WithOutput(&out)
		require.NoError(t, form.Run())
	}
	values := []string{"a", "a", "a", "a"}
	for index := range values {
		run(huh.NewSelect[string]().Title("pick").Options(huh.NewOptions("a", "b", "c")...).Value(&values[index]))
	}
	confirmed := false
	run(huh.NewConfirm().Title("apply?").Value(&confirmed))
	afterEnd := "b"
	run(huh.NewSelect[string]().Title("last").Options(huh.NewOptions("a", "b", "c")...).Value(&afterEnd))
	return values, confirmed, afterEnd
}

// With one byte per Read every form reads its own line: 2 picks b, Enter
// keeps the bound default, y confirms, and input end keeps the default.
func TestComposeFormProbeA2_OneByteReaderFeedsEachFormItsOwnLine(t *testing.T) {
	values, confirmed, afterEnd := runProbeA2Forms(t, iotest.OneByteReader(strings.NewReader("2\n\n3\n\ny\n")))

	assert.Equal(t, []string{"b", "a", "c", "a"}, values)
	assert.True(t, confirmed)
	assert.Equal(t, "b", afterEnd, "input end keeps the bound default")
}

// The counter-probe: over a reader that returns everything at once, the
// first prompt's Scanner buffers every line, so the later forms see only the
// end of input and keep their defaults. This is why the seam needs the
// one-byte reader.
func TestComposeFormProbeA2_BufferedReaderLosesTheLaterLines(t *testing.T) {
	values, confirmed, _ := runProbeA2Forms(t, strings.NewReader("2\n\n3\n\ny\n"))

	assert.Equal(t, []string{"b", "a", "a", "a"}, values)
	assert.False(t, confirmed)
}
