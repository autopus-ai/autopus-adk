package cli

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/evalregression"
)

// Load-failure reason codes emitted by the CLI loader before the pure gate
// runs. These mirror the machine-readable reason literals asserted by the
// deterministic oracles (G3, G4, G6). The pure gate emits the verdict-path
// reason codes (regression_blocked / ok / artifact_stale / artifact_unsafe /
// artifact_invalid); these three cover the pre-decode load failures.
const (
	reasonArtifactMissing = "artifact_missing"
	reasonArtifactInvalid = "artifact_invalid"
	reasonArtifactUnsafe  = "artifact_unsafe"
)

// readEvalRegressionReportBytes reads the raw artifact bytes at path ONCE so the
// same bytes feed both signature verification and strict-decode (no double
// os.ReadFile — closing the time-of-check/time-of-use gap where the file could
// change between verify and decode).
//
// On an absent file (or empty path) it returns reason artifact_missing so the
// caller can fail closed BEFORE verification (missing precedes verify).
func readEvalRegressionReportBytes(path string) (data []byte, reason string, err error) {
	if strings.TrimSpace(path) == "" {
		return nil, reasonArtifactMissing, fmt.Errorf("eval-regression artifact path is empty")
	}
	data, err = os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, reasonArtifactMissing, fmt.Errorf("eval-regression artifact not found")
	}
	if err != nil {
		return nil, reasonArtifactInvalid, err
	}
	return data, "", nil
}

// decodeEvalRegressionReport strictly decodes the given report bytes with
// json.Decoder.DisallowUnknownFields() and a trailing-token EOF check. It is
// split out of the reader so the SAME bytes that were signature-verified are
// the bytes that get decoded.
//
// On failure it returns a machine-readable reason:
//   - unknown/out-of-schema field → artifact_unsafe (REQ-ECI-STRICT-001): an
//     out-of-schema raw-payload field must NOT be silently dropped.
//   - any other JSON syntax/type error, or trailing content after the object →
//     artifact_invalid (REQ-ECI-INVALID-001).
func decodeEvalRegressionReport(data []byte) (evalregression.EvalRegressionReportV1, string, error) {
	var report evalregression.EvalRegressionReportV1

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&report); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return report, reasonArtifactUnsafe, err
		}
		return report, reasonArtifactInvalid, err
	}

	// Reject trailing content after the object: a valid object followed by extra
	// bytes (e.g. `{...}{"x":1}`) is malformed input and must fail closed.
	if _, err := dec.Token(); err != io.EOF {
		return report, reasonArtifactInvalid, fmt.Errorf("trailing data after JSON object")
	}

	return report, "", nil
}

// loadEvalRegressionReport preserves the original single-call loader contract
// (read + strict decode) for the existing G3/G4/unknown-field/trailing-data
// oracles that assert loader behavior directly. It composes the split reader and
// decoder so their behavior stays identical for every load-failure case.
func loadEvalRegressionReport(path string) (evalregression.EvalRegressionReportV1, string, error) {
	data, reason, err := readEvalRegressionReportBytes(path)
	if err != nil {
		return evalregression.EvalRegressionReportV1{}, reason, err
	}
	return decodeEvalRegressionReport(data)
}

// deriveEvalRegressionAttestationPath computes the default sidecar path from the
// report artifact path: same directory, with the base filename component
// `eval_regression_report` rewritten to `eval_regression_attestation` when
// present, else `<artifact>.attestation.json`. An explicit --eval-regression-
// attestation flag overrides this when non-empty.
func deriveEvalRegressionAttestationPath(artifactPath string) string {
	if strings.TrimSpace(artifactPath) == "" {
		return ""
	}
	dir := filepath.Dir(artifactPath)
	base := filepath.Base(artifactPath)
	if strings.Contains(base, "eval_regression_report") {
		return filepath.Join(dir, strings.Replace(base, "eval_regression_report", "eval_regression_attestation", 1))
	}
	return artifactPath + ".attestation.json"
}

// checkEvalRegressionStrict is the v2 production trust path, the only one:
// the v1 helpers that trust the whole allowlist without a lane policy live in
// eval_regression_v1_test.go (SPEC-HARNEVAL-003 REQ-HR-05). It reads the
// artifact bytes once, verifies the signature and the lane context BEFORE any
// decode or gate logic, then strictly decodes and evaluates the gate with the
// injected clock, printing one redacted verdict line (REQ-ECI-SANITIZE-001).
// An invalid policy remains mandatory even when the caller requested advisory
// mode.
func checkEvalRegressionStrict(dir, artifactPath, attestationPath string, maxAge time.Duration, now time.Time, trusted map[string]ed25519.PublicKey, policy evalregression.EvalRegressionAttestationPolicyV2, out io.Writer, quiet, warnOnly bool) bool {
	_ = dir
	_, policyOK := evalregression.ValidateEvalRegressionAttestationPolicyV2(policy)
	return writeEvalRegressionDecision(
		evaluateEvalRegressionStrict(artifactPath, attestationPath, maxAge, now, trusted, policy),
		out,
		quiet,
		warnOnly && policyOK,
	)
}

func writeEvalRegressionDecision(decision evalregression.GateDecision, out io.Writer, quiet, warnOnly bool) bool {
	if !quiet {
		line := "eval-regression: " + decision.Reason
		if decision.AttributedVersion != "" {
			line += " (version=" + decision.AttributedVersion + ")"
		}
		if warnOnly && decision.ExitCode != 0 {
			line += " [advisory: warn-only]"
		}
		fmt.Fprintln(out, line)
	}

	if warnOnly {
		return true
	}
	return decision.ExitCode == 0
}

// @AX:WARN [AUTO] Fail-closed verify-before-trust ordering is load-bearing — missing precedes verify, verify precedes decode, decode precedes gate (INV-EVP-01/04).
// @AX:REASON: reordering steps 1-4 would allow a tampered or unverified artifact to influence the gate result before its signature is checked; the unverified report bytes are never decoded and the blocked field is never read on any verify failure.
// evaluateEvalRegressionStrict builds the fail-closed GateDecision from the
// verify-before-trust chain:
//  1. missing report file            → artifact_missing (precedes verify — S6d)
//  2. signature or lane verify fails → the verify reason (blocked never read)
//  3. strict-decode fails             → the decode reason
//  4. otherwise                       → pure gate verdict
func evaluateEvalRegressionStrict(artifactPath, attestationPath string, maxAge time.Duration, now time.Time, trusted map[string]ed25519.PublicKey, policy evalregression.EvalRegressionAttestationPolicyV2) evalregression.GateDecision {
	if reason, ok := evalregression.ValidateEvalRegressionAttestationPolicyV2(policy); !ok {
		return evalregression.GateDecision{Blocked: true, ExitCode: 1, Reason: reason}
	}
	return evaluateEvalRegressionWithVerifier(
		artifactPath,
		attestationPath,
		maxAge,
		now,
		func(reportBytes, attestationBytes []byte) (string, bool) {
			return evalregression.VerifyEvalRegressionArtifactV2Strict(reportBytes, attestationBytes, trusted, policy)
		},
	)
}

type evalRegressionVerifier func(reportBytes, attestationBytes []byte) (reason string, ok bool)

func evaluateEvalRegressionWithVerifier(artifactPath, attestationPath string, maxAge time.Duration, now time.Time, verify evalRegressionVerifier) evalregression.GateDecision {
	// 1. Read report bytes once. Missing precedes verify (fail closed).
	reportBytes, missReason, missErr := readEvalRegressionReportBytes(artifactPath)
	if missErr != nil {
		return evalregression.GateDecision{Blocked: true, ExitCode: 1, Reason: missReason}
	}

	// 2. Read attestation bytes. An absent file yields empty bytes; the verifier
	// maps empty bytes to artifact_unsigned, so the read error is intentionally
	// ignored (empty content handles the absent case).
	attData, _ := os.ReadFile(attestationPath)

	// 3. Verify signature over the raw report bytes BEFORE decode. On failure the
	// unverified report is never decoded and its blocked field is never read.
	if reason, ok := verify(reportBytes, attData); !ok {
		return evalregression.GateDecision{Blocked: true, ExitCode: 1, Reason: reason}
	}

	// 4. Only a verified artifact is strictly decoded and gated.
	report, decodeReason, decodeErr := decodeEvalRegressionReport(reportBytes)
	if decodeErr != nil {
		return evalregression.GateDecision{Blocked: true, ExitCode: 1, Reason: decodeReason}
	}
	return evalregression.EvaluateEvalRegressionGate(report, now, maxAge)
}
