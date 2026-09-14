package omp

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileOMPModelDoctorActivationExpectation_IsDeterministicAndConcrete(t *testing.T) {
	t.Parallel()

	profile := integrationHarnessConfig(config.RoleModelConfigModeOverlay).RoleModelPolicy.Profiles["p1"]
	catalog, reason := NormalizeOMPModelCatalog(newModelIntegrationRunner().catalog, 16*1024)
	require.Equal(t, "catalog_ready", reason)

	want, err := CompileOMPModelDoctorActivationExpectation(profile, catalog)
	require.NoError(t, err)
	require.NotEmpty(t, want.ConfigBytes)
	assert.Equal(t, OMPModelSHA256(want.ConfigBytes), want.ConfigHash)
	assert.Equal(t, true, want.ExpectedValues["retry.modelFallback"])
	overrides := want.ExpectedValues[config.OMPNativeAgentModelOverridesKey].(map[string]string)
	assert.Equal(t, "anthropic/alpha-reasoner:xhigh", overrides["task"])
	assert.Contains(t, string(want.ConfigBytes), "task: anthropic/alpha-reasoner:xhigh")
	assert.NotContains(t, string(want.ConfigBytes), "autopus_")
	assert.NotContains(t, strings.ToLower(string(want.ConfigBytes)), "api_key")

	for iteration := 0; iteration < 40; iteration++ {
		got, compileErr := CompileOMPModelDoctorActivationExpectation(profile, catalog)
		require.NoError(t, compileErr, "iteration=%d", iteration)
		assert.Equal(t, want.ConfigHash, got.ConfigHash, "iteration=%d", iteration)
		assert.Equal(t, want.ConfigBytes, got.ConfigBytes, "iteration=%d", iteration)
		assert.True(t, reflect.DeepEqual(want.ExpectedValues, got.ExpectedValues), "iteration=%d", iteration)
	}
	names := make([]string, 0, len(overrides))
	for agent := range overrides {
		names = append(names, agent)
	}
	sort.Strings(names)
	wantAgents := append([]string(nil), config.OMPNativeAgentNames()...)
	sort.Strings(wantAgents)
	assert.Equal(t, wantAgents, names)
}

func TestCompileOMPModelDoctorActivationExpectation_FailsClosedOnInvalidInputs(t *testing.T) {
	t.Parallel()

	validProfile := integrationHarnessConfig(config.RoleModelConfigModeOverlay).RoleModelPolicy.Profiles["p1"]
	catalog, reason := NormalizeOMPModelCatalog(newModelIntegrationRunner().catalog, 16*1024)
	require.Equal(t, "catalog_ready", reason)

	tests := []struct {
		name string
		want string
		edit func(*config.RoleModelProfileConf, *OMPModelCatalog)
	}{
		{
			name: "unmapped agent override", want: "agent_role_unmapped",
			edit: func(profile *config.RoleModelProfileConf, _ *OMPModelCatalog) {
				profile.Agents = map[string]config.RoleAgentOverrideConf{"future-agent": {}}
			},
		},
		{
			name: "override tuple mismatch", want: "role_capability_mismatch",
			edit: func(profile *config.RoleModelProfileConf, _ *OMPModelCatalog) {
				profile.Agents = map[string]config.RoleAgentOverrideConf{
					"executor": {Role: config.OMPAgentRoleName("planner")},
				}
			},
		},
		{
			name: "capability omitted", want: "capability_missing",
			edit: func(profile *config.RoleModelProfileConf, _ *OMPModelCatalog) {
				delete(profile.Capabilities, config.CapabilityDeterministicTransform)
			},
		},
		{
			name: "catalog cannot resolve required route", want: "required_route_unresolved",
			edit: func(_ *config.RoleModelProfileConf, catalog *OMPModelCatalog) {
				*catalog = OMPModelCatalog{}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := validProfile
			profile.Capabilities = cloneOMPDoctorCapabilities(validProfile.Capabilities)
			currentCatalog := catalog
			tc.edit(&profile, &currentCatalog)
			got, err := CompileOMPModelDoctorActivationExpectation(profile, currentCatalog)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, got.ConfigHash)
			assert.Empty(t, got.ConfigBytes)
			assert.Empty(t, got.ExpectedValues)
		})
	}
}

func TestOMPModelDoctorReceiptConfigSource_ValidatesOverlayAndProjectOwnership(t *testing.T) {
	t.Parallel()

	missingSource, missingDigest, missingReason := OMPModelDoctorReceiptConfigSource(t.TempDir())
	assert.Empty(t, missingSource)
	assert.Empty(t, missingDigest)
	assert.Equal(t, "receipt_missing", missingReason)

	overlayRoot := t.TempDir()
	writeOMPDoctorReceiptForSource(t, overlayRoot, "overlay", "")
	source, digest, reason := OMPModelDoctorReceiptConfigSource(overlayRoot)
	assert.Equal(t, "overlay", source)
	assert.Empty(t, digest)
	assert.Empty(t, reason)

	projectRoot := t.TempDir()
	ownership, ownershipData, err := newOMPModelProjectOwnership(
		[]byte("user: original\n"), false, []byte("task:\n  agentModelOverrides: {}\n"),
		map[string]string{config.OMPNativeAgentModelOverridesKey: OMPModelSHA256([]byte("{}"))},
	)
	require.NoError(t, err)
	writeOMPDoctorOwnedFile(t, projectRoot, OMPModelProjectOwnershipRelativePath, ownershipData, 0o600)
	writeOMPDoctorReceiptForSource(t, projectRoot, config.RoleModelConfigModeProjectManaged, ownership.LedgerDigest)
	source, digest, reason = OMPModelDoctorReceiptConfigSource(projectRoot)
	assert.Equal(t, config.RoleModelConfigModeProjectManaged, source)
	assert.Equal(t, ownership.LedgerDigest, digest)
	assert.Empty(t, reason)

	writeOMPDoctorReceiptForSource(t, projectRoot, config.RoleModelConfigModeProjectManaged, doctorHash("d"))
	source, digest, reason = OMPModelDoctorReceiptConfigSource(projectRoot)
	assert.Empty(t, source)
	assert.Empty(t, digest)
	assert.Equal(t, "receipt_invalid", reason)
}

func TestRootedOMPModelDoctorReaders_RejectNonCanonicalEvidence(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace, err := openOMPRootedWorkspace(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, workspace.Close()) })

	_, receiptReason := readOMPModelDoctorReceiptAt(workspace)
	assert.Equal(t, "receipt_missing", receiptReason)

	writeOMPDoctorReceiptForSource(t, root, "overlay", "")
	receipt, receiptReason := readOMPModelDoctorReceiptAt(workspace)
	assert.Empty(t, receiptReason)
	assert.Equal(t, "overlay", receipt.ConfigSource)

	receiptPath := filepath.Join(root, OMPModelReceiptRelativePath)
	require.NoError(t, os.Chmod(receiptPath, 0o644))
	_, receiptReason = readOMPModelDoctorReceiptAt(workspace)
	assert.Equal(t, "receipt_invalid", receiptReason)
	require.NoError(t, os.Chmod(receiptPath, 0o600))

	valid, err := os.ReadFile(receiptPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(receiptPath, append(valid, []byte("{}\n")...), 0o600))
	_, receiptReason = readOMPModelDoctorReceiptAt(workspace)
	assert.Equal(t, "receipt_invalid", receiptReason)

	ownership, ownershipData, err := newOMPModelProjectOwnership(
		[]byte("original"), false, []byte("emitted"),
		map[string]string{config.OMPNativeAgentModelOverridesKey: OMPModelSHA256([]byte("value"))},
	)
	require.NoError(t, err)
	writeOMPDoctorOwnedFile(t, root, OMPModelProjectOwnershipRelativePath, ownershipData, 0o600)
	gotOwnership, exists, err := readOMPModelProjectOwnershipAt(workspace)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, ownership.LedgerDigest, gotOwnership.LedgerDigest)

	writeOMPDoctorOwnedFile(t, root, configFile, []byte("emitted"), 0o640)
	configBytes, missing, mode, err := readOMPModelProjectConfigAt(workspace)
	require.NoError(t, err)
	assert.False(t, missing)
	assert.Equal(t, []byte("emitted"), configBytes)
	assert.Equal(t, os.FileMode(0o640), mode)

	current, err := validateCurrentOMPModelProjectConfigAt(workspace, ownership)
	require.NoError(t, err)
	assert.Equal(t, []byte("emitted"), current)
	require.NoError(t, os.WriteFile(filepath.Join(root, configFile), []byte("changed"), 0o640))
	_, err = validateCurrentOMPModelProjectConfigAt(workspace, ownership)
	assert.ErrorContains(t, err, "managed_key_conflict")
}

func cloneOMPDoctorCapabilities(input map[string]config.RoleCapabilityRouteConf) map[string]config.RoleCapabilityRouteConf {
	result := make(map[string]config.RoleCapabilityRouteConf, len(input))
	for key, value := range input {
		value.Candidates = append([]config.RoleModelCandidateConf(nil), value.Candidates...)
		result[key] = value
	}
	return result
}

func writeOMPDoctorReceiptForSource(t *testing.T, root, source, ownershipDigest string) {
	t.Helper()
	receipt := modelReceiptFixture(time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC))
	receipt.ConfigSource = source
	receipt.ProjectOwnershipDigest = ownershipDigest
	for index := range receipt.Roles {
		receipt.Roles[index].ConfigSource = source
	}
	_, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{WorkspaceRoot: root, Receipt: receipt})
	require.NoError(t, err)
}

func writeOMPDoctorOwnedFile(t *testing.T, root, relative string, data []byte, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, mode))
}
