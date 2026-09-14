package controlplane

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsignable is a value json.Marshal always rejects, used to exercise the
// canonical-payload marshal failure paths.
type unsignable struct{}

func (unsignable) MarshalJSON() ([]byte, error) {
	return nil, assert.AnError
}

// With a secret configured, a policy signature is required and only the
// matching HMAC is accepted. Guards against an empty or forged signature
// passing validation.
func TestValidateSecurityPolicySignature_SecretConfigured(t *testing.T) {
	t.Setenv(PolicySigningSecretEnv, "top-secret")
	policy := map[string]any{"allow": []string{"read"}}

	require.EqualError(t, ValidateSecurityPolicySignature("task-1", policy, "   "), "missing policy signature")

	signature, err := SignSecurityPolicy("task-1", policy, "top-secret")
	require.NoError(t, err)
	require.NoError(t, ValidateSecurityPolicySignature("task-1", policy, " "+signature+" "))

	// A signature bound to a different task ID must not validate.
	otherSignature, err := SignSecurityPolicy("task-2", policy, "top-secret")
	require.NoError(t, err)
	require.EqualError(t, ValidateSecurityPolicySignature("task-1", policy, otherSignature), "policy signature mismatch")
}

// A policy that cannot be canonicalized must fail signing instead of
// producing an HMAC over a partial payload.
func TestSignSecurityPolicy_UnmarshalablePolicy(t *testing.T) {
	t.Parallel()

	_, err := SignSecurityPolicy("task-1", unsignable{}, "secret")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal canonical policy payload")
}

// Control-plane validation precedence: metadata-free unsigned requests are
// accepted, capabilities are demanded before the signature, and mismatched
// signatures are rejected.
func TestValidateControlPlaneSignature_Precedence(t *testing.T) {
	t.Setenv(PolicySigningSecretEnv, "cp-secret")
	budget := map[string]any{"limit": 3}

	// No metadata, no capabilities, no signature -> nothing to verify.
	require.NoError(t, ValidateControlPlaneSignature("t1", "", nil, nil, nil, nil, nil, ""))

	// Metadata present but capabilities missing -> capabilities error wins.
	err := ValidateControlPlaneSignature("t1", "gpt-x", nil, nil, nil, nil, nil, "abc")
	require.EqualError(t, err, "missing control plane capabilities")

	// Capabilities present, signature missing.
	err = ValidateControlPlaneSignature("t1", "", nil, nil, nil, budget, []string{"code"}, "  ")
	require.EqualError(t, err, "missing control plane signature")

	signature, err := SignControlPlane("t1", "gpt-x", []string{"plan"},
		map[string]string{"plan": "do"}, map[string]string{"plan": "tpl"}, budget, []string{"code"}, "cp-secret")
	require.NoError(t, err)
	require.NoError(t, ValidateControlPlaneSignature("t1", "gpt-x", []string{"plan"},
		map[string]string{"plan": "do"}, map[string]string{"plan": "tpl"}, budget, []string{"code"}, signature))

	// Any covered field change invalidates the signature.
	err = ValidateControlPlaneSignature("t1", "gpt-y", []string{"plan"},
		map[string]string{"plan": "do"}, map[string]string{"plan": "tpl"}, budget, []string{"code"}, signature)
	require.EqualError(t, err, "control plane signature mismatch")
}

// Signing must fail rather than silently dropping an uncanonicalizable
// iteration budget from the signed payload.
func TestSignControlPlane_UnmarshalableBudget(t *testing.T) {
	t.Parallel()

	_, err := SignControlPlane("t1", "m", nil, nil, nil, unsignable{}, []string{"code"}, "secret")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal canonical control plane payload")
}

// hasControlPlaneMetadata treats blank model plus empty collections as "no
// metadata" so unsigned metadata-free requests are not forced through
// verification, and any single populated field flips it.
func TestHasControlPlaneMetadata(t *testing.T) {
	t.Parallel()

	assert.False(t, hasControlPlaneMetadata("  ", nil, nil, nil, nil))
	assert.False(t, hasControlPlaneMetadata("", []string{}, map[string]string{}, map[string]string{}, nil))
	assert.True(t, hasControlPlaneMetadata(" m ", nil, nil, nil, nil))
	assert.True(t, hasControlPlaneMetadata("", []string{"plan"}, nil, nil, nil))
	assert.True(t, hasControlPlaneMetadata("", nil, map[string]string{"plan": "x"}, nil, nil))
	assert.True(t, hasControlPlaneMetadata("", nil, nil, map[string]string{"plan": "x"}, nil))
	assert.True(t, hasControlPlaneMetadata("", nil, nil, nil, map[string]any{"limit": 1}))
}

// hasIterationBudget counts only a positive limit; nil, typed-nil, zero,
// negative, and unmarshalable budgets must not count as metadata.
func TestHasIterationBudget(t *testing.T) {
	t.Parallel()

	assert.False(t, hasIterationBudget(nil))
	assert.False(t, hasIterationBudget((*struct{ Limit int })(nil)))
	assert.False(t, hasIterationBudget(map[string]any{"limit": 0}))
	assert.False(t, hasIterationBudget(map[string]any{"limit": -2}))
	assert.False(t, hasIterationBudget(unsignable{}))
	assert.False(t, hasIterationBudget("not-an-object"))
	assert.True(t, hasIterationBudget(map[string]any{"limit": 1}))
	assert.True(t, hasIterationBudget(struct {
		Limit int `json:"limit"`
	}{Limit: 5}))
}

// The cached-policy round trip derives the task ID from the filename and
// rejects filenames outside the autopus-policy-{task}.json contract.
func TestVerifyCachedPolicyFile_RoundTripAndFilenameContract(t *testing.T) {
	t.Setenv(PolicySigningSecretEnv, "cache-secret")
	dir := t.TempDir()
	policy := map[string]any{"allow": []string{"write"}}

	policyPath := filepath.Join(dir, "autopus-policy-task-9.json")
	signature, err := SignSecurityPolicy("task-9", policy, "cache-secret")
	require.NoError(t, err)
	require.NoError(t, WritePolicySignature(policyPath, signature))
	require.NoError(t, VerifyCachedPolicyFile(policyPath, policy))

	// Tampered policy content no longer matches the cached signature.
	require.EqualError(t, VerifyCachedPolicyFile(policyPath, map[string]any{"allow": []string{"read"}}),
		"policy signature mismatch")

	badName := filepath.Join(dir, "policy-task-9.txt")
	err = VerifyCachedPolicyFile(badName, policy)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected policy filename")

	// Missing sidecar signature is an error, not an implicit pass.
	err = VerifyCachedPolicyFile(filepath.Join(dir, "autopus-policy-task-absent.json"), policy)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read policy signature")
}

// WritePolicySignature skips blank signatures (no sidecar created) and
// surfaces write failures instead of reporting success.
func TestWritePolicySignature_BlankAndFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyPath := filepath.Join(dir, "autopus-policy-task-blank.json")

	require.NoError(t, WritePolicySignature(policyPath, "   "))
	_, statErr := os.Stat(PolicySignaturePath(policyPath))
	assert.True(t, os.IsNotExist(statErr))

	err := WritePolicySignature(filepath.Join(dir, "missing-dir", "autopus-policy-x.json"), "sig")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write policy signature")
}
