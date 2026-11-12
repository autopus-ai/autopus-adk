package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The plan and brainstorm defaults left orchestra.commands; both commands
// must still run with the strategy those defaults gave them.
func TestResolveStrategy_RetiredCommandDefaultsKeepTheirStrategy(t *testing.T) {
	t.Parallel()
	conf := &config.DefaultFullConfig("demo").Orchestra

	assert.Equal(t, "debate", resolveStrategy(conf, "brainstorm", ""))
	assert.Equal(t, "consensus", resolveStrategy(conf, "plan", ""))
	assert.Equal(t, "fastest", resolveStrategy(conf, "brainstorm", "fastest"))

	conf.Commands = map[string]config.CommandEntry{"brainstorm": {Strategy: "consensus"}}
	assert.Equal(t, "consensus", resolveStrategy(conf, "brainstorm", ""))
}
