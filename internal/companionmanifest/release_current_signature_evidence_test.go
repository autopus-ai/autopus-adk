package companionmanifest

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func assertCurrentReleaseSignatureLog(t *testing.T, path string) {
	t.Helper()
	log, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(log, []byte("openssl-verify\ncosign verify-blob ")) {
		t.Fatalf("signature verification order = %s", log)
	}
	for _, required := range []string{
		"--bundle", "checksums.txt.bundle", "--certificate-identity",
		"https://github.com/autopus-ai/autopus-adk/.github/workflows/release.yaml@refs/tags/v0.50.118",
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com",
	} {
		if !bytes.Contains(log, []byte(required)) {
			t.Fatalf("cosign verification arguments missing %q: %s", required, log)
		}
	}
}

func TestCurrentReleaseVerifier_RejectsCryptographicFailures(t *testing.T) {
	tests := []struct {
		name, want, wantLog, forbidLog string
		mutate                         func(*currentReleaseFixture)
	}{
		{name: "ecdsa", want: "release ECDSA signature verification failed",
			wantLog: "openssl-verify", forbidLog: "cosign ",
			mutate: func(f *currentReleaseFixture) { f.openSSLVerifyFail = true }},
		{name: "cosign", want: "release keyless signature verification failed",
			wantLog: "openssl-verify\ncosign verify-blob",
			mutate:  func(f *currentReleaseFixture) { f.cosignFail = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCurrentReleaseFixture(t)
			test.mutate(fixture)
			output, err := fixture.run()
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("cryptographic failure result: %v\n%s", err, output)
			}
			if _, statErr := os.Lstat(fixture.output); !os.IsNotExist(statErr) {
				t.Fatalf("failed verification materialized output: %v", statErr)
			}
			log, readErr := os.ReadFile(fixture.signatureLog)
			if readErr != nil || !bytes.Contains(log, []byte(test.wantLog)) ||
				(test.forbidLog != "" && bytes.Contains(log, []byte(test.forbidLog))) {
				t.Fatalf("signature failure log = %q, error = %v", log, readErr)
			}
		})
	}
}
