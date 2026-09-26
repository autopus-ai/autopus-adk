package codex

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

// A supervisor tuple an earlier Autopus release wrote must still read as
// managed after the Sol tier moves to GPT-6; otherwise the update preserves
// it as a user override and the config never reaches the new model.
func TestIsKnownManagedCodexSupervisorTuple_RecognizesPreviousSol(t *testing.T) {
	t.Parallel()

	quote := strconv.Quote
	tests := []struct {
		name   string
		model  string
		effort string
		want   bool
	}{
		{"current Sol xhigh", config.CodexSolModel, config.CodexEffortXHigh, true},
		{"previous Sol xhigh", config.CodexPreviousSolModel, config.CodexEffortXHigh, true},
		{"previous Sol ultra", config.CodexPreviousSolModel, config.CodexEffortUltra, true},
		{"previous Sol medium is user-owned", config.CodexPreviousSolModel, config.CodexEffortMedium, false},
		{"Terra is never a managed supervisor", "gpt-5.6-terra", config.CodexEffortXHigh, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isKnownManagedCodexSupervisorTuple(quote(tt.model), true, quote(tt.effort), true)
			assert.Equal(t, tt.want, got)
		})
	}
}
