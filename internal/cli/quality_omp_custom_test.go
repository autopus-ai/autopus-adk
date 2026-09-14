package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// A per-agent choice must reach the persisted policy as an exact selector for
// the bundled agent the operator named, and must leave the other agents on the
// base profile instead of inventing a route for them.
func TestQualityOMPWizardCustomPinsChosenModelPerBundledAgent(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	runner.catalog = ompCLIBalancedCatalogJSON()

	// custom -> task: model 4 (openai-codex/gpt-6-astra) at thinking max;
	// every other agent keeps the balanced default.
	out, err := runQualityOMPWizard(t, dir, "omp\ncustom\ngpt\n\n\n\n4\nmax\n\ny\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate:  func(context.Context, string, *config.HarnessConfig) error { return nil },
	})
	require.NoError(t, err, out)
	assert.Contains(t, out, "task=openai-codex/gpt-6-astra:max")

	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "balanced", cfg.RoleModelPolicy.Profile)
	pinned := cfg.RoleModelPolicy.Agents["task"].Candidates
	require.Len(t, pinned, 1)
	assert.Equal(t, "openai-codex/gpt-6-astra", pinned[0].Selector)
	assert.Equal(t, "max", pinned[0].Thinking)
	for _, agent := range []string{"scout", "reviewer", "security-reviewer", "sonic"} {
		assert.NotContains(t, cfg.RoleModelPolicy.Agents, agent,
			"an unpinned agent must stay on the base profile")
	}
}

// Pinning nothing is not a change: the wizard must not persist a policy that
// merely restates the base profile.
func TestQualityOMPWizardCustomWithoutPinsWritesNothing(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	runner.catalog = ompCLIBalancedCatalogJSON()
	before := readAutopusConfigTree(t, dir)

	out, err := runQualityOMPWizard(t, dir, "omp\ncustom\ngpt\n\n\n\n\n\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate: func(context.Context, string, *config.HarnessConfig) error {
			t.Fatal("an empty selection must not activate a profile")
			return nil
		},
	})
	require.NoError(t, err, out)
	assert.Contains(t, out, "No agent was pinned")
	assert.Equal(t, before, readAutopusConfigTree(t, dir))
}

// A thinking level the installed model does not report must fail closed rather
// than fall back to a level the session cannot run.
func TestQualityOMPWizardCustomRejectsUnsupportedThinking(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	runner.catalog = ompCLIBalancedCatalogJSON()
	before := readAutopusConfigTree(t, dir)

	// scout requires fast_validation, which claude-sonnet-5 (2) declares.
	_, err := runQualityOMPWizard(t, dir, "omp\ncustom\ngpt\n2\nminimal\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate: func(context.Context, string, *config.HarnessConfig) error {
			t.Fatal("an unsupported thinking level must not activate a profile")
			return nil
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support thinking")
	assert.Equal(t, before, readAutopusConfigTree(t, dir))
}

// A model the catalog says cannot serve the agent's capability is refused at
// the prompt, naming the models that can. Accepting it would only surface as a
// blocker after every remaining prompt was answered.
func TestQualityOMPWizardCustomRefusesModelMissingTheAgentCapability(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	runner.catalog = ompCLIBalancedCatalogJSON()
	before := readAutopusConfigTree(t, dir)

	// scout requires fast_validation; claude-fable-5-1 (1) does not declare it.
	_, err := runQualityOMPWizard(t, dir, "omp\ncustom\ngpt\n1\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate: func(context.Context, string, *config.HarnessConfig) error {
			t.Fatal("an ineligible model must not activate a profile")
			return nil
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not declare fast_validation")
	assert.Contains(t, err.Error(), "anthropic/claude-sonnet-5")
	assert.Equal(t, before, readAutopusConfigTree(t, dir))
}
