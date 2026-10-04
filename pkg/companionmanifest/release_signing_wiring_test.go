package companionmanifest

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"strings"
	"testing"
)

const (
	releaseK1Fingerprint = "e1fdfe066484c7eae8ff16fa4b1ee6237b8d06299c2b66ced485f029af77837f"
	releaseK2Fingerprint = "93d9f681d829f2d0bdba7e1853e6acf9ae2ffd2c760355853218e920c35cc5ff"
)

func TestReleaseSigning_CheckedInAnchorsMatchFullSPKIFingerprints(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint string
	}{
		{name: "k1", fingerprint: releaseK1Fingerprint},
		{name: "k2", fingerprint: releaseK2Fingerprint},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertCheckedInReleaseAnchor(t, test.name, test.fingerprint)
		})
	}
}

func assertCheckedInReleaseAnchor(t *testing.T, name, fingerprint string) {
	t.Helper()
	publicPEM := readRepositoryFile(t, "scripts/release-signing/release-"+name+"-public.pem")
	fingerprintFile := readRepositoryFile(t, "scripts/release-signing/release-"+name+".fingerprint")

	if !bytes.HasPrefix(publicPEM, []byte("-----BEGIN PUBLIC KEY-----")) {
		t.Fatalf("release %s PUBLIC KEY PEM must begin at byte zero", name)
	}
	block, rest := pem.Decode(publicPEM)
	if block == nil || block.Type != "PUBLIC KEY" || len(block.Headers) != 0 || len(strings.TrimSpace(string(rest))) != 0 {
		t.Fatalf("release %s must contain exactly one headerless PUBLIC KEY PEM block", name)
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse release %s SPKI: %v", name, err)
	}
	publicKey, ok := parsed.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve.Params().Name != "P-256" {
		t.Fatalf("release %s is not ECDSA P-256: %T", name, parsed)
	}
	digest := sha256.Sum256(block.Bytes)
	if got := hex.EncodeToString(digest[:]); got != fingerprint {
		t.Fatalf("release %s fingerprint = %s, want %s", name, got, fingerprint)
	}
	if got := strings.TrimSpace(string(fingerprintFile)); got != fingerprint {
		t.Fatalf("checked-in %s fingerprint = %q, want %s", name, got, fingerprint)
	}
}

func TestReleaseSigning_ProducerAndPreflightAreFailClosed(t *testing.T) {
	producer := string(readRepositoryFile(t, "scripts/release-signing/sign-checksums.sh"))
	preflight := string(readRepositoryFile(t, "scripts/release-signing/verify-key-pair.sh"))

	for name, source := range map[string]string{"producer": producer, "preflight": preflight} {
		for _, forbidden := range []string{"jq ", "json", "set -x", "|| true", "eval ", "printenv"} {
			if strings.Contains(strings.ToLower(source), forbidden) {
				t.Fatalf("%s contains forbidden token %q", name, forbidden)
			}
		}
		for _, required := range []string{"set -eu", "umask 077", "prime256v1"} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s missing fail-closed contract %q", name, required)
			}
		}
	}
	for _, required := range []string{
		"AUTOPUS-RELEASE-SIGNATURE-V1", "LC_ALL=C sort", "openssl dgst -sha256 -sign",
	} {
		if !strings.Contains(producer, required) {
			t.Fatalf("producer missing envelope contract %q", required)
		}
	}
}

func TestReleaseSigning_WorkflowMaterializesAndPreflightsKeyBeforeGoReleaser(t *testing.T) {
	workflow := releaseWorkflowContract(t)
	workflowSource := string(readRepositoryFile(t, ".github/workflows/release.yaml"))
	for _, required := range []string{
		"- 'v0.50.123'", `autopus-$GITHUB_REF_NAME-checksums.txt`,
		`GITHUB_REF_NAME="$GITHUB_REF_NAME"`, `COMPANION_VERSION="${GITHUB_REF_NAME#v}"`,
	} {
		if !strings.Contains(workflowSource, required) {
			t.Fatalf("signing workflow missing derived coordinate contract %q", required)
		}
	}
	// The release workflow trigger owns exactly one A23 tag coordinate.
	if got := strings.Count(workflowSource, "- 'v0.50.123'"); got != 1 {
		t.Fatalf("signing workflow release trigger is not exact: count=%d", got)
	}
	prepareIndex, _ := releaseWorkflowStepContaining(t, workflow, "Prepare release credentials")
	materializeIndex, materialize := releaseWorkflowStepContaining(t, workflow, "Materialize release signing key")
	preflightIndex, preflight := releaseWorkflowStepContaining(t, workflow, "Verify release signing key pair")
	releaseIndex, release := releaseWorkflowStepContaining(t, workflow, "goreleaser release --clean")
	if !(prepareIndex < materializeIndex && materializeIndex < preflightIndex && preflightIndex < releaseIndex) {
		t.Fatalf("release signing order = prepare:%d materialize:%d preflight:%d release:%d", prepareIndex, materializeIndex, preflightIndex, releaseIndex)
	}
	if got := materialize.Env["ADK_RELEASE_ECDSA_PRIVATE_KEY"]; got != "${{ secrets.ADK_RELEASE_ECDSA_PRIVATE_KEY }}" {
		t.Fatalf("materialize secret binding = %q", got)
	}
	for _, required := range []string{
		`release_signing_key_path="$credential_dir/release-ecdsa-private-key"`,
		`printf '%s' "$ADK_RELEASE_ECDSA_PRIVATE_KEY" > "$release_signing_key_path"`,
		`chmod 0600`,
	} {
		if !strings.Contains(materialize.Run, required) {
			t.Fatalf("credential preparation missing %q", required)
		}
	}
	for _, required := range []string{
		"scripts/release-signing/verify-key-pair.sh",
		"scripts/release-signing/release-k1-public.pem",
		"scripts/release-signing/release-k1.fingerprint",
	} {
		if !strings.Contains(preflight.Run, required) {
			t.Fatalf("key-pair preflight missing %q", required)
		}
	}
	if strings.Contains(preflight.Run, "ADK_RELEASE_ECDSA_PRIVATE_KEY\"") {
		t.Fatal("preflight consumes raw private-key secret instead of the 0600 file")
	}
	if _, ok := release.Env["ADK_RELEASE_ECDSA_PRIVATE_KEY"]; ok {
		t.Fatal("GoReleaser receives raw release private-key secret")
	}
	if got := release.Env["ADK_RELEASE_ECDSA_PRIVATE_KEY_FILE"]; got == "" {
		t.Fatal("GoReleaser does not receive the release private-key file path")
	}
	if !strings.Contains(release.Run, `ADK_RELEASE_ECDSA_PRIVATE_KEY_FILE="$ADK_RELEASE_ECDSA_PRIVATE_KEY_FILE"`) {
		t.Fatal("GoReleaser env -i command does not receive the key-file path")
	}
	if _, err := os.Stat("../../scripts/release-signing/sign-checksums.sh"); err != nil {
		t.Fatalf("release signing producer is not checked in: %v", err)
	}
	if strings.Contains(workflowSource, "release-k2") {
		t.Fatal("v0.50.109 workflow must keep K2 offline and sign with K1 only")
	}
}
