package omp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckOMPModelRoutingDoctor_NoOptInReturnsNoChecks(t *testing.T) {
	t.Parallel()

	report := CheckOMPModelRoutingDoctor(OMPModelDoctorInput{WorkspaceRoot: t.TempDir()})
	assert.False(t, report.Enabled)
	assert.Empty(t, report.Status)
	assert.Empty(t, report.Roles)
}

func TestCheckOMPModelRoutingDoctor_FreshReceiptReturnsSafeRoleRows(t *testing.T) {
	t.Parallel()

	input := writeOMPModelDoctorFixture(t)
	report := CheckOMPModelRoutingDoctor(input)
	require.True(t, report.Enabled)
	assert.Equal(t, "supported", report.Status)
	assert.Equal(t, "fresh", report.Reason)
	require.Len(t, report.Roles, 2)
	assert.Equal(t, OMPModelDoctorRoleRow{
		Agent: "reviewer", Role: "autopus_reviewer", Capability: "independent_dissent",
		Status: "supported", Reason: "selected", FamilyDiversity: "satisfied",
		FamilyReason:  "not_applicable",
		EvidenceClass: "availability", QuorumEvidence: false,
	}, report.Roles[0])
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	for _, forbidden := range []string{"p/code", "q/review", input.WorkspaceRoot, "provider"} {
		assert.NotContains(t, string(encoded), forbidden)
	}
}

func TestCheckOMPModelRoutingDoctor_StaleEvidenceReturnsExactReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
		edit func(*OMPModelDoctorInput)
	}{
		{"version", "version_stale", func(input *OMPModelDoctorInput) { input.Probe.Version = "omp/17.1.9" }},
		{"catalog", "catalog_stale", func(input *OMPModelDoctorInput) {
			input.Probe.Catalog.Fingerprint = doctorHash("d")
		}},
		{"profile", "projection_mismatch", func(input *OMPModelDoctorInput) { input.Profile = "quality" }},
		{"config hash", "projection_mismatch", func(input *OMPModelDoctorInput) {
			input.Activation.ConfigHash = doctorHash("e")
		}},
		{"readback hash", "projection_mismatch", func(input *OMPModelDoctorInput) {
			input.Activation.ReadbackHash = doctorHash("f")
		}},
		{"role tuple", "projection_mismatch", func(input *OMPModelDoctorInput) {
			input.Compilation.Resolutions[0].Thinking = "xhigh"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := writeOMPModelDoctorFixture(t)
			tc.edit(&input)
			report := CheckOMPModelRoutingDoctor(input)
			assert.Equal(t, "blocked", report.Status)
			assert.Equal(t, tc.want, report.Reason)
		})
	}
}

func TestCheckOMPModelRoutingDoctor_MissingInvalidAndMetadataGapFailClosed(t *testing.T) {
	t.Parallel()

	missing := modelDoctorInput(t.TempDir())
	assert.Equal(t, "receipt_missing", CheckOMPModelRoutingDoctor(missing).Reason)

	invalidRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(invalidRoot, ".autopus"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(invalidRoot, OMPModelReceiptRelativePath), []byte("{secret"), 0o600))
	invalid := modelDoctorInput(invalidRoot)
	assert.Equal(t, "receipt_invalid", CheckOMPModelRoutingDoctor(invalid).Reason)

	wrongMode := writeOMPModelDoctorFixture(t)
	require.NoError(t, os.Chmod(filepath.Join(wrongMode.WorkspaceRoot, OMPModelReceiptRelativePath), 0o644))
	assert.Equal(t, "receipt_invalid", CheckOMPModelRoutingDoctor(wrongMode).Reason)

	gap := writeOMPModelDoctorFixture(t)
	gap.Probe.Status = "blocked"
	gap.Probe.Reason = "catalog_metadata_insufficient"
	gap.Probe.Catalog = OMPModelCatalog{}
	report := CheckOMPModelRoutingDoctor(gap)
	assert.Equal(t, "blocked", report.Status)
	assert.Equal(t, "catalog_metadata_insufficient", report.Reason)
}

func TestCheckOMPModelRoutingDoctor_DegradedAndBlockedRolesRemainAvailabilityOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	receipt := modelReceiptFixture(time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC))
	// Keep only the task row: the degraded reviewer route is excluded from
	// the current projection, and reviewer sorts first in the role rows.
	receipt.Roles = receipt.Roles[1:]
	_, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{
		WorkspaceRoot: root,
		Receipt:       receipt,
	})
	require.NoError(t, err)
	input := modelDoctorInput(root)
	input.Compilation.Resolutions[1].Status = "degraded"
	input.Compilation.Resolutions[1].Reason = "explicit_degraded"
	input.Compilation.Resolutions[1].DegradedReason = "explicit_runtime_default"
	report := CheckOMPModelRoutingDoctor(input)
	assert.Equal(t, "degraded", report.Status)
	assert.Equal(t, "role_degraded", report.Reason)
	assert.Equal(t, "degraded", report.Roles[0].Status)
	assert.Equal(t, "availability", report.Roles[0].EvidenceClass)
	assert.False(t, report.Roles[0].QuorumEvidence)

	input.Compilation.Resolutions[1].Status = "blocked"
	input.Compilation.Resolutions[1].Reason = "no_compatible_candidate"
	report = CheckOMPModelRoutingDoctor(input)
	assert.Equal(t, "blocked", report.Status)
	assert.Equal(t, "role_blocked", report.Reason)
}

func writeOMPModelDoctorFixture(t *testing.T) OMPModelDoctorInput {
	t.Helper()
	root := t.TempDir()
	receipt := modelReceiptFixture(time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC))
	_, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{WorkspaceRoot: root, Receipt: receipt})
	require.NoError(t, err)
	return modelDoctorInput(root)
}

func modelDoctorInput(root string) OMPModelDoctorInput {
	return OMPModelDoctorInput{
		Enabled: true, WorkspaceRoot: root, Profile: "balanced", ConfigSource: "overlay",
		Probe: OMPModelCatalogProbeResult{
			Status: "ready", Reason: "catalog_ready", Version: "omp/17.1.8",
			Catalog: OMPModelCatalog{Fingerprint: doctorHash("a")},
		},
		Activation: OMPModelActivationEvidence{ConfigHash: doctorHash("b"), ReadbackHash: doctorHash("c")},
		Compilation: OMPModelRoutingCompilation{ResolutionDigest: doctorHash("9"), Resolutions: []OMPModelRouteResolution{
			{RouteID: "task", Agent: "task", RequestedRole: "autopus_planner", Capability: "deep_reasoning", Status: "selected", Reason: "selected", EffectiveProvider: "p", EffectiveModel: "code", EffectiveSelector: "p/code:medium", Thinking: "medium", EvidenceClass: "availability"},
			{RouteID: "reviewer", Agent: "reviewer", RequestedRole: "autopus_reviewer", Capability: "independent_dissent", Status: "selected", Reason: "selected", EffectiveProvider: "q", EffectiveModel: "review", EffectiveSelector: "q/review:high", Thinking: "high", EvidenceClass: "availability", FamilyDiversity: OMPFamilyDiversity{Status: "satisfied", Executor: "p", Reviewer: "q"}},
		}},
	}
}

func doctorHash(char string) string {
	return "sha256:" + strings.Repeat(char, 64)
}
