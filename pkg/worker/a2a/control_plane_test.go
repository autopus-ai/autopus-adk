package a2a

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/worker/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCapabilities(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{
		CapabilityServerModelV1,
		CapabilityPipelinePhasesV1,
		CapabilityPipelineInstructionsV1,
		CapabilityPipelinePromptTemplatesV1,
		CapabilityIterationBudgetV1,
		CapabilitySignedPolicyV1,
		CapabilitySignedControlPlaneV1,
		CapabilityRedlineInstructionsV1,
	}, DefaultCapabilities())
	assert.Contains(t, DefaultCapabilities(), "redline_instructions.v1")
}

func TestSignAndVerifySecurityPolicySignature(t *testing.T) {
	t.Parallel()

	policy := SecurityPolicy{AllowNetwork: true, AllowFS: true, TimeoutSec: 60}
	signature, err := signSecurityPolicy("task-1", policy, "secret")
	require.NoError(t, err)
	require.NotEmpty(t, signature)

	require.NoError(t, verifySecurityPolicySignature("task-1", policy, signature, "secret"))
	assert.Error(t, verifySecurityPolicySignature("task-1", policy, signature, "wrong-secret"))
}

func TestCacheSecurityPolicy_WritesSignatureSidecar(t *testing.T) {
	t.Parallel()

	taskID := "signed-policy-task"
	policy := SecurityPolicy{AllowNetwork: false, AllowFS: true, TimeoutSec: 30}
	signature, err := signSecurityPolicy(taskID, policy, "secret")
	require.NoError(t, err)

	require.NoError(t, cacheSecurityPolicy(taskID, policy, signature))

	dir, err := policyDir()
	require.NoError(t, err)
	policyPath := filepath.Join(dir, "autopus-policy-"+taskID+".json")
	defer func() { _ = os.Remove(policyPath) }()
	defer func() { _ = os.Remove(policySignaturePath(policyPath)) }()

	gotSignature, err := readPolicySignature(policyPath)
	require.NoError(t, err)
	assert.Equal(t, signature, gotSignature)
}

func TestVerifyCachedPolicyFile(t *testing.T) {
	t.Setenv(PolicySigningSecretEnv, "secret")

	taskID := "verify-policy-task"
	policy := security.SecurityPolicy{
		AllowNetwork:    false,
		AllowFS:         true,
		AllowedCommands: []string{"go test"},
		TimeoutSec:      30,
	}
	signature, err := signSecurityPolicy(taskID, policy, "secret")
	require.NoError(t, err)

	dir, err := policyDir()
	require.NoError(t, err)
	policyPath := filepath.Join(dir, "autopus-policy-"+taskID+".json")
	data, err := json.MarshalIndent(policy, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(policyPath, data, 0o600))
	require.NoError(t, writePolicySignature(policyPath, signature))
	defer func() { _ = os.Remove(policyPath) }()
	defer func() { _ = os.Remove(policySignaturePath(policyPath)) }()

	require.NoError(t, VerifyCachedPolicyFile(policyPath, policy))
}

func TestSignAndVerifyControlPlaneSignature(t *testing.T) {
	t.Parallel()

	signature, err := signControlPlane(
		"task-1",
		"gpt-5.4",
		[]string{"planner", "reviewer"},
		map[string]string{"planner": "Plan carefully."},
		map[string]string{"planner": "SERVER TEMPLATE\n\n{{input}}"},
		&IterationBudget{Limit: 12, WarnThreshold: 0.7, DangerThreshold: 0.9},
		[]string{CapabilityServerModelV1, CapabilityPipelinePhasesV1, CapabilityPipelineInstructionsV1},
		"secret",
	)
	require.NoError(t, err)
	require.NotEmpty(t, signature)

	require.NoError(t, verifyControlPlaneSignature(
		"task-1",
		"gpt-5.4",
		[]string{"planner", "reviewer"},
		map[string]string{"planner": "Plan carefully."},
		map[string]string{"planner": "SERVER TEMPLATE\n\n{{input}}"},
		&IterationBudget{Limit: 12, WarnThreshold: 0.7, DangerThreshold: 0.9},
		[]string{CapabilityServerModelV1, CapabilityPipelinePhasesV1, CapabilityPipelineInstructionsV1},
		signature,
		"secret",
	))
	assert.Error(t, verifyControlPlaneSignature(
		"task-1",
		"gpt-5.4",
		[]string{"planner"},
		map[string]string{"planner": "Plan carefully."},
		map[string]string{"planner": "SERVER TEMPLATE\n\n{{input}}"},
		&IterationBudget{Limit: 12, WarnThreshold: 0.7, DangerThreshold: 0.9},
		[]string{CapabilityServerModelV1, CapabilityPipelinePhasesV1},
		signature,
		"secret",
	))
}

func TestApplyControlPlaneCapabilities(t *testing.T) {
	t.Parallel()

	model, phases, instructions, promptTemplates, iterationBudget := applyControlPlaneCapabilities(
		"gpt-5.4",
		[]string{"planner", "reviewer"},
		map[string]string{"planner": "Plan carefully."},
		map[string]string{"planner": "SERVER TEMPLATE\n\n{{input}}"},
		&IterationBudget{Limit: 9, WarnThreshold: 0.7, DangerThreshold: 0.9},
		[]string{CapabilityServerModelV1, CapabilityPipelineInstructionsV1},
	)

	assert.Equal(t, "gpt-5.4", model)
	assert.Nil(t, phases)
	assert.Equal(t, map[string]string{"planner": "Plan carefully."}, instructions)
	assert.Nil(t, promptTemplates)
	assert.Nil(t, iterationBudget)
}

func TestApplyControlPlaneCapabilities_PreservesPromptTemplatesWhenAuthorized(t *testing.T) {
	t.Parallel()

	_, _, _, promptTemplates, _ := applyControlPlaneCapabilities(
		"",
		nil,
		nil,
		map[string]string{"planner": "SERVER TEMPLATE\n\n{{input}}"},
		nil,
		[]string{CapabilityPipelinePromptTemplatesV1},
	)

	assert.Equal(t, map[string]string{"planner": "SERVER TEMPLATE\n\n{{input}}"}, promptTemplates)
}
