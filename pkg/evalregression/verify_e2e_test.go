package evalregression

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

const e2eKeyID = "egl-e2e-1"

var e2eFixedNow = time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

func e2eReportJSON(blocked bool, producedAt, attributedVersion string) []byte {
	reason := "candidate within threshold"
	delta := "0.04"
	if blocked {
		reason = "regression exceeds threshold"
		delta = "0.30"
	}
	return []byte(`{"schema_version":"eval_regression_report.v1","blocked":` +
		boolStr(blocked) + `,"regression_delta":` + delta +
		`,"attributed_version":"` + attributedVersion +
		`","comparison_scope":"workspace","threshold_metric":"pass_rate","threshold_value":0.10,` +
		`"reason":"` + reason +
		`","baseline_ref":"baseline-e2e","produced_at":"` + producedAt +
		`","workspace_scope":"ws-e2e","raw_payload_present":false,` +
		`"redaction_status":"redacted","retention_class":"standard"}`)
}

func evaluateE2E(t *testing.T, reportBytes, attBytes []byte, trusted map[string]ed25519.PublicKey) (string, int) {
	t.Helper()
	if reason, ok := VerifyEvalRegressionArtifact(reportBytes, attBytes, trusted); !ok {
		return reason, 1
	}
	var report EvalRegressionReportV1
	if err := jsonUnmarshalReport(reportBytes, &report); err != nil {
		t.Fatalf("verified e2e report should decode: %v", err)
	}
	decision := EvaluateEvalRegressionGate(report, e2eFixedNow, 24*time.Hour)
	return decision.Reason, decision.ExitCode
}

func jsonUnmarshalReport(data []byte, report *EvalRegressionReportV1) error {
	return json.Unmarshal(data, report)
}

func TestEvalRegressionLiveGateE2EReasons(t *testing.T) {
	producedAt := e2eFixedNow.Add(-1 * time.Hour).Format(time.RFC3339)
	headSHA := "0123456789abcdef0123456789abcdef01234567"

	t.Run("green pass", func(t *testing.T) {
		priv, trusted := newSigner(t)
		report := e2eReportJSON(false, producedAt, "candidate")
		att := signBytes(t, report, verifyKeyID, priv)

		reason, exitCode := evaluateE2E(t, report, att, trusted)
		if reason != reasonOK || exitCode != 0 {
			t.Fatalf("green pass: expected (%q, 0), got (%q, %d)", reasonOK, reason, exitCode)
		}
	})

	t.Run("tampered byte", func(t *testing.T) {
		priv, trusted := newSigner(t)
		original := e2eReportJSON(true, producedAt, headSHA)
		att := signBytes(t, original, verifyKeyID, priv)
		mutated := []byte(strings.Replace(string(original), `"blocked":true`, `"blocked":false`, 1))
		if string(mutated) == string(original) {
			t.Fatalf("tampered byte: setup failed to mutate blocked")
		}

		reason, exitCode := evaluateE2E(t, mutated, att, trusted)
		if reason != reasonSignatureInvalid || exitCode != 1 {
			t.Fatalf("tampered byte: expected (%q, 1), got (%q, %d)", reasonSignatureInvalid, reason, exitCode)
		}
	})

	t.Run("missing attestation", func(t *testing.T) {
		_, trusted := newSigner(t)
		report := e2eReportJSON(false, producedAt, "candidate")

		reason, exitCode := evaluateE2E(t, report, nil, trusted)
		if reason != reasonArtifactUnsigned || exitCode != 1 {
			t.Fatalf("missing attestation: expected (%q, 1), got (%q, %d)", reasonArtifactUnsigned, reason, exitCode)
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		priv, _ := newSigner(t)
		report := e2eReportJSON(false, producedAt, "candidate")
		att := signBytes(t, report, e2eKeyID, priv)

		reason, exitCode := evaluateE2E(t, report, att, map[string]ed25519.PublicKey{})
		if reason != reasonSignatureKeyUnknown || exitCode != 1 {
			t.Fatalf("unknown key: expected (%q, 1), got (%q, %d)", reasonSignatureKeyUnknown, reason, exitCode)
		}
	})

	t.Run("blocked head sha", func(t *testing.T) {
		priv, trusted := newSigner(t)
		report := e2eReportJSON(true, producedAt, headSHA)
		att := signBytes(t, report, verifyKeyID, priv)

		reason, exitCode := evaluateE2E(t, report, att, trusted)
		if reason != reasonBlocked || exitCode != 1 {
			t.Fatalf("blocked head sha: expected (%q, 1), got (%q, %d)", reasonBlocked, reason, exitCode)
		}
	})
}

// promotionKeyID is the Autopus staging-to-main lane key id.
const promotionKeyID = "autopus-eval-staging-to-main-2026-07"

// committedPublicKeys is the exact committed allowlist (SPEC-HARNEVAL-003
// Existing Test Changes, L120 and L138): the Autopus promotion key and the
// harness lane key under ADKHarnessEvalKeyID (issued in T10), each as base64
// of its 32-byte ed25519 public key. A rotation changes this table on purpose.
var committedPublicKeys = map[string]string{
	promotionKeyID:      "D6euTz5IarNy68TfJ4tdzOwVomIXoiDEzEtefKmprz8=",
	ADKHarnessEvalKeyID: "onPT0WydtQRPPSx13eSKsAcW5GeBvjwDF+fz8xILMNs=",
}

func allowlistKeyIDs[V any](keys map[string]V) []string {
	ids := make([]string, 0, len(keys))
	for id := range keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// requireCommittedAllowlist fails unless keys is exactly committedPublicKeys.
func requireCommittedAllowlist(t *testing.T, keys map[string]ed25519.PublicKey, when string) {
	t.Helper()
	if got, want := allowlistKeyIDs(keys), allowlistKeyIDs(committedPublicKeys); !slices.Equal(got, want) {
		t.Fatalf("%s: committed allowlist key ids = %v, want %v", when, got, want)
	}
	for keyID, want := range committedPublicKeys {
		if len(keys[keyID]) != ed25519.PublicKeySize {
			t.Fatalf("%s: committed public key %q length = %d, want %d", when, keyID, len(keys[keyID]), ed25519.PublicKeySize)
		}
		if got := base64.StdEncoding.EncodeToString(keys[keyID]); got != want {
			t.Fatalf("%s: committed public key %q = %q, want %q", when, keyID, got, want)
		}
	}
}

func TestCommittedAllowlistContainsPromotionKeyAndIsDefensiveForE2E(t *testing.T) {
	keys := CommittedEvalRegressionPublicKeys()
	requireCommittedAllowlist(t, keys, "first read")
	// Each lane policy pins its own key id, and that only separates the lanes
	// while the ids hold different keys: under one shared key, evidence one
	// signer relabels with the other lane's key id and values would verify there.
	if bytes.Equal(keys[promotionKeyID], keys[ADKHarnessEvalKeyID]) {
		t.Fatal("the promotion and harness lane key ids hold the same public key")
	}

	for keyID := range committedPublicKeys {
		keys[keyID][0] ^= 0xff
	}
	delete(keys, promotionKeyID)
	keys["egl-should-not-stick"] = ed25519.PublicKey("attacker")
	requireCommittedAllowlist(t, CommittedEvalRegressionPublicKeys(), "after mutating the defensive copy")
}
