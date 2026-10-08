package evalregression

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// The ADK harness-eval signing lane (SPEC-HARNEVAL-003 REQ-HR-03, REQ-HR-05).
// Its evidence and the Autopus staging-to-main evidence reject each other with
// attestation_policy_mismatch because their key ids differ. A key rotation
// replaces ADKHarnessEvalKeyID, its committed public key, and the Environment
// secret together; the binding digest covers the key id, so a session signed
// before the rotation never counts for the new binding.
const (
	// ADKHarnessEvalKeyID selects the harness lane public key in the committed
	// allowlist and is the expected key id of its strict policy.
	ADKHarnessEvalKeyID = "adk-harness-eval-2026-10"
	// ADKHarnessEvalTrustLane is the trust_lane of every harness lane
	// attestation.
	ADKHarnessEvalTrustLane = "adk-harness-eval"
)

// Signer refusals. They never quote key bytes or report bytes.
var (
	// ErrSignContextInvalid: a context field is blank or padded, which the
	// strict verifier rejects as attestation_policy_invalid.
	ErrSignContextInvalid = errors.New("eval regression signer: the attestation context needs every field, unpadded")
	// ErrSignKeyInvalid: the key is not 64 bytes or its public half is not
	// the one its seed derives, so no verifier could accept its signature.
	ErrSignKeyInvalid = errors.New("eval regression signer: the private key is not a consistent 64-byte ed25519 key")
	// ErrSignReportInvalid: the report is not one JSON document with an
	// unpadded produced_at and the context's workspace_scope.
	ErrSignReportInvalid = errors.New("eval regression signer: the report needs one JSON document with an unpadded produced_at and the context's workspace_scope")
)

// SignEvalRegressionAttestationV2 signs the exact report bytes into an
// eval_regression_attestation.v2 bound to context, the strict policy a
// verifier of this evidence expects: ExpectedKeyID becomes key_id. The signed
// message is built by evalRegressionAttestationV2Message, the builder the
// strict verifier reconstructs it with, so the two layouts cannot drift.
//
// produced_at is copied from the report as the string it holds, never
// re-rendered: the verifier compares the report's produced_at string with the
// attestation's after the signature check. There is no unsigned output; any
// refusal returns the zero attestation.
func SignEvalRegressionAttestationV2(reportBytes []byte, context EvalRegressionAttestationPolicyV2, privateKey ed25519.PrivateKey) (EvalRegressionAttestationV2, error) {
	if _, ok := ValidateEvalRegressionAttestationPolicyV2(context); !ok {
		return EvalRegressionAttestationV2{}, ErrSignContextInvalid
	}
	if !consistentEd25519PrivateKey(privateKey) {
		return EvalRegressionAttestationV2{}, ErrSignKeyInvalid
	}
	producedAt, ok := signableReportProducedAt(reportBytes, context.WorkspaceScope)
	if !ok {
		return EvalRegressionAttestationV2{}, ErrSignReportInvalid
	}
	sum := sha256.Sum256(reportBytes)
	att := EvalRegressionAttestationV2{
		SchemaVersion:     EvalRegressionAttestationSchemaV2,
		KeyID:             context.ExpectedKeyID,
		Algorithm:         "ed25519",
		ReportSHA256:      hex.EncodeToString(sum[:]),
		ProducedAt:        producedAt,
		TrustLane:         context.TrustLane,
		SourceEnvironment: context.SourceEnvironment,
		TargetEnvironment: context.TargetEnvironment,
		SourceRevision:    context.SourceRevision,
		WorkspaceScope:    context.WorkspaceScope,
	}
	message, err := evalRegressionAttestationV2Message(att)
	if err != nil {
		return EvalRegressionAttestationV2{}, err
	}
	att.SignatureB64 = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message))
	return att, nil
}

// consistentEd25519PrivateKey reports whether key is 64 bytes whose public
// half is the one its seed derives. A corrupted secret with a foreign public
// half signs messages that verify under neither key.
func consistentEd25519PrivateKey(key ed25519.PrivateKey) bool {
	if len(key) != ed25519.PrivateKeySize {
		return false
	}
	seed := key.Seed()
	derived := ed25519.NewKeyFromSeed(seed)
	defer clear(seed)
	defer clear(derived)
	return subtle.ConstantTimeCompare(derived, key) == 1
}

// signableReportProducedAt reads the report context the way the strict
// verifier does: one JSON document, its produced_at and workspace_scope.
func signableReportProducedAt(reportBytes []byte, workspaceScope string) (string, bool) {
	var context evalRegressionReportContextV2
	dec := json.NewDecoder(bytes.NewReader(reportBytes))
	if err := dec.Decode(&context); err != nil {
		return "", false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return "", false
	}
	if context.ProducedAt == "" || strings.TrimSpace(context.ProducedAt) != context.ProducedAt ||
		context.WorkspaceScope != workspaceScope {
		return "", false
	}
	return context.ProducedAt, true
}
