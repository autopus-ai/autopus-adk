package a2a

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/worker/controlplane"
)

const (
	PolicySigningSecretEnv              = controlplane.PolicySigningSecretEnv
	AllowUnsignedControlPlaneEnv        = controlplane.AllowUnsignedControlPlaneEnv
	CapabilityServerModelV1             = "server_model_v1"
	CapabilityPipelinePhasesV1          = "pipeline_phases_v1"
	CapabilityPipelineInstructionsV1    = "pipeline_instructions_v1"
	CapabilityPipelinePromptTemplatesV1 = "pipeline_prompt_templates_v1"
	CapabilityIterationBudgetV1         = "iteration_budget_v1"
	CapabilitySignedPolicyV1            = "signed_policy_v1"
	CapabilitySignedControlPlaneV1      = "signed_control_plane_v1"
	// CapabilityRedlineInstructionsV1 tells the backend this worker appends a
	// task's redline_instructions to the prompt it runs, so a reviewer's redline
	// attempt can be routed here. The dotted id matches the desktop worker card.
	CapabilityRedlineInstructionsV1 = "redline_instructions.v1"
)

var defaultCapabilities = []string{
	CapabilityServerModelV1,
	CapabilityPipelinePhasesV1,
	CapabilityPipelineInstructionsV1,
	CapabilityPipelinePromptTemplatesV1,
	CapabilityIterationBudgetV1,
	CapabilitySignedPolicyV1,
	CapabilitySignedControlPlaneV1,
	CapabilityRedlineInstructionsV1,
}

// DefaultCapabilities returns the worker control-plane capabilities advertised at registration.
// @AX:ANCHOR [AUTO] registration capability contract; keep ordering and exported values stable for agent-card generation and control-plane verification.
// @AX:REASON: Used by server registration, agent-card construction, and control-plane tests to assert the worker's advertised feature set.
func DefaultCapabilities() []string {
	return append([]string(nil), defaultCapabilities...)
}

// SignedControlPlaneEnforced returns true when the worker is running in a mode
// where server-issued control-plane metadata must be trusted over local fallback.
func SignedControlPlaneEnforced() bool {
	return controlplane.SignedControlPlaneEnforced()
}

func validateSecurityPolicySignature(taskID string, policy SecurityPolicy, signature string) error {
	return controlplane.ValidateSecurityPolicySignature(taskID, policy, signature)
}

func validateControlPlaneSignature(taskID, model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget *IterationBudget, capabilities []string, signature string) error {
	return controlplane.ValidateControlPlaneSignature(taskID, model, pipelinePhases, pipelineInstructions, pipelinePromptTemplates, iterationBudget, capabilities, signature)
}

func signSecurityPolicy(taskID string, policy any, secret string) (string, error) {
	return controlplane.SignSecurityPolicy(taskID, policy, secret)
}

func verifySecurityPolicySignature(taskID string, policy any, signature, secret string) error {
	return controlplane.VerifySecurityPolicySignature(taskID, policy, signature, secret)
}

func signControlPlane(taskID, model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget *IterationBudget, capabilities []string, secret string) (string, error) {
	return controlplane.SignControlPlane(taskID, model, pipelinePhases, pipelineInstructions, pipelinePromptTemplates, iterationBudget, capabilities, secret)
}

func verifyControlPlaneSignature(taskID, model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget *IterationBudget, capabilities []string, signature, secret string) error {
	return controlplane.VerifyControlPlaneSignature(taskID, model, pipelinePhases, pipelineInstructions, pipelinePromptTemplates, iterationBudget, capabilities, signature, secret)
}

func applyControlPlaneCapabilities(model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget *IterationBudget, capabilities []string) (string, []string, map[string]string, map[string]string, *IterationBudget) {
	if len(capabilities) == 0 {
		return strings.TrimSpace(model), append([]string(nil), pipelinePhases...), cloneStringMap(pipelineInstructions), cloneStringMap(pipelinePromptTemplates), cloneIterationBudget(iterationBudget)
	}

	var filteredModel string
	var filteredPhases []string
	var filteredInstructions map[string]string
	var filteredPromptTemplates map[string]string
	var filteredIterationBudget *IterationBudget

	if hasCapability(capabilities, CapabilityServerModelV1) {
		filteredModel = strings.TrimSpace(model)
	}
	if hasCapability(capabilities, CapabilityPipelinePhasesV1) {
		filteredPhases = append([]string(nil), pipelinePhases...)
	}
	if hasCapability(capabilities, CapabilityPipelineInstructionsV1) {
		filteredInstructions = cloneStringMap(pipelineInstructions)
	}
	if hasCapability(capabilities, CapabilityPipelinePromptTemplatesV1) {
		filteredPromptTemplates = cloneStringMap(pipelinePromptTemplates)
	}
	if hasCapability(capabilities, CapabilityIterationBudgetV1) {
		filteredIterationBudget = cloneIterationBudget(iterationBudget)
	}
	return filteredModel, filteredPhases, filteredInstructions, filteredPromptTemplates, filteredIterationBudget
}

func hasCapability(capabilities []string, target string) bool {
	for _, capability := range capabilities {
		if capability == target {
			return true
		}
	}
	return false
}

func hasIterationBudget(iterationBudget *IterationBudget) bool {
	return iterationBudget != nil && iterationBudget.Limit > 0
}

func cloneIterationBudget(iterationBudget *IterationBudget) *IterationBudget {
	if iterationBudget == nil {
		return nil
	}
	cloned := *iterationBudget
	return &cloned
}

func policySignaturePath(policyPath string) string {
	return controlplane.PolicySignaturePath(policyPath)
}

func writePolicySignature(policyPath, signature string) error {
	return controlplane.WritePolicySignature(policyPath, signature)
}

func readPolicySignature(policyPath string) (string, error) {
	return controlplane.ReadPolicySignature(policyPath)
}

// VerifyCachedPolicyFile verifies the sidecar signature for a cached policy file when
// signature validation is enabled via AUTOPUS_A2A_POLICY_SIGNING_SECRET.
func VerifyCachedPolicyFile(policyPath string, policy any) error {
	return controlplane.VerifyCachedPolicyFile(policyPath, policy)
}
