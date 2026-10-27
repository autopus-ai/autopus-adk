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

// Pinning nothing is not a change: the wizard must not persist a policy that
// merely restates the base profile.
func TestQualityOMPWizardCustomWithoutPinsWritesNothing(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	runner.catalog = ompCLIBalancedCatalogJSON()
	before := readAutopusConfigTree(t, dir)

	out, err := runQualityOMPWizard(t, dir, "omp\ncustom\ngpt\n"+strings.Repeat("\n", 17), ompPlatformDependencies{
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

	// annotator requires fast_validation, which claude-sonnet-5-5 (3) declares.
	_, err := runQualityOMPWizard(t, dir, "omp\ncustom\ngpt\n3\nminimal\n", ompPlatformDependencies{
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

	// annotator requires fast_validation; claude-fable-5-1 (1) does not declare it.
	_, err := runQualityOMPWizard(t, dir, "omp\ncustom\ngpt\n1\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate: func(context.Context, string, *config.HarnessConfig) error {
			t.Fatal("an ineligible model must not activate a profile")
			return nil
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not declare fast_validation")
	assert.Contains(t, err.Error(), "anthropic/claude-sonnet-5-5")
	assert.Equal(t, before, readAutopusConfigTree(t, dir))
}
