package cli

import (
	"crypto/ed25519"
	"io"
	"time"

	"github.com/insajin/autopus-adk/pkg/evalregression"
)

// The v1 helpers trust every allowlisted key without a lane policy, so no
// production path may call them (SPEC-HARNEVAL-003 REQ-HR-05). They stay
// here for the tests of the v1 sidecar contract; the static test in
// eval_harness_lane_test.go keeps the non-test callers of
// VerifyEvalRegressionArtifact at zero.

// checkEvalRegression reads the artifact bytes once, verifies the v1 ed25519
// signature BEFORE any decode or gate logic, then strictly decodes and
// evaluates the gate with the injected clock, printing one redacted verdict
// line. WHERE warnOnly is set it always returns true (REQ-ECI-WARN-001).
func checkEvalRegression(dir, artifactPath, attestationPath string, maxAge time.Duration, now time.Time, trusted map[string]ed25519.PublicKey, out io.Writer, quiet, warnOnly bool) bool {
	_ = dir // artifact path is absolute/explicit; dir is accepted for dispatch symmetry.
	return writeEvalRegressionDecision(
		evaluateEvalRegression(artifactPath, attestationPath, maxAge, now, trusted),
		out,
		quiet,
		warnOnly,
	)
}

// evaluateEvalRegression builds the fail-closed v1 GateDecision through the
// same verify-before-trust chain as the strict path.
func evaluateEvalRegression(artifactPath, attestationPath string, maxAge time.Duration, now time.Time, trusted map[string]ed25519.PublicKey) evalregression.GateDecision {
	return evaluateEvalRegressionWithVerifier(
		artifactPath,
		attestationPath,
		maxAge,
		now,
		func(reportBytes, attestationBytes []byte) (string, bool) {
			return evalregression.VerifyEvalRegressionArtifact(reportBytes, attestationBytes, trusted)
		},
	)
}
