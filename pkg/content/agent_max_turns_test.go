package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/content"
)

// A turn cap stops a long agent mid-task with no wrap-up step, so only the
// short judging agents carry one (#301).
func TestAgentMaxTurns_OnlyShortJudgingAgentsAreCapped(t *testing.T) {
	t.Parallel()
	capped := map[string]int{"ux-validator": 30}

	sources, err := content.LoadAgentSourcesFromFS(contentfs.FS, "agents")
	require.NoError(t, err)
	require.NotEmpty(t, sources)
	for _, src := range sources {
		assert.Equal(t, capped[src.Meta.Name], src.Meta.MaxTurns, "agent %s", src.Meta.Name)
		if src.Meta.MaxTurns == 0 {
			assert.NotContains(t, content.TransformAgentForOpenCode(src), "\nsteps:", "agent %s", src.Meta.Name)
		}
	}
}
