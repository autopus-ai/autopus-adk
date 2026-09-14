package config

import (
	"strings"
	"testing"
)

const missingManagedFingerprintFixture = "sha256:6ce4ae8701413d8bd479df412f102783e1cd3d21e85e6ae91088fe1ee12aa75e"

func TestOMPMissingManagedValueFingerprint_IsStable(t *testing.T) {
	t.Parallel()

	if got := OMPMissingManagedValueFingerprint(); got != missingManagedFingerprintFixture {
		t.Fatalf("missing managed fingerprint = %q", got)
	}
}

func TestRoleModelPolicy_ProjectManaged_RequiresExplicitCompleteClaims(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		claims map[string]RoleManagedKeyClaimConf
		code   string
	}{
		{"missing", nil, "managed_key_claim_required"},
		{"unknown", map[string]RoleManagedKeyClaimConf{"unknown.key": validManagedClaim(false)}, "managed_key_claim_invalid"},
		{"incomplete", map[string]RoleManagedKeyClaimConf{OMPNativeAgentModelOverridesKey: {PriorFingerprint: missingManagedFingerprintFixture}}, "managed_key_claim_invalid"},
		{"invalid fingerprint", map[string]RoleManagedKeyClaimConf{OMPNativeAgentModelOverridesKey: {PriorFingerprint: "sha256:bad", Complete: true}}, "managed_key_claim_invalid"},
		{"array incomplete", map[string]RoleManagedKeyClaimConf{"retry.fallbackChains": validManagedClaim(false)}, "array_ownership_required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := validRoleModelPolicyFixture()
			profile := policy.Profiles["p1"]
			profile.ConfigMode = RoleModelConfigModeProjectManaged
			profile.ManagedKeys = tt.claims
			policy.Profiles["p1"] = profile
			if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), tt.code) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.code)
			}
		})
	}
}

func TestRoleModelPolicy_ProjectManaged_AcceptsExactClaims(t *testing.T) {
	t.Parallel()

	policy := validRoleModelPolicyFixture()
	profile := policy.Profiles["p1"]
	profile.ConfigMode = RoleModelConfigModeProjectManaged
	profile.ManagedKeys = map[string]RoleManagedKeyClaimConf{
		OMPNativeAgentModelOverridesKey: validManagedClaim(false),
		"retry.fallbackChains":          validManagedClaim(true),
		"retry.modelFallback":           validManagedClaim(false),
	}
	policy.Profiles["p1"] = profile
	if err := policy.Validate(); err != nil {
		t.Fatalf("valid managed claims rejected: %v", err)
	}
}

func validManagedClaim(array bool) RoleManagedKeyClaimConf {
	return RoleManagedKeyClaimConf{
		PriorFingerprint: missingManagedFingerprintFixture,
		Complete:         true, FullArrayOwnership: array,
	}
}
