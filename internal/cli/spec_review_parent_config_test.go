package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestRunSpecReview_InheritsParentReviewWiringForNestedModule(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "autopus-desktop")
	require.NoError(t, os.MkdirAll(child, 0o755))
	specDir := scaffoldReviewSpec(t, child, "SPEC-REVIEW-PARENT-001")
	writeSparseModuleConfig(t, child)

	parentCfg := config.DefaultFullConfig("root-workspace")
	parentCfg.Spec.ReviewGate.Providers = []string{"claude"}
	parentCfg.Orchestra.Commands["review"] = config.CommandEntry{Strategy: "debate", Providers: []string{"claude"}}
	parentCfg.Orchestra.Providers = map[string]config.ProviderEntry{"claude": {Binary: "claude"}}
	require.NoError(t, config.Save(root, parentCfg))

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(child))

	var capturedProviders []string
	var capturedProjectName string
	stubSpecReviewAssembly(t, func(cfg *config.HarnessConfig, names []string) []orchestra.ProviderConfig {
		require.NotNil(t, cfg)
		capturedProjectName = cfg.ProjectName
		capturedProviders = append([]string(nil), names...)
		return []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	})

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: "VERDICT: PASS"}}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	require.NoError(t, runSpecReview(context.Background(), "SPEC-REVIEW-PARENT-001", "consensus", 10))

	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "approved", doc.Status)
	assert.Equal(t, "child-module", capturedProjectName)
	assert.Equal(t, []string{"claude"}, capturedProviders)
}

func TestLoadHarnessConfig_InheritsParentReviewGateWhenChildHasLocalOrchestra(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "autopus-desktop")
	require.NoError(t, os.MkdirAll(child, 0o755))
	writeSparseModuleConfigWithLocalOrchestra(t, child)

	parentCfg := config.DefaultFullConfig("root-workspace")
	parentCfg.Spec.ReviewGate.Providers = []string{"claude"}
	parentCfg.Spec.ReviewGate.Strategy = "debate"
	parentCfg.Quality.Default = "ultra"
	require.NoError(t, config.Save(root, parentCfg))

	cfg, err := loadHarnessConfigForDir(child, globalFlags{})
	require.NoError(t, err)

	assert.Equal(t, "child-module", cfg.ProjectName)
	assert.Equal(t, []string{"claude"}, cfg.Spec.ReviewGate.Providers)
	assert.Equal(t, "debate", cfg.Spec.ReviewGate.Strategy)
	assert.Equal(t, "ultra", cfg.Quality.Default)
	assert.Contains(t, cfg.Orchestra.Providers, "codex")
	assert.NotContains(t, cfg.Orchestra.Providers, "claude")
}

func TestLoadHarnessConfig_InheritsParentVerifyWhenChildSparse(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "autopus-desktop")
	require.NoError(t, os.MkdirAll(child, 0o755))
	writeSparseModuleConfig(t, child)

	parentCfg := config.DefaultFullConfig("root-workspace")
	parentCfg.Verify.Enabled = true
	parentCfg.Verify.DefaultViewport = "mobile"
	parentCfg.Verify.MaxFixAttempts = 4
	require.NoError(t, config.Save(root, parentCfg))

	cfg, err := loadHarnessConfigForDir(child, globalFlags{})
	require.NoError(t, err)

	assert.Equal(t, "child-module", cfg.ProjectName)
	assert.True(t, cfg.Verify.Enabled)
	assert.Equal(t, "mobile", cfg.Verify.DefaultViewport)
	assert.Equal(t, 4, cfg.Verify.MaxFixAttempts)
}

func TestLoadHarnessConfig_PreservesExplicitChildVerifyDisabled(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "autopus-desktop")
	require.NoError(t, os.MkdirAll(child, 0o755))
	writeSparseModuleConfigWithVerify(t, child, false)

	parentCfg := config.DefaultFullConfig("root-workspace")
	parentCfg.Verify.Enabled = true
	parentCfg.Verify.DefaultViewport = "mobile"
	require.NoError(t, config.Save(root, parentCfg))

	cfg, err := loadHarnessConfigForDir(child, globalFlags{})
	require.NoError(t, err)

	assert.False(t, cfg.Verify.Enabled)
	assert.Empty(t, cfg.Verify.DefaultViewport)
}

func TestValidateQualityPreset_InheritsParentQualityForNestedModule(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "autopus-desktop")
	require.NoError(t, os.MkdirAll(child, 0o755))
	writeSparseModuleConfig(t, child)

	parentCfg := config.DefaultFullConfig("root-workspace")
	parentCfg.Quality.Default = "balanced"
	require.NoError(t, config.Save(root, parentCfg))

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(child))

	require.NoError(t, validateQualityPreset(nil, "", "ultra"))
}

func TestRunSpecReview_ExplicitConfigDoesNotInheritParentReviewWiring(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "autopus-desktop")
	require.NoError(t, os.MkdirAll(child, 0o755))
	scaffoldReviewSpec(t, child, "SPEC-REVIEW-EXPLICIT-001")
	childConfigPath := writeSparseModuleConfig(t, child)

	parentCfg := config.DefaultFullConfig("root-workspace")
	parentCfg.Spec.ReviewGate.Providers = []string{"claude"}
	parentCfg.Orchestra.Providers = map[string]config.ProviderEntry{"claude": {Binary: "claude"}}
	require.NoError(t, config.Save(root, parentCfg))

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(child))

	var capturedProviders []string
	stubSpecReviewAssembly(t, func(_ *config.HarnessConfig, names []string) []orchestra.ProviderConfig {
		capturedProviders = append([]string(nil), names...)
		return nil
	})

	ctx := withGlobalFlags(context.Background(), globalFlags{ConfigPath: childConfigPath})
	err = runSpecReview(ctx, "SPEC-REVIEW-EXPLICIT-001", "consensus", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "사용 가능한 프로바이더")
	assert.Empty(t, capturedProviders)
}

func writeSparseModuleConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "autopus.yaml")
	body := []byte(`mode: full
project_name: child-module
platforms:
  - codex
isolate_rules: true
architecture:
  auto_generate: false
  enforce: false
lore:
  enabled: false
  auto_inject: false
  required_trailers: []
  stale_threshold_days: 0
spec:
  id_format: ""
  ears_types: []
hooks:
  pre_commit_arch: false
  pre_commit_lore: false
  react_ci_failure: false
  react_review: false
design:
  enabled: true
  max_context_lines: 80
  inject_on_review: true
  inject_on_verify: true
  external_imports: false
`)
	require.NoError(t, os.WriteFile(path, body, 0o644))
	return path
}

func writeSparseModuleConfigWithVerify(t *testing.T, dir string, enabled bool) string {
	t.Helper()
	path := writeSparseModuleConfig(t, dir)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	body := string(data) + "verify:\n  enabled: " + map[bool]string{true: "true", false: "false"}[enabled] + "\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func writeSparseModuleConfigWithLocalOrchestra(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "autopus.yaml")
	body := []byte(`mode: full
project_name: child-module
platforms:
  - codex
isolate_rules: true
architecture:
  auto_generate: false
  enforce: false
lore:
  enabled: false
  auto_inject: false
  required_trailers: []
  stale_threshold_days: 0
spec:
  id_format: ""
  ears_types: []
orchestra:
  enabled: true
  providers:
    codex:
      binary: codex
  commands:
    plan:
      strategy: consensus
      providers:
        - codex
hooks:
  pre_commit_arch: false
  pre_commit_lore: false
  react_ci_failure: false
  react_review: false
design:
  enabled: true
  max_context_lines: 80
  inject_on_review: true
  inject_on_verify: true
  external_imports: false
`)
	require.NoError(t, os.WriteFile(path, body, 0o644))
	return path
}
