package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ompadapter "github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/pipeline"
)

func TestPipelineOMPBackend_ActiveRuntimeOwnsPrivateSessionDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX launcher")
	}
	config, logPath := pipelineOMPBackendTestConfig(t)
	backend, err := newPipelineOMPBackend(config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Close() })
	_, err = backend.Execute(context.Background(), sealedPipelineOMPRequest(t, config, pipeline.PhasePlan, "PLAN-PHASE-PROMPT", nil))
	require.NoError(t, err)
	starts, _ := pipelineOMPRPCRecordsByKind(readPipelineOMPRPCRecords(t, logPath))
	require.Len(t, starts, 1)
	index := -1
	for i, arg := range starts[0].Args {
		if arg == "--session-dir" {
			index = i
		}
	}
	require.GreaterOrEqual(t, index, 0)
	require.Less(t, index+1, len(starts[0].Args))
	sessionDir := starts[0].Args[index+1]
	relative, err := filepath.Rel(config.RuntimeBase, sessionDir)
	require.NoError(t, err)
	assert.False(t, relative == "." || strings.HasPrefix(relative, ".."))
	assert.Equal(t, "sessions", filepath.Base(sessionDir))
	for _, path := range []string{filepath.Dir(sessionDir), sessionDir} {
		info, statErr := os.Lstat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestLoadPipelineOMPPhaseModels_RequiresExactInstalledVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX launcher")
	}
	projectDir := t.TempDir()
	writePipelineOMPModelReceipt(t, projectDir, "omp/17.1.8")

	models, err := loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.8"))
	require.NoError(t, err)
	assert.Equal(t, map[pipeline.PhaseID]string{
		pipeline.PhasePlan: "provider/task", pipeline.PhaseTestScaffold: "provider/task",
		pipeline.PhaseImplement: "provider/task", pipeline.PhaseValidate: "provider/task",
		pipeline.PhaseReview: "provider/reviewer",
	}, models)

	models, err = loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.9"))
	require.ErrorContains(t, err, "does not match installed executable")
	assert.Nil(t, models)
}

func TestLoadPipelineOMPPhaseModels_RejectsExecutableReplacedByVersionProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable replacement uses a POSIX script")
	}
	projectDir := t.TempDir()
	executable := filepath.Join(t.TempDir(), "omp")
	replacement := executable + ".next"
	require.NoError(t, os.WriteFile(replacement, []byte("#!/bin/sh\nprintf 'omp/99.0.0\\n'\n"), 0o700))
	script := "#!/bin/sh\nmv " + shellQuotePipelineOMP(replacement) + " " + shellQuotePipelineOMP(executable) + "\nprintf 'omp/17.1.8\\n'\n"
	require.NoError(t, os.WriteFile(executable, []byte(script), 0o700))
	canonical, identity, err := canonicalPipelineOMPExecutable(executable)
	require.NoError(t, err)

	environment, envErr := normalizePipelineOMPEnvironment(os.Environ())
	require.NoError(t, envErr)
	models, err := loadPipelineOMPPhaseModelsWithAuthority(projectDir, canonical, identity, environment)

	require.ErrorContains(t, err, "identity changed")
	assert.Nil(t, models)
}

func TestLoadPipelineOMPPhaseModels_StripsPoisonedProbeEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX script")
	}
	projectDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "poisoned-probe")
	executable := filepath.Join(t.TempDir(), "omp")
	script := "#!/bin/sh\nif [ -n \"$NODE_OPTIONS\" ]; then printf poison > " +
		shellQuotePipelineOMP(marker) + "; fi\nprintf 'omp/17.1.8\\n'\n"
	require.NoError(t, os.WriteFile(executable, []byte(script), 0o700))
	canonical, identity, err := canonicalPipelineOMPExecutable(executable)
	require.NoError(t, err)

	models, err := loadPipelineOMPPhaseModelsWithAuthority(projectDir, canonical, identity, []string{
		"PATH=/usr/bin:/bin", "NODE_OPTIONS=--require=/tmp/evil.js",
	})

	require.NoError(t, err)
	assert.Empty(t, models)
	_, statErr := os.Stat(marker)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestPipelineOMPBackend_NormalizesEnvironmentAndRejectsExecutableReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable identity uses POSIX permissions")
	}
	config, _ := pipelineOMPBackendTestConfig(t)
	config.Environment = []string{
		"PATH=/usr/bin", "NODE_OPTIONS=--require=/tmp/evil.js", "DYLD_INSERT_LIBRARIES=/tmp/evil.dylib",
		"AUTOPUS_OMP_MANAGED_INNER=0", "PROVIDER_API_KEY=retained",
	}
	normalized, err := normalizePipelineOMPBackendConfig(config)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"PATH=/usr/bin", "PROVIDER_API_KEY=retained", "AUTOPUS_OMP_MANAGED_INNER=1",
	}, normalized.Environment)

	replacement := normalized.Executable + ".replacement"
	require.NoError(t, os.WriteFile(replacement, []byte("#!/bin/sh\nexit 1\n"), 0o700))
	require.NoError(t, os.Rename(replacement, normalized.Executable))
	_, err = startPipelineOMPProcess(context.Background(), normalized)
	require.ErrorContains(t, err, "identity changed")
	assertPipelineOMPRuntimeEmpty(t, normalized.RuntimeBase)
}

func TestPipelineOMPBackend_CanonicalProcessStripsOnlyActiveBrokerAuthority(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX launcher")
	}
	config, logPath := pipelineOMPBackendTestConfig(t)
	brokerMarker := filepath.Join(t.TempDir(), "canonical-broker-authority")
	providerMarker := filepath.Join(t.TempDir(), "canonical-provider-env-missing")
	script := "#!/bin/sh\nif [ -n \"$" + pipelineOMPActiveEndpointKey + "\" ] || [ -n \"$" +
		pipelineOMPActiveCredentialKey + "\" ]; then printf leak > " + shellQuotePipelineOMP(brokerMarker) +
		"; fi\nif [ \"$GENERAL_PROVIDER_TOKEN\" != retained ]; then printf missing > " +
		shellQuotePipelineOMP(providerMarker) + "; fi\nexec " + shellQuotePipelineOMP(os.Args[0]) + " -test.run=^$ -- \"$@\"\n"
	require.NoError(t, os.WriteFile(config.Executable, []byte(script), 0o700))
	config.Environment = append(pipelineOMPCanonicalEnvironment(config.Environment),
		pipelineOMPActiveEndpointKey+"=http://127.0.0.1:43123",
		pipelineOMPActiveCredentialKey+"=must-not-reach-canonical",
		"GENERAL_PROVIDER_TOKEN=retained",
	)
	backend, err := newPipelineOMPBackend(config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Close() })

	_, err = backend.Execute(
		context.Background(), sealedPipelineOMPRequest(t, config, pipeline.PhasePlan, "PLAN-PHASE-PROMPT", nil),
	)
	require.NoError(t, err)
	_, brokerErr := os.Lstat(brokerMarker)
	assert.ErrorIs(t, brokerErr, os.ErrNotExist)
	_, providerErr := os.Lstat(providerMarker)
	assert.ErrorIs(t, providerErr, os.ErrNotExist)
	assert.FileExists(t, logPath)
}

func TestPipelineOMPBackend_CanceledReadinessCleansOwnedRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX script")
	}
	config, _ := pipelineOMPBackendTestConfig(t)
	require.NoError(t, os.WriteFile(config.Executable, []byte("#!/bin/sh\nsleep 10\n"), 0o700))
	normalized, err := normalizePipelineOMPBackendConfig(config)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err = startPipelineOMPProcess(ctx, normalized)
	require.ErrorContains(t, err, "readiness")
	assert.Less(t, time.Since(started), 2*time.Second)
	assertPipelineOMPRuntimeEmpty(t, normalized.RuntimeBase)
}

func writePipelineOMPVersionFixture(t *testing.T, version string) string {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "omp")
	require.NoError(t, os.WriteFile(executable, []byte("#!/bin/sh\nprintf '"+version+"\\n'\n"), 0o700))
	return executable
}

func writePipelineOMPModelReceipt(t *testing.T, root, version string) {
	t.Helper()
	overlay := []byte("models:\n  pipeline: provider/model\n")
	overlayPath := filepath.Join(root, filepath.FromSlash(ompadapter.DefaultOMPModelOverlayPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(overlayPath), 0o700))
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))
	// The receipt is keyed by the agents OMP actually registers.
	agents := []string{"scout", "reviewer", "security-reviewer", "task", "sonic"}
	roles := make([]ompadapter.OMPModelRoleReceipt, 0, len(agents))
	for _, agent := range agents {
		roles = append(roles, ompadapter.OMPModelRoleReceipt{
			Agent: agent, Profile: "balanced", ConfigSource: "overlay",
			RequestedRole: "autopus_planner", EffectiveRole: "autopus_planner",
			Capability: "pipeline", Provider: "provider", Model: agent,
			Selector: "provider/" + agent, Thinking: "medium",
			FamilyDiversity: ompadapter.OMPModelFamilyDiversityReceipt{Status: "not_applicable"},
			SafetySource:    "autopus_profile",
		})
	}
	_, err := ompadapter.WriteOMPModelResolutionReceipt(ompadapter.OMPModelReceiptWriteInput{
		WorkspaceRoot: root,
		Receipt: ompadapter.OMPModelResolutionReceipt{
			OMPVersion: version, CatalogFingerprint: "sha256:" + strings.Repeat("a", 64),
			Profile: "balanced", ConfigSource: "overlay", GeneratedAt: time.Now().UTC(), Roles: roles,
			Activation: ompadapter.OMPModelActivationReceipt{
				Argv: []string{"omp"}, ConfigHash: ompadapter.OMPModelSHA256(overlay),
				ReadbackHash: "sha256:" + strings.Repeat("c", 64),
			},
			Safety: ompadapter.OMPModelSafetyReceipt{ApprovalMode: "write", IsolationMode: "auto", Source: "autopus_profile"},
		},
	})
	require.NoError(t, err)
}
