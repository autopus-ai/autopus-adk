package omp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMPModelProjectManaged_UpdateUsesLastEmittedOwnership(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	original := []byte("# user\nunknown: keep\n")
	configPath := filepath.Join(root, configFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, original, 0o640))

	runner := newModelIntegrationRunner()
	adapter := NewWithRoot(root).WithModelIntegrationRunner(runner)
	cfg := integrationHarnessConfig(config.RoleModelConfigModeProjectManaged)
	_, err := adapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	_, err = adapter.Update(context.Background(), cfg)
	require.NoError(t, err, "an unchanged profile must accept the last Autopus-emitted values")

	changed := projectManagedSafetyProfile(cfg)
	_, err = adapter.Update(context.Background(), changed)
	require.NoError(t, err, "a changed profile must replace only the last Autopus-emitted values")

	projectBytes := mustReadOMPReviewFile(t, configPath)
	assert.Contains(t, string(projectBytes), "approvalMode: write")
	assert.Contains(t, string(projectBytes), "mode: auto")
	ledgerPath := filepath.Join(root, OMPModelProjectOwnershipRelativePath)
	ledgerBeforeMutation := mustReadOMPReviewFile(t, ledgerPath)
	receiptPath := filepath.Join(root, OMPModelReceiptRelativePath)
	receiptBeforeMutation := mustReadOMPReviewFile(t, receiptPath)

	mutated := strings.Replace(string(projectBytes), "approvalMode: write", "approvalMode: ask", 1)
	require.NotEqual(t, string(projectBytes), mutated)
	require.NoError(t, os.WriteFile(configPath, []byte(mutated), 0o640))
	_, err = adapter.Update(context.Background(), changed)
	require.ErrorContains(t, err, "managed_key_conflict")
	assert.Equal(t, mutated, string(mustReadOMPReviewFile(t, configPath)))
	assert.Equal(t, ledgerBeforeMutation, mustReadOMPReviewFile(t, ledgerPath))
	assert.Equal(t, receiptBeforeMutation, mustReadOMPReviewFile(t, receiptPath))
}

func TestOMPModelProjectManaged_CleanRestoresExactPreimageOrFailsClosed(t *testing.T) {
	t.Parallel()

	t.Run("exact preimage", func(t *testing.T) {
		root := t.TempDir()
		original := []byte("# exact user preimage\r\nunknown: ${TOKEN_REF}\r\n")
		path := filepath.Join(root, configFile)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, original, 0o640))
		adapter := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner())
		_, err := adapter.Generate(context.Background(), integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))
		require.NoError(t, err)

		require.NoError(t, adapter.Clean(context.Background()))
		assert.Equal(t, original, mustReadOMPReviewFile(t, path))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
		assert.NoFileExists(t, filepath.Join(root, OMPModelReceiptRelativePath))
		assert.NoFileExists(t, filepath.Join(root, OMPModelProjectOwnershipRelativePath))
	})

	t.Run("user mutation", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, configFile)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("user: keep\n"), 0o600))
		adapter := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner())
		_, err := adapter.Generate(context.Background(), integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))
		require.NoError(t, err)

		mutated := append(mustReadOMPReviewFile(t, path), []byte("user_after_generate: keep\n")...)
		require.NoError(t, os.WriteFile(path, mutated, 0o600))
		receiptPath := filepath.Join(root, OMPModelReceiptRelativePath)
		ledgerPath := filepath.Join(root, OMPModelProjectOwnershipRelativePath)
		receipt := mustReadOMPReviewFile(t, receiptPath)
		ledger := mustReadOMPReviewFile(t, ledgerPath)

		err = adapter.Clean(context.Background())
		require.ErrorContains(t, err, "managed_key_conflict")
		assert.Equal(t, mutated, mustReadOMPReviewFile(t, path))
		assert.Equal(t, receipt, mustReadOMPReviewFile(t, receiptPath))
		assert.Equal(t, ledger, mustReadOMPReviewFile(t, ledgerPath))
	})
}

func TestOMPModelProjectManaged_ProbeNeverReceivesFullSecretConfig(t *testing.T) {
	t.Parallel()

	const sentinel = "sk-project-secret-sentinel"
	root := t.TempDir()
	path := filepath.Join(root, configFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("credential: "+sentinel+"\nunknown: keep\n"), 0o600))
	runner := &rejectSecretConfigRunner{modelIntegrationFakeRunner: newModelIntegrationRunner(), sentinel: sentinel}

	_, err := NewWithRoot(root).WithModelIntegrationRunner(runner).
		Generate(context.Background(), integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))
	require.NoError(t, err)
	assert.False(t, runner.observedSecret)
	receipt := mustReadOMPReviewFile(t, filepath.Join(root, OMPModelReceiptRelativePath))
	assert.NotContains(t, string(receipt), sentinel)
	assert.NotContains(t, string(receipt), OMPModelSHA256(mustReadOMPReviewFile(t, path)))
}

func TestOMPGenerate_TransactionRollsBackLateWriteAndConfigReadFailure(t *testing.T) {
	t.Parallel()

	t.Run("late write", func(t *testing.T) {
		root := t.TempDir()
		blocker := filepath.Join(root, ".omp", "skills")
		require.NoError(t, os.MkdirAll(filepath.Dir(blocker), 0o755))
		require.NoError(t, os.WriteFile(blocker, []byte("user blocker\n"), 0o600))
		_, err := NewWithRoot(root).Generate(context.Background(), configForOMP())
		require.Error(t, err)
		assert.Equal(t, []byte("user blocker\n"), mustReadOMPReviewFile(t, blocker))
		assert.NoDirExists(t, filepath.Join(root, ".omp", "rules"))
		assert.NoDirExists(t, filepath.Join(root, ".omp", "commands"))
		assert.NoFileExists(t, filepath.Join(root, ".autopus", "omp-manifest.json"))
	})

	t.Run("config read error", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, configFile)
		require.NoError(t, os.MkdirAll(path, 0o700))
		_, err := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner()).
			Generate(context.Background(), integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))
		require.ErrorContains(t, err, "config.yml")
		assert.DirExists(t, path)
		assert.NoDirExists(t, filepath.Join(root, ".omp", "rules"))
		assert.NoFileExists(t, filepath.Join(root, ".autopus", "omp-manifest.json"))
	})
}

type rejectSecretConfigRunner struct {
	*modelIntegrationFakeRunner
	sentinel       string
	observedSecret bool
}

func (runner *rejectSecretConfigRunner) Run(ctx context.Context, executable string, args ...string) ([]byte, error) {
	if len(args) > 1 && args[0] == "--config" {
		data, err := os.ReadFile(args[1])
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(data), runner.sentinel) {
			runner.observedSecret = true
			return nil, fmt.Errorf("secret-bearing config reached probe")
		}
	}
	return runner.modelIntegrationFakeRunner.Run(ctx, executable, args...)
}

func projectManagedSafetyProfile(cfg *config.HarnessConfig) *config.HarnessConfig {
	copy := *cfg
	copy.RoleModelPolicy = cfg.RoleModelPolicy
	copy.RoleModelPolicy.Profiles = make(map[string]config.RoleModelProfileConf, len(cfg.RoleModelPolicy.Profiles))
	for name, value := range cfg.RoleModelPolicy.Profiles {
		copy.RoleModelPolicy.Profiles[name] = value
	}
	profile := copy.RoleModelPolicy.Profiles["p1"]
	profile.Safety.ApprovalMode = "write"
	profile.Safety.IsolationMode = "auto"
	profile.ManagedKeys = make(map[string]config.RoleManagedKeyClaimConf, len(profile.ManagedKeys)+2)
	for path, claim := range cfg.RoleModelPolicy.Profiles["p1"].ManagedKeys {
		profile.ManagedKeys[path] = claim
	}
	missing := OMPMissingManagedValueFingerprint()
	profile.ManagedKeys["tools.approvalMode"] = config.RoleManagedKeyClaimConf{PriorFingerprint: missing, Complete: true}
	profile.ManagedKeys["task.isolation.mode"] = config.RoleManagedKeyClaimConf{PriorFingerprint: missing, Complete: true}
	copy.RoleModelPolicy.Profiles["p1"] = profile
	return &copy
}

func mustReadOMPReviewFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
