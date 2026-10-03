package scaffold

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
)

// AC-QALOOP-018: the capture README carries a browser-staging pack that greps
// @journey and never the read-only @explore tag.
func TestCaptureReadmeCarriesJourneyPack(t *testing.T) {
	readme := captureReadmeBody(projectSignals{PackageManager: "npm"})
	assert.Contains(t, readme, "### The journey subset")
	assert.Contains(t, readme, ".autopus/qa/journeys/"+BrowserGUIJourneyPackID+".yaml")

	var pack journey.Pack
	require.NoError(t, yaml.Unmarshal([]byte(browserGUIJourneyPackExample(projectSignals{PackageManager: "npm"})), &pack))
	require.NoError(t, journey.Validate(pack, t.TempDir()))
	assert.Equal(t, []string{"browser-staging"}, pack.Lanes)
	assert.Equal(t, "playwright", pack.Adapter.ID)
	argv := strings.Join(pack.Command.Argv, " ")
	assert.Contains(t, argv, "--grep @journey")
	assert.NotContains(t, argv, "@explore")
	assert.Contains(t, argv, "test -- --grep", "npm needs the separator to forward --grep")
}

func TestJourneyGrepArgvPnpmHasNoSeparator(t *testing.T) {
	argv := strings.Join(journeyGrepArgv("pnpm"), " ")
	assert.NotContains(t, argv, " -- ")
	assert.Contains(t, argv, "--grep @journey")
}

// exploreBlocks returns the README's gui-explore pack examples. The README also
// carries the @journey pack, which runs under the playwright adapter and is
// checked by TestCaptureReadmeCarriesJourneyPack instead.
func exploreBlocks(t *testing.T, body string) []string {
	t.Helper()
	out := []string{}
	for _, block := range yamlBlocks(body) {
		var pack journey.Pack
		require.NoError(t, yaml.Unmarshal([]byte(block), &pack))
		if pack.Adapter.ID == "gui-explore" {
			out = append(out, block)
		}
	}
	return out
}
