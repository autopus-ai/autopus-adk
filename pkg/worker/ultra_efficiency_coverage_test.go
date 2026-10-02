package worker

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/worker/adapter"
	"github.com/insajin/autopus-adk/pkg/worker/routing"
	"github.com/stretchr/testify/assert"
)

func TestUltraEfficiencyCoverage_ResolveModelUsesConfiguredRouter(t *testing.T) {
	t.Setenv("AUTOPUS_A2A_POLICY_SIGNING_SECRET", "")
	config := routing.DefaultConfig()
	config.Enabled = true
	pe := NewPipelineExecutor(adapter.NewClaudeAdapter(), "", t.TempDir())
	pe.SetRouter(routing.NewRouter(config))

	model := pe.resolveModel("", "fix typo")

	assert.Equal(t, "claude-sonnet-5-5", model)
}
