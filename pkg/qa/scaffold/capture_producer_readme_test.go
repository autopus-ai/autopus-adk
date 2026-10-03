package scaffold

import (
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestCaptureReadmeEmbedsValidatingGUIExplorePacks is the reason the gui-explore
// pack may live in a document at all. A snippet nobody validates rots into the same
// unrunnable pack this change removed from the generator, so every yaml block in the
// README is parsed and validated as a real Journey Pack.
func TestCaptureReadmeEmbedsValidatingGUIExplorePacks(t *testing.T) {
	t.Parallel()
	blocks := exploreBlocks(t, captureReadmeBody(projectSignals{PackageManager: "npm"}))
	// Two: the browser pack and the desktop pack. A future edit must not silently
	// drop one and leave that surface undocumented.
	require.GreaterOrEqual(t, len(blocks), 2, "README must carry both gui-explore examples")

	ids := make([]string, 0, len(blocks))
	for _, block := range blocks {
		var pack journey.Pack
		require.NoErrorf(t, yaml.Unmarshal([]byte(block), &pack), "block:\n%s", block)
		require.NoErrorf(t, journey.Validate(pack, t.TempDir()), "block:\n%s", block)
		assert.Equal(t, "gui-explore", pack.Adapter.ID)
		assert.Equal(t, expectedCapturePolicy(), pack.GUI.Capture)
		ids = append(ids, pack.ID)
	}
	assert.ElementsMatch(t, []string{BrowserGUIJourneyID, DesktopGUIJourneyID}, ids)
}

// TestCaptureReadmeExamplesSelectTheExploreSubset guards the two properties that
// decide whether a copied example can pass: it must run the tagged subset instead of
// the whole suite, and it must certify no artifacts, because the harness owns
// capture_index and the guard receipt.
func TestCaptureReadmeExamplesSelectTheExploreSubset(t *testing.T) {
	t.Parallel()
	blocks := exploreBlocks(t, captureReadmeBody(projectSignals{PackageManager: "npm"}))
	require.Len(t, blocks, 2)

	for _, block := range blocks {
		var pack journey.Pack
		require.NoError(t, yaml.Unmarshal([]byte(block), &pack))
		assert.Contains(t, pack.Command.Argv, "--grep")
		assert.Contains(t, pack.Command.Argv, exploreTag)
		assert.Empty(t, pack.Artifacts)
		assert.NotContains(t, block, "artifacts:")
	}

	body := captureReadmeBody(projectSignals{PackageManager: "npm"})
	assert.Contains(t, body, "@explore")
	assert.Contains(t, body, "autopus-capture.fixture.cjs")
	assert.Contains(t, body, ".autopus/qa/journeys/browser-gui-explore.yaml")
	assert.Contains(t, body, ".autopus/qa/journeys/desktop-gui-explore.yaml")
}

// TestCaptureReadmeExampleTargetsDetectedBaseURL wires baseURL detection through to
// the emitted document, which is now the only place a wrong origin would bite: the
// example is what a project copies, so a guessed origin there becomes a pack that
// fails its own origin guard.
func TestCaptureReadmeExampleTargetsDetectedBaseURL(t *testing.T) {
	t.Parallel()
	body := captureReadmeBody(projectSignals{PackageManager: "npm", BaseOrigin: "http://127.0.0.1:4173"})

	blocks := exploreBlocks(t, body)
	require.Len(t, blocks, 2)
	var pack journey.Pack
	require.NoError(t, yaml.Unmarshal([]byte(blocks[0]), &pack))
	assert.Equal(t, BrowserGUIJourneyID, pack.ID)
	assert.Equal(t, []string{"http://127.0.0.1:4173"}, pack.GUI.AllowedOrigins)
	assert.NotContains(t, body, defaultBrowserGUIOrigin)
}

// yamlBlocks returns the body of every fenced yaml block. The README also carries
// ts, js, and text blocks, so the opening fence is matched exactly.
func yamlBlocks(readme string) []string {
	const fence = "```"
	var blocks []string
	var current []string
	open := false
	for _, line := range strings.Split(readme, "\n") {
		switch {
		case !open && line == fence+"yaml":
			open = true
			current = nil
		case open && line == fence:
			open = false
			blocks = append(blocks, strings.Join(current, "\n")+"\n")
		case open:
			current = append(current, line)
		}
	}
	return blocks
}
