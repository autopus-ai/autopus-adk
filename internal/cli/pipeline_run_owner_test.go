package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/execplane"
	"github.com/insajin/autopus-adk/pkg/orcarun"
)

func TestPipelineRunCmd_ExecutionOwnerAcceptsOnlyExactSingleValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "default", want: pipelineExecutionOwnerOMP},
		{name: "explicit omp", values: []string{"omp"}, want: pipelineExecutionOwnerOMP},
		{name: "explicit orca", values: []string{"orca"}, want: pipelineExecutionOwnerOrca},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &pipelineRunConfig{}
			cmd := newPipelineRunCmdWithConfig(cfg)
			args := []string{"SPEC-OWNER-001", "--platform", "omp"}
			for _, value := range test.values {
				args = append(args, "--execution-owner", value)
			}

			require.NoError(t, cmd.ParseFlags(args))
			assert.Equal(t, test.want, cfg.ExecutionOwner)
			assert.Equal(t, len(test.values) == 1, cfg.executionOwnerExplicit)
		})
	}

	for _, value := range []string{"OMP", "Orca", "omp,orca", "omp ", "local", "supervised"} {
		t.Run("reject "+value, func(t *testing.T) {
			cmd := newPipelineRunCmdWithConfig(&pipelineRunConfig{})
			err := cmd.ParseFlags([]string{
				"SPEC-OWNER-001", "--platform", "omp", "--execution-owner", value,
			})
			assert.ErrorContains(t, err, "must be exactly omp or orca")
		})
	}

	cmd := newPipelineRunCmdWithConfig(&pipelineRunConfig{})
	err := cmd.ParseFlags([]string{
		"SPEC-OWNER-001", "--platform", "omp",
		"--execution-owner", "omp", "--execution-owner", "orca",
	})
	assert.ErrorContains(t, err, "specified exactly once")
}

func TestPipelineRunCmd_InvalidOrMixedOwnerStopsBeforeFilesystemEffects(t *testing.T) {
	for _, args := range [][]string{
		{"SPEC-OWNER-001", "--platform", "omp", "--execution-owner", "omp,orca"},
		{"SPEC-OWNER-001", "--platform", "omp", "--execution-owner", "omp", "--execution-owner", "orca"},
	} {
		t.Run(args[len(args)-1], func(t *testing.T) {
			root := t.TempDir()
			chdirForTest(t, root)
			cmd := newPipelineRunCmd()
			cmd.SetArgs(args)

			require.Error(t, cmd.Execute())
			_, err := os.Stat(pipelineStateDir)
			assert.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestResolvePipelineExecutionOwner_IsMutuallyExclusiveAndRequiresOMPBoundary(t *testing.T) {
	tests := []struct {
		name    string
		cfg     pipelineRunConfig
		owner   string
		source  string
		wantErr string
	}{
		{
			name: "default omp", cfg: pipelineRunConfig{Platform: "omp"},
			owner: pipelineExecutionOwnerOMP, source: pipelineExecutionOwnerSourceDefault,
		},
		{
			name:  "explicit omp",
			cfg:   pipelineRunConfig{Platform: "omp", ExecutionOwner: "omp", executionOwnerExplicit: true},
			owner: pipelineExecutionOwnerOMP, source: pipelineExecutionOwnerSourceExplicit,
		},
		{
			name:  "explicit orca",
			cfg:   pipelineRunConfig{Platform: "omp", ExecutionOwner: "orca", executionOwnerExplicit: true},
			owner: pipelineExecutionOwnerOrca, source: pipelineExecutionOwnerSourceExplicit,
		},
		{
			name:    "implicit orca rejected",
			cfg:     pipelineRunConfig{Platform: "omp", ExecutionOwner: "orca"},
			wantErr: "must be selected explicitly",
		},
		{
			name:    "non OMP boundary rejected",
			cfg:     pipelineRunConfig{Platform: "codex", ExecutionOwner: "omp", executionOwnerExplicit: true},
			wantErr: "requires exact --platform omp",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := resolvePipelineExecutionOwner(&test.cfg)
			if test.wantErr != "" {
				assert.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.owner, decision.Owner)
			assert.Equal(t, test.source, decision.Source)
			assert.NotEmpty(t, decision.Reason)
			assert.NotEqual(t, decision.Owner == pipelineExecutionOwnerOMP,
				decision.Owner == pipelineExecutionOwnerOrca,
				"exactly one execution owner must be selected")
		})
	}
}

func TestPipelineRun_OrcaOwnerRecordsTheGateThenRefusesToFallBackToOMP(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)
	specID := "SPEC-OWNER-ORCA-001"
	writePipelineOwnerSpec(t, root, specID)
	// PATH carries an omp trap and no orca at all: the run must reach for the
	// supervised backend the operator named and stop there, never for the native
	// OMP one they declined.
	markers := isolatePipelineOwnerPath(t, root, "omp")
	// verification_status is derived from the tier integrity gate instead of
	// being hardcoded, so this test pins the gate's only outside-world seam.
	// Without the stub the expectation below would depend on whichever provider
	// accounts the workstation running the suite happens to have registered.
	stubExecplaneTierProbe(t, verifiedExecplaneTierEvidence)

	var stdout, stderr bytes.Buffer
	cmd := newPipelineRunCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		specID, "--platform", "omp", "--execution-owner", "orca",
	})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorIs(t, err, orcarun.ErrOrcaUnavailable,
		"an absent process plane is an environment failure, not a handoff")
	assert.Contains(t, err.Error(), "--execution-owner omp",
		"a blocked run must name what the operator can do about it")
	assert.NoFileExists(t, markers["omp"],
		"an absent orca must not silently fall back to the execution owner the operator declined")
	assert.NotContains(t, stdout.String(), "handoff_required",
		"REQ-108: handoff_required is no longer a terminal state")
	// REQ-108: the outcome lands on the same blocked-receipt surface every other
	// preflight failure uses, with no phase progress behind it.
	assert.Contains(t, assertPipelineRunBlockedBeforeAnyPhase(t, specID).Blocker, "orca")

	ownerPath := filepath.Join(pipelineStateDir, specID+".execution-owner.json")
	body, readErr := os.ReadFile(ownerPath)
	require.NoError(t, readErr)
	var receipt pipelineExecutionOwnerReceipt
	require.NoError(t, json.Unmarshal(body, &receipt))
	assert.Equal(t, pipelineExecutionOwnerOrca, receipt.Owner)
	assert.Equal(t, pipelineExecutionOwnerSourceExplicit, receipt.Source)
	assert.Equal(t, specID, receipt.SpecID)
	assert.NotEmpty(t, receipt.RunID)
	assert.Equal(t, execplane.StatusVerified, receipt.VerificationStatus,
		"a verified gate is what puts verified in the receipt")
	assertPipelineExecutionOwnerReceiptIsBodyFree(t, body)
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(ownerPath)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	assert.Contains(t, stderr.String(), "Tier integrity "+execplane.StatusVerified,
		"no handoff payload carries the verdict any more, so the run must report it")
}

func TestPipelineRun_OMPOwnerPreservesDryRunForDefaultAndExplicitSelection(t *testing.T) {
	for _, test := range []struct {
		name   string
		flag   []string
		source string
	}{
		{name: "default", source: pipelineExecutionOwnerSourceDefault},
		{name: "explicit", flag: []string{"--execution-owner", "omp"}, source: pipelineExecutionOwnerSourceExplicit},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			chdirForTest(t, root)
			specID := "SPEC-OWNER-OMP-001"
			writePipelineOwnerSpec(t, root, specID)
			orcaMarker := installPipelineOwnerProcessTrap(t, root, "orca")
			var stdout bytes.Buffer
			cmd := newPipelineRunCmd()
			cmd.SetOut(&stdout)
			cmd.SetErr(&bytes.Buffer{})
			args := []string{specID, "--platform", "omp", "--dry-run"}
			args = append(args, test.flag...)
			cmd.SetArgs(args)

			require.NoError(t, cmd.Execute())
			assert.NoFileExists(t, orcaMarker, "OMP ownership must not create an Orca Run")
			var receipt pipelineExecutionOwnerReceipt
			body, err := os.ReadFile(filepath.Join(pipelineStateDir, specID+".execution-owner.json"))
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &receipt))
			assert.Equal(t, pipelineExecutionOwnerOMP, receipt.Owner)
			assert.Equal(t, test.source, receipt.Source)
			assert.Equal(t, specID, receipt.SpecID)
			assert.NotEmpty(t, receipt.RunID)
			assert.False(t, receipt.CheckedAt.IsZero())
			// REQ-008 scopes the tier integrity gate to the orca-owned path, so
			// the OMP-owned path probes nothing — and a receipt that claims
			// "verified" without a check is the silent downgrade
			// SPEC-EXECPLANE-001 exists to prevent.
			assert.Equal(t, execplane.StatusUnverified, receipt.VerificationStatus)
			assert.NoFileExists(t, filepath.Join(pipelineStateDir, specID+".tier-integrity.json"),
				"no gate ran, so there is no integrity receipt to read")
			assertPipelineExecutionOwnerReceiptIsBodyFree(t, body)
		})
	}
}
