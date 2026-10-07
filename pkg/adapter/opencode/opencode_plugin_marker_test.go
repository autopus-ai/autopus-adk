package opencode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// V1PluginExport tells the two generated plugins apart for `auto doctor`: only
// the V1 plugin carries it.
func TestV1PluginExport_MarksOnlyTheV1Plugin(t *testing.T) {
	t.Parallel()

	v1, err := renderHookPlugin(nil)
	require.NoError(t, err)
	v2, err := renderHookPluginV2(nil)
	require.NoError(t, err)
	assert.True(t, strings.Contains(v1, "\n"+V1PluginExport+"\n"), "the V1 plugin carries the export line")
	assert.NotContains(t, v2, V1PluginExport)
}
