package controlplane

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/insajin/autopus-adk/pkg/privatedir"
)

const PolicySigningSecretEnv = "AUTOPUS_A2A_POLICY_SIGNING_SECRET"

// unsignedWarnOnce guards the once-per-process warning emitted when a signature
// verification entry point takes the fail-open (unsigned) path because the
// signing secret is unset AND the operator explicitly opted out via
// AllowUnsignedControlPlaneEnv (see the @AX:ANCHOR below for the
// enforce-by-default contract). The warning only makes the opted-out state
// observable.
var unsignedWarnOnce sync.Once

// warnUnsignedControlPlane emits the unsigned-mode warning exactly once per
// process. It is called right before an entry point returns nil under the
// explicit AllowUnsignedControlPlaneEnv opt-out. Return values and the
// opt-out fail-open policy are unchanged.
func warnUnsignedControlPlane() {
	unsignedWarnOnce.Do(func() {
		log.Printf("[controlplane] %s is unset; control-plane/policy signature verification is disabled (fail-open) because %s is set. Unset it and configure the secret to enforce signed metadata.", PolicySigningSecretEnv, AllowUnsignedControlPlaneEnv)
	})
}

// @AX:ANCHOR [AUTO] signed control-plane enforcement gate; enforce-by-default — unsigned mode is only permitted when AllowUnsignedControlPlaneEnv is explicitly truthy. Keep this contract stable across worker startup, request-intake, diagnostics, worker routing, and prompt fallback paths.
// @AX:REASON: Called by loop_task, pipeline execution, and phase parsing to decide when server-signed metadata must override local defaults; also backs EnforceSignedControlPlane (WorkerLoop.Start) and the fail-closed default in the Validate*/VerifyCachedPolicyFile entry points below.
func SignedControlPlaneEnforced() bool {
	return signingSecret() != ""
}

func ValidateSecurityPolicySignature(taskID string, policy any, signature string) error {
	secret := signingSecret()
	if secret == "" {
		return unsignedResult("ValidateSecurityPolicySignature")
	}
	if strings.TrimSpace(signature) == "" {
		return fmt.Errorf("missing policy signature")
	}
	return VerifySecurityPolicySignature(taskID, policy, signature, secret)
}

func SignSecurityPolicy(taskID string, policy any, secret string) (string, error) {
	payload, err := canonicalSecurityPolicyPayload(taskID, policy)
	if err != nil {
		return "", err
	}
	return signPayload(payload, secret, "policy")
}

func VerifySecurityPolicySignature(taskID string, policy any, signature, secret string) error {
	expected, err := SignSecurityPolicy(taskID, policy, secret)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return fmt.Errorf("policy signature mismatch")
	}
	return nil
}

// @AX:NOTE [AUTO] [downgraded from ANCHOR - fan_in < 3] cached policy signature verification boundary; fail-open or filename changes here would weaken local policy trust.
// @AX:REASON: Called from CLI validation and retained A2A compatibility paths, and depends on the autopus-policy-{task}.json naming contract for sidecar signature lookup.
func VerifyCachedPolicyFile(policyPath string, policy any) error {
	secret := signingSecret()
	if secret == "" {
		return unsignedResult("VerifyCachedPolicyFile")
	}
	taskID, err := taskIDFromPolicyPath(policyPath)
	if err != nil {
		return err
	}
	signature, err := ReadPolicySignature(policyPath)
	if err != nil {
		return err
	}
	return VerifySecurityPolicySignature(taskID, policy, signature, secret)
}

func ValidateControlPlaneSignature(taskID, model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget any, capabilities []string, signature string) error {
	secret := signingSecret()
	if secret == "" {
		return unsignedResult("ValidateControlPlaneSignature")
	}
	if !hasControlPlaneMetadata(model, pipelinePhases, pipelineInstructions, pipelinePromptTemplates, iterationBudget) && len(capabilities) == 0 && strings.TrimSpace(signature) == "" {
		return nil
	}
	if len(capabilities) == 0 {
		return fmt.Errorf("missing control plane capabilities")
	}
	if strings.TrimSpace(signature) == "" {
		return fmt.Errorf("missing control plane signature")
	}
	return VerifyControlPlaneSignature(taskID, model, pipelinePhases, pipelineInstructions, pipelinePromptTemplates, iterationBudget, capabilities, signature, secret)
}

func SignControlPlane(taskID, model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget any, capabilities []string, secret string) (string, error) {
	payload, err := canonicalControlPlanePayload(taskID, model, pipelinePhases, pipelineInstructions, pipelinePromptTemplates, iterationBudget, capabilities)
	if err != nil {
		return "", err
	}
	return signPayload(payload, secret, "control plane")
}

func VerifyControlPlaneSignature(taskID, model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget any, capabilities []string, signature, secret string) error {
	expected, err := SignControlPlane(taskID, model, pipelinePhases, pipelineInstructions, pipelinePromptTemplates, iterationBudget, capabilities, secret)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return fmt.Errorf("control plane signature mismatch")
	}
	return nil
}

func PolicySignaturePath(policyPath string) string {
	return policyPath + ".sig"
}

func WritePolicySignature(policyPath, signature string) error {
	if strings.TrimSpace(signature) == "" {
		return nil
	}
	path := PolicySignaturePath(policyPath)
	// The sidecar joins a policy file already written; it never creates the
	// policy directory on its own.
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return fmt.Errorf("write policy signature: %w", err)
	}
	if err := privatedir.WriteFile(filepath.Dir(path), filepath.Base(path), []byte(signature+"\n")); err != nil {
		return fmt.Errorf("write policy signature: %w", err)
	}
	return nil
}

func ReadPolicySignature(policyPath string) (string, error) {
	data, err := os.ReadFile(PolicySignaturePath(policyPath))
	if err != nil {
		return "", fmt.Errorf("read policy signature: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func signingSecret() string {
	return strings.TrimSpace(os.Getenv(PolicySigningSecretEnv))
}

func signPayload(payload []byte, secret, label string) (string, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(payload); err != nil {
		return "", fmt.Errorf("sign %s payload: %w", label, err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func canonicalSecurityPolicyPayload(taskID string, policy any) ([]byte, error) {
	payload := struct {
		TaskID string `json:"task_id"`
		Policy any    `json:"policy"`
	}{
		TaskID: taskID,
		Policy: policy,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical policy payload: %w", err)
	}
	return data, nil
}

func canonicalControlPlanePayload(taskID, model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget any, capabilities []string) ([]byte, error) {
	payload := struct {
		TaskID                  string            `json:"task_id"`
		Model                   string            `json:"model,omitempty"`
		PipelinePhases          []string          `json:"pipeline_phases,omitempty"`
		PipelineInstructions    map[string]string `json:"pipeline_instructions,omitempty"`
		PipelinePromptTemplates map[string]string `json:"pipeline_prompt_templates,omitempty"`
		IterationBudget         any               `json:"iteration_budget,omitempty"`
		Capabilities            []string          `json:"capabilities,omitempty"`
	}{
		TaskID:                  taskID,
		Model:                   strings.TrimSpace(model),
		PipelinePhases:          append([]string(nil), pipelinePhases...),
		PipelineInstructions:    cloneStringMap(pipelineInstructions),
		PipelinePromptTemplates: cloneStringMap(pipelinePromptTemplates),
		IterationBudget:         iterationBudget,
		Capabilities:            append([]string(nil), capabilities...),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical control plane payload: %w", err)
	}
	return data, nil
}

func hasControlPlaneMetadata(model string, pipelinePhases []string, pipelineInstructions map[string]string, pipelinePromptTemplates map[string]string, iterationBudget any) bool {
	return strings.TrimSpace(model) != "" || len(pipelinePhases) > 0 || len(pipelineInstructions) > 0 || len(pipelinePromptTemplates) > 0 || hasIterationBudget(iterationBudget)
}

func hasIterationBudget(iterationBudget any) bool {
	if iterationBudget == nil {
		return false
	}
	data, err := json.Marshal(iterationBudget)
	if err != nil || string(data) == "null" {
		return false
	}
	var payload struct {
		Limit int `json:"limit"`
	}
	return json.Unmarshal(data, &payload) == nil && payload.Limit > 0
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func taskIDFromPolicyPath(policyPath string) (string, error) {
	base := filepath.Base(policyPath)
	const prefix = "autopus-policy-"
	const suffix = ".json"
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, suffix) {
		return "", fmt.Errorf("unexpected policy filename: %s", base)
	}
	return strings.TrimSuffix(strings.TrimPrefix(base, prefix), suffix), nil
}
