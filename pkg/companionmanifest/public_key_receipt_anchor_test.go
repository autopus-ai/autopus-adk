package companionmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func publicKeyReceiptA0PinsFixture(t *testing.T) ([4]string, publicKeyReceiptA0Anchor) {
	t.Helper()
	receiptBytes, signature, _, publicKey := issuedPublicKeyReceipt(t)
	digests := [4][sha256.Size]byte{
		sha256.Sum256(receiptBytes),
		sha256.Sum256(signature),
		sha256.Sum256(publicKey),
		publicKeyReceiptRecordDigest(receiptBytes, signature),
	}
	var pins [4]string
	for index, digest := range digests {
		pins[index] = "sha256:" + hex.EncodeToString(digest[:])
	}
	return pins, testPublicKeyReceiptA0Anchor(t, receiptBytes, signature, publicKey)
}

func TestPublicKeyReceiptA0AnchorFromPins_ExactPinsConstructAnchor(t *testing.T) {
	pins, want := publicKeyReceiptA0PinsFixture(t)
	got, err := publicKeyReceiptA0AnchorFromPins(pins)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("anchor = %#v, want %#v", got, want)
	}
}

func TestPublicKeyReceiptA0AnchorFromPins_InvalidPinsFailClosed(t *testing.T) {
	valid, _ := publicKeyReceiptA0PinsFixture(t)
	tests := []struct {
		name   string
		index  int
		value  string
		isGone bool
	}{
		{name: "unprovisioned", index: 0, value: "", isGone: true},
		{name: "receipt", index: 0, value: "sha256:short"},
		{name: "signature", index: 1, value: "sha256:short"},
		{name: "public key", index: 2, value: "sha256:short"},
		{name: "record", index: 3, value: "sha256:short"},
		{name: "nonhex", index: 0, value: "sha256:" + string(make([]byte, 64))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pins := valid
			pins[test.index] = test.value
			_, err := publicKeyReceiptA0AnchorFromPins(pins)
			if err == nil {
				t.Fatal("invalid A0 pin was accepted")
			}
			if test.isGone && !errors.Is(err, ErrPublicKeyReceiptA0Unprovisioned) {
				t.Fatalf("unprovisioned error = %v", err)
			}
		})
	}
}

func TestTrustedPublicKeyReceipt_ReceiptRequiresValidCapability(t *testing.T) {
	trusted := trustedPublicKeyReceiptFixture(t)
	receipt, ok := trusted.Receipt()
	if !ok || receipt.KeyID != validPublicKeyReceiptClaims().KeyID {
		t.Fatalf("trusted receipt claims = %#v/%v", receipt, ok)
	}
	if _, ok := (TrustedPublicKeyReceipt{}).Receipt(); ok {
		t.Fatal("zero capability exposed receipt claims")
	}
}
