//go:build darwin || linux

package companionmanifest

import (
	"encoding/base64"
	"testing"
	"time"
)

const productionA0Receipt = `{"schema_version":"adk-companion-public-key-receipt.v1","key_id":"adk-release-2026-q3-b0","algorithm":"ed25519","public_key_encoding":"base64-raw-32","public_key_base64":"lxdZaGsN1ZHutUtoYdRZa/JWB2gbG33HtMehzV4DG18=","public_key_sha256":"sha256:c387da9e9c43dbaa2605207a00635c84937ff397a8b6ed73414d2e66b89941a4","issued_at":"2026-07-14T12:43:14Z","expires_at":"2027-07-14T12:43:14Z","handoff":"v1","minimum_rollback_floor":5069}`
const productionA0SignatureBase64 = "jeJtBnOeDV5scylKHn4UbYVhS7BUawEpgzMJa+dWzpSzKJN4ADrJmstCjdTnaWtzFfGeM3MvdnVH4zGFXt1gDg=="

func productionA0Bundle(t *testing.T, mutate func([]byte, []byte)) string {
	t.Helper()
	receipt := []byte(productionA0Receipt)
	signature, err := base64.StdEncoding.Strict().DecodeString(productionA0SignatureBase64)
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(receipt, signature)
	}
	return writePublicKeyReceiptTrustBundle(t, receipt, signature)
}

func productionA0Policy() PublicKeyReceiptPolicy {
	return PublicKeyReceiptPolicy{
		Now:                  time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
		ExpectedKeyID:        "adk-release-2026-q3-b0",
		ExpectedHandoff:      "v1",
		MinimumRollbackFloor: 5069,
	}
}

func TestVerifyConfiguredPublicKeyReceiptBundle_ProductionA0Record_ReturnsTrust(t *testing.T) {
	trusted, err := VerifyConfiguredPublicKeyReceiptBundle(
		productionA0Bundle(t, nil),
		productionA0Policy(),
	)
	if err != nil {
		t.Fatalf("production A0 bundle verification: %v", err)
	}
	receipt, ok := trusted.Receipt()
	if !ok || receipt.KeyID != productionA0Policy().ExpectedKeyID {
		t.Fatalf("trusted production receipt = %#v/%v", receipt, ok)
	}
}

func TestVerifyConfiguredPublicKeyReceiptBundle_ProductionA0Tamper_FailsClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]byte, []byte)
	}{
		{name: "receipt", mutate: func(receipt, _ []byte) { receipt[len(receipt)-1] ^= 1 }},
		{name: "signature", mutate: func(_, signature []byte) { signature[0] ^= 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := VerifyConfiguredPublicKeyReceiptBundle(
				productionA0Bundle(t, test.mutate),
				productionA0Policy(),
			); err == nil {
				t.Fatal("tampered production A0 bundle established trust")
			}
		})
	}
}

// The bundle loader exists only on darwin and linux; elsewhere bundle
// verification is unsupported, so this test lives with the unix fixtures.
func TestVerifyConfiguredPublicKeyReceiptBundle_PrivateLoaderUsesBundleAuthority(t *testing.T) {
	bundlePath, anchor := anchoredPublicKeyReceiptBundleFixture(t)
	trusted, err := verifyConfiguredPublicKeyReceiptBundle(
		bundlePath,
		validPublicKeyReceiptPolicy(),
		func() (publicKeyReceiptA0Anchor, error) { return anchor, nil },
	)
	if err != nil || !trusted.valid() {
		t.Fatalf("configured bundle trust = %#v/%v", trusted, err)
	}
}
