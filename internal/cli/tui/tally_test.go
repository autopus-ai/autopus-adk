package tui_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

func TestTally_CountsOnlyFailLines(t *testing.T) {
	var buf bytes.Buffer
	tally := tui.NewTally(&buf)

	tui.OK(tally, "fine")
	tui.SKIP(tally, "degraded")
	tui.Info(tally, "note")
	assert.Zero(t, tally.Errors(), "OK, WARN and info lines are not failures")

	tui.FAIL(tally, "broken")
	tui.FAIL(tally, "also broken")
	assert.Equal(t, 2, tally.Errors())
	assert.Contains(t, buf.String(), "broken", "the tally forwards every line it counts")
	assert.Contains(t, buf.String(), "degraded")
}

func TestFAIL_PlainWriterStillPrints(t *testing.T) {
	var buf bytes.Buffer
	tui.FAIL(&buf, "broken")
	assert.Contains(t, buf.String(), "broken")
}

func TestWarnResultBox_IsNeitherPassNorFail(t *testing.T) {
	var buf bytes.Buffer
	tui.WarnResultBox(&buf, "review the warnings")
	out := buf.String()
	assert.Contains(t, out, "WARN")
	assert.Contains(t, out, "review the warnings")
	assert.NotContains(t, out, "FAIL")
	assert.NotContains(t, out, "PASS")
}
