package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// In autopus mode the custom editor asks the generated Autopus agents and no
// bundled name. A task pin would fold onto the roles that collapsed onto task
// before v0.50.127 (Route Contract rule 2), so pinning "task" used to hand
// frontend-specialist and ux-validator a model without their capability and
// the plan refused it.
func TestQualityOMPWizardCustomAutopusPinsOneAutopusAgent(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	runner.catalog = ompCLIBalancedCatalogJSON()

	// reviewer is the third agent: model 4 (openai-codex/gpt-6-astra) at max.
	input := "omp\ncustom\ngpt\n" + strings.Repeat("\n", 2) + "4\nmax\n" + strings.Repeat("\n", 1) + "y\n"
	out, err := runQualityOMPWizard(t, dir, input, ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate:  func(context.Context, string, *config.HarnessConfig) error { return nil },
	})
	require.NoError(t, err, out)
	assert.Contains(t, out, "reviewer=openai-codex/gpt-6-astra:max")
	for _, bundled := range []string{"task (", "scout (", "sonic ("} {
		assert.NotContains(t, out, bundled, "autopus mode must not offer a bundled name")
	}

	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "balanced", cfg.RoleModelPolicy.Profile)
	pinned := cfg.RoleModelPolicy.Agents["reviewer"].Candidates
	require.Len(t, pinned, 1)
	assert.Equal(t, "openai-codex/gpt-6-astra", pinned[0].Selector)
	assert.Len(t, cfg.RoleModelPolicy.Agents, 1, "only the chosen agent is pinned")
}
