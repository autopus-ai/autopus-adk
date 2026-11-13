package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/qa/intent"
)

func TestWriteQAIntentText_PrintsOptionDetails(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	writeQAIntentText(&out, qaIntentPayload{Report: intent.Report{ID: "INTENT-1", Questions: []intent.Question{{
		ID: "Q1", Criterion: "AC-1", Question: "Keep empty tables?",
		Options: []string{"kept", "omitted"}, Details: []string{"round-trips exactly", "shorter output"},
	}}}})
	assert.Contains(t, out.String(), "* 1. kept\n         round-trips exactly\n")
	assert.Contains(t, out.String(), "  2. omitted\n         shorter output\n")
}
