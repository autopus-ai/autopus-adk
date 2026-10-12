package cli

// SPEC-MODELUX-002 Phase 4 (security I1, I2): the OMP line editor's model
// list and the OMP summary table print catalog and plan text only through
// terminalSafe, whatever reached them.

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderOMPCustomModelChoices_EscapesCatalogText(t *testing.T) {
	var out bytes.Buffer
	renderOMPCustomModelChoices(&out, []ompCustomModelChoice{{
		Provider: "openai-codex", Model: "gpt" + terminalHostile, Thinking: []string{"max" + terminalHostile},
	}})

	assert.Contains(t, out.String(), "openai-codex/gpt"+terminalSafe(terminalHostile))
	assertNoTerminalControls(t, out.String())
}

func TestRenderOMPQualitySummary_EscapesEveryCell(t *testing.T) {
	var out bytes.Buffer
	renderOMPQualitySummary(&out, ompProfileApplyPreviewPayload{
		Name: "balanced" + terminalHostile,
		Agents: []ompProfileAgentPreviewPayload{
			{Agent: "task" + terminalHostile, Source: ompProfileSourceAgent, Availability: ompProfileAvailabilityAvailable,
				EffectiveSelector: "p/m" + terminalHostile, EffectiveThinking: "max" + terminalHostile},
			{Agent: "scout", Availability: ompProfileAvailabilityUnavailable,
				RequestedSelector: "p/n" + terminalHostile, RequestedThinking: "high" + terminalHostile},
		},
	})

	assert.Contains(t, out.String(), "task"+terminalSafe(terminalHostile)+"*")
	assert.Contains(t, out.String(), "p/n"+terminalSafe(terminalHostile)+" (unavailable)")
	assertNoTerminalControls(t, out.String())
}
