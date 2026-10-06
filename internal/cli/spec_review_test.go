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

// TestNewSpecReviewCmd_Structure verifies that the spec review command
// has the correct Use field and expected flags.
func TestNewSpecReviewCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := newSpecReviewCmd()
	require.NotNil(t, cmd)

	// Use should start with "review"
	assert.Equal(t, "review <SPEC-ID>", cmd.Use)

	assert.NotNil(t, cmd.Flags().Lookup("strategy"), "strategy flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("timeout"), "timeout flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("required-document"), "required-document flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("conditional-profile"), "conditional-profile flag must exist")
}

// TestNewSpecReviewCmd_RequiresExactlyOneArg verifies cobra arg validation.
func TestNewSpecReviewCmd_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()

	cmd := newSpecReviewCmd()
	require.NotNil(t, cmd)

	// Args constraint is cobra.ExactArgs(1)
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err, "no args should fail validation")

	err = cmd.Args(cmd, []string{"SPEC-001"})
	assert.NoError(t, err, "exactly one arg should pass")

	err = cmd.Args(cmd, []string{"SPEC-001", "extra"})
	assert.Error(t, err, "two args should fail validation")
}

func TestResolveSpecReviewProviderNames_Default(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("test-project")
	cfg.Spec.ReviewGate.Providers = []string{"claude", "gemini"}

	result := resolveSpecReviewProviderNames(cfg, false)
	assert.Equal(t, []string{"claude", "gemini"}, result)
}

func TestResolveSpecReviewProviderNames_MultiExpandsAndDeduplicates(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("test-project")
	cfg.Spec.ReviewGate.Providers = []string{"claude"}
	cfg.Orchestra.Commands["review"] = config.CommandEntry{
		Strategy:  "debate",
		Providers: []string{"claude", "codex"},
	}
	cfg.Orchestra.Providers = map[string]config.ProviderEntry{
		"gemini": {Binary: "gemini"},
		"codex":  {Binary: "codex"},
		"claude": {Binary: "claude"},
	}

	result := resolveSpecReviewProviderNames(cfg, true)
	assert.Equal(t, []string{"claude", "codex", "gemini"}, result)
}

// TestRunSpecReview_SpecLoadError verifies that runSpecReview returns an error
// when the SPEC directory does not exist.
func TestRunSpecReview_SpecLoadError(t *testing.T) {
	// Uses os.Chdir — not parallel-safe.
	dir := t.TempDir()

	orig, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(orig) }()

	require.NoError(t, os.Chdir(dir))

	execErr := runSpecReview(context.Background(), "SPEC-NONEXISTENT-999", "", 0)
	assert.Error(t, execErr, "runSpecReview should fail when SPEC dir is missing")
}

func TestRunSpecReview_PassApprovesSpec(t *testing.T) {
	dir := t.TempDir()
	specDir := scaffoldReviewSpec(t, dir, "SPEC-REVIEW-001")
	setFakeProviderOnPath(t, dir, "claude")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		// Default config has 2 providers (claude, gemini); both must return PASS so
		// SPEC-SPECREV-001 AC-VERD-1 dropped-provider rule does not flip to REVISE.
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{
			{Provider: "claude", Output: "VERDICT: PASS"},
			{Provider: "gemini", Output: "VERDICT: PASS"},
		}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	require.NoError(t, runSpecReview(context.Background(), "SPEC-REVIEW-001", "consensus", 10))

	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "approved", doc.Status)
}

func TestRunSpecReview_HonorsConfigPathFromContext(t *testing.T) {
	dir := t.TempDir()
	specDir := scaffoldReviewSpec(t, dir, "SPEC-REVIEW-CONFIG-001")
	altCfgDir := t.TempDir()

	cfg := config.DefaultFullConfig("test-project")
	cfg.Spec.ReviewGate.Providers = []string{"claude"}
	require.NoError(t, config.Save(altCfgDir, cfg))

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	var capturedProviders []string
	stubSpecReviewAssembly(t, func(cfg *config.HarnessConfig, names []string) []orchestra.ProviderConfig {
		require.NotNil(t, cfg)
		capturedProviders = append([]string(nil), names...)
		return []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	})

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: "VERDICT: PASS"}}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	ctx := withGlobalFlags(context.Background(), globalFlags{
		ConfigPath: filepath.Join(altCfgDir, "autopus.yaml"),
	})
	require.NoError(t, runSpecReview(ctx, "SPEC-REVIEW-CONFIG-001", "consensus", 10))

	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "approved", doc.Status)
	assert.Equal(t, []string{"claude"}, capturedProviders)
}

func TestRunSpecReview_ReviseDoesNotApproveSpec(t *testing.T) {
	dir := t.TempDir()
	specDir := scaffoldReviewSpec(t, dir, "SPEC-REVIEW-002")
	setFakeProviderOnPath(t, dir, "claude")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	callCount := 0
	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		callCount++
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{
			Provider: "claude",
			Output:   "VERDICT: REVISE\nFINDING: [major] [correctness] REQ-001 Needs revision",
		}}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	require.NoError(t, runSpecReview(context.Background(), "SPEC-REVIEW-002", "consensus", 10))

	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "draft", doc.Status)
	assert.GreaterOrEqual(t, callCount, 1)
}

// Regression guard for GitHub issue #38.
// A subsequent review on a SPEC that is already `completed` (or `implemented`)
// MUST NOT silently downgrade its status to `approved`. Re-review of shipped
// work must be an explicit user action, not a side effect of an exploratory
// review pass.
func TestRunSpecReview_RefusesToRegressCompletedStatus(t *testing.T) {
	dir := t.TempDir()
	specDir := scaffoldReviewSpec(t, dir, "SPEC-REVIEW-REGRESS-001")
	require.NoError(t, spec.UpdateStatus(specDir, "completed"))
	setFakeProviderOnPath(t, dir, "claude")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: "VERDICT: PASS"}}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	require.NoError(t, runSpecReview(context.Background(), "SPEC-REVIEW-REGRESS-001", "consensus", 10))

	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "completed", doc.Status, "completed SPEC must not be silently regressed to approved")
}

// Regression guard for GitHub issue #38.
// When PersistFindings or PersistReview fails mid-pipeline (e.g. the SPEC
// directory disappeared between resolution and persistence), the CLI must
// fail fast instead of continuing through syncReviewedSpecStatus. Silent
// continuation was the enabling condition for the observed data corruption.
func TestRunSpecReview_PersistFailureAbortsPipeline(t *testing.T) {
	dir := t.TempDir()
	specDir := scaffoldReviewSpec(t, dir, "SPEC-REVIEW-ABORT-001")
	setFakeProviderOnPath(t, dir, "claude")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		// Simulate the SPEC dir vanishing between resolve and persist.
		if err := os.RemoveAll(specDir); err != nil {
			return nil, err
		}
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: "VERDICT: PASS"}}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	err = runSpecReview(context.Background(), "SPEC-REVIEW-ABORT-001", "consensus", 10)
	require.Error(t, err, "persist failure must abort the pipeline")
	assert.Contains(t, err.Error(), "review")

	// The SPEC directory must remain absent — we did not recreate it.
	_, statErr := os.Stat(specDir)
	assert.True(t, os.IsNotExist(statErr), "pipeline must not recreate a vanished SPEC dir")
}

func scaffoldReviewSpec(t *testing.T, dir, specID string) string {
	t.Helper()
	require.NoError(t, spec.Scaffold(dir, specID[len("SPEC-"):], "리뷰 테스트"))
	return filepath.Join(dir, ".autopus", "specs", specID)
}

func setFakeProviderOnPath(t *testing.T, dir, binary string) {
	t.Helper()
	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	path := filepath.Join(binDir, binary)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
