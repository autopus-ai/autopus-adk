package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

func qualityOMPWizardFixture(t *testing.T) (string, *ompCLIFakeRunner) {
	t.Helper()
	root, runner := writeOMPBalancedProject(t)
	cfg, err := config.LoadPreview(root)
	require.NoError(t, err)
	cfg.Quality.Default = "ultra"
	cfg.Platforms = []string{"omp", "claude-code"}
	require.NoError(t, config.Save(root, cfg))
	runner.catalog = ompCLIProfileNativeCatalogJSON()
	return root, runner
}

func runQualityOMPWizard(t *testing.T, dir, input string, deps ompPlatformDependencies, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	old, _, err := root.Find([]string{"quality"})
	require.NoError(t, err)
	root.RemoveCommand(old)
	root.AddCommand(newQualityCmdWithOMPDependencies(deps))
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetIn(strings.NewReader(input))
	root.SetArgs(append([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality"}, args...))
	err = root.Execute()
	return out.String(), err
}

func TestQualityOMPWizardAppliesChosenFamilyWithoutGlobalQualityChange(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	out, err := runQualityOMPWizard(t, dir, "omp\n1\n1\ny\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate:  func(context.Context, string, *config.HarnessConfig) error { return nil },
	})
	require.NoError(t, err, out)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "ultra", cfg.Quality.Default)
	assert.Equal(t, "balanced", cfg.RoleModelPolicy.Profile)
	assert.Equal(t, "openai", cfg.RoleModelPolicy.Family)
	assert.Empty(t, cfg.RoleModelPolicy.Profiles)
	assert.Contains(t, out, "openai-codex/gpt-5.6-luna")
	assert.Contains(t, out, "openai-codex/gpt-6-astra")
	assert.Contains(t, out, "task")
}

func TestQualityOMPWizardCancellationNeverWritesOrActivates(t *testing.T) {
	for name, confirmation := range map[string]string{"decline": "n\n", "enter": "\n", "eof": ""} {
		t.Run(name, func(t *testing.T) {
			dir, runner := qualityOMPWizardFixture(t)
			before := readAutopusConfigTree(t, dir)
			out, err := runQualityOMPWizard(t, dir, "omp\n1\n2\n"+confirmation, ompPlatformDependencies{
				newRunner: func() omp.OMPModelCatalogRunner { return runner },
				activate: func(context.Context, string, *config.HarnessConfig) error {
					t.Fatal("cancellation must not activate a profile")
					return nil
				},
			}, "--apply")
			require.NoError(t, err, out)
			assert.Equal(t, before, readAutopusConfigTree(t, dir))
			assert.Contains(t, out, "Cancelled")
		})
	}
}

func TestQualityOMPWizardBlocksUnavailableModelsBeforeConfirmation(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	runner.catalog = ompCLIBalancedCatalogWithout(t, "openai-codex/gpt-6-astra")
	before := readAutopusConfigTree(t, dir)
	out, err := runQualityOMPWizard(t, dir, "omp\n1\n1\ny\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate: func(context.Context, string, *config.HarnessConfig) error {
			t.Fatal("unavailable models must not activate")
			return nil
		},
	})
	require.Error(t, err)
	assert.Equal(t, before, readAutopusConfigTree(t, dir))
	assert.Contains(t, out, "task")
	assert.NotContains(t, out, "Apply these models?")
}

func TestQualityOMPWizardKeepsGlobalChoiceAvailable(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	out, err := runQualityOMPWizard(t, dir, "1\n2\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
	})
	require.NoError(t, err, out)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "balanced", cfg.Quality.Default)
	assert.Empty(t, cfg.RoleModelPolicy.Profile)
	assert.Empty(t, runner.calls, "global quality selection does not probe OMP")
}

func TestQualityOMPWizardRespectsExplicitBalancedDefinition(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	profile, ok := config.BuiltinRoleModelProfile("balanced", cfg.Quality, "openai", "overlay")
	require.True(t, ok)
	cfg.RoleModelPolicy.Profiles = map[string]config.RoleModelProfileConf{"balanced": profile}
	require.NoError(t, config.Save(dir, cfg))
	out, err := runQualityOMPWizard(t, dir, "omp\n1\ny\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate:  func(context.Context, string, *config.HarnessConfig) error { return nil },
	})
	require.NoError(t, err, out)
	loaded, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, profile, loaded.RoleModelPolicy.Profiles["balanced"])
	assert.NotContains(t, out, "Choose model family")
}

func TestQualityOMPWizardAbortBeforeModeLeavesConfigUntouched(t *testing.T) {
	dir, runner := qualityOMPWizardFixture(t)
	path := filepath.Join(dir, "autopus.yaml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = runQualityOMPWizard(t, dir, "omp\n", ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
	})
	require.Error(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Empty(t, runner.calls)
}
