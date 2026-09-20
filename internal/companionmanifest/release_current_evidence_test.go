package companionmanifest

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCurrentReleaseVerifier_AcceptsExactImmutableRelease(t *testing.T) {
	fixture := newCurrentReleaseFixture(t)
	output, err := fixture.run()
	if err != nil {
		t.Fatalf("exact release rejected: %v\n%s", err, output)
	}
	got, err := os.ReadFile(fixture.output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, fixture.checksums) {
		t.Fatal("materialized checksums differ from server bytes")
	}
	assertCurrentReleaseSignatureLog(t, fixture.signatureLog)
	assertCurrentReleaseVerifierLog(t, fixture.verifierLog)
}

func TestCurrentReleaseVerifier_RejectsUntrustedReleaseEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*currentReleaseFixture)
		want   string
	}{
		{name: "wrong_release_id", mutate: func(f *currentReleaseFixture) {
			f.release.ID++
		}, want: "release id, tag, source, author, state, assets, or digests differ"},
		{name: "wrong_source", mutate: func(f *currentReleaseFixture) {
			f.release.TargetCommitish = strings.Repeat("f", 40)
		}, want: "release id, tag, source, author, state, assets, or digests differ"},
		{name: "wrong_author", mutate: func(f *currentReleaseFixture) {
			f.release.Author.ID++
		}, want: "release id, tag, source, author, state, assets, or digests differ"},
		{name: "mutable_release", mutate: func(f *currentReleaseFixture) {
			f.release.Immutable = false
		}, want: "release id, tag, source, author, state, assets, or digests differ"},
		{name: "partial_asset_set", mutate: func(f *currentReleaseFixture) {
			f.release.Assets = f.release.Assets[:len(f.release.Assets)-1]
		}, want: "release id, tag, source, author, state, assets, or digests differ"},
		{name: "duplicate_asset", mutate: func(f *currentReleaseFixture) {
			f.release.Assets[len(f.release.Assets)-1].Name = f.release.Assets[0].Name
		}, want: "release id, tag, source, author, state, assets, or digests differ"},
		{name: "asset_not_uploaded", mutate: func(f *currentReleaseFixture) {
			f.asset("checksums.txt").State = "new"
		}, want: "release id, tag, source, author, state, assets, or digests differ"},
		{name: "checksums_server_digest_mismatch", mutate: func(f *currentReleaseFixture) {
			f.asset("checksums.txt").Digest = "sha256:" + strings.Repeat("f", 64)
		}, want: "differs from GitHub metadata"},
		{name: "bundle_server_digest_mismatch", mutate: func(f *currentReleaseFixture) {
			f.asset("checksums.txt.bundle").Digest = "sha256:" + strings.Repeat("f", 64)
		}, want: "differs from GitHub metadata"},
		{name: "envelope_server_digest_mismatch", mutate: func(f *currentReleaseFixture) {
			f.asset("checksums.txt.signatures").Digest = "sha256:" + strings.Repeat("f", 64)
		}, want: "differs from GitHub metadata"},
		{name: "cask_archive_checksum_mismatch", mutate: func(f *currentReleaseFixture) {
			f.asset(currentReleaseArchives[0]).Digest = "sha256:" + strings.Repeat("e", 64)
		}, want: "checksums.txt differs from GitHub API digest"},
		{name: "windows_archive_checksum_mismatch", mutate: func(f *currentReleaseFixture) {
			f.asset(currentReleaseArchives[7]).Digest = "sha256:" + strings.Repeat("d", 64)
		}, want: "checksums.txt differs from GitHub API digest"},
		{name: "report_differs_from_evidence_pin", mutate: func(f *currentReleaseFixture) {
			f.replaceAsset("omp-context-promotion-report.v1.json", []byte("{}\n"))
		}, want: "released OMP report digest differs"},
		{name: "attestation_differs_from_evidence_pin", mutate: func(f *currentReleaseFixture) {
			f.replaceAsset("omp-context-promotion-attestation.v2.json", []byte("{}\n"))
		}, want: "released OMP attestation digest differs"},
		{name: "standalone_lineage_differs_from_archive", mutate: func(f *currentReleaseFixture) {
			f.replaceAsset("release-lineage-v1.json", []byte("different standalone lineage\n"))
		}, want: "standalone and archived lineage bytes differ"},
		{name: "lineage_signature_is_not_raw_ed25519", mutate: func(f *currentReleaseFixture) {
			f.replaceAsset("release-lineage-v1.sig", bytes.Repeat([]byte{0x31}, 63))
		}, want: "lineage signature is not raw Ed25519 bytes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCurrentReleaseFixture(t)
			test.mutate(fixture)
			fixture.writeRelease(t)
			output, err := fixture.run()
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("untrusted evidence result: %v\n%s", err, output)
			}
			if _, statErr := os.Lstat(fixture.output); !os.IsNotExist(statErr) {
				t.Fatalf("failed verification materialized output: %v", statErr)
			}
		})
	}
}

func TestCurrentReleaseVerifier_UsesPolicyBoundHistoricalRecovery(t *testing.T) {
	source := readReleaseFile(t, "scripts/companion-release/verify-current-release.sh")
	for _, required := range []string{
		"OMP_CONTEXT_STATIC_POLICY_B64", "--mode historical",
		"omp-context-promotion-report.v1.json", "omp-context-promotion-attestation.v2.json",
		"exactly fifteen A29 normal release assets verified",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("current release verifier missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"--mode active", "--expected-signing-key-id", "omp-context-bridge-release.v1.json",
		"adk-key-rotation-v1.json", "adk-key-rotation-v1.sig",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("current release verifier retains forbidden authority or asset %q", forbidden)
		}
	}
}

func TestCurrentReleaseVerifier_RejectsHistoricalEvidenceFailure(t *testing.T) {
	fixture := newCurrentReleaseFixture(t)
	fixture.evidenceHistoricalFail = true
	fixture.writeCommandMocks(t)
	output, err := fixture.run()
	if err == nil || !strings.Contains(output, "historical-recovery OMP evidence is invalid") {
		t.Fatalf("historical evidence failure result: %v\n%s", err, output)
	}
	if _, statErr := os.Lstat(fixture.output); !os.IsNotExist(statErr) {
		t.Fatalf("failed verification materialized output: %v", statErr)
	}
}

func assertCurrentReleaseVerifierLog(t *testing.T, path string) {
	t.Helper()
	log, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(string(log)), "\n"); len(lines) != 3 {
		t.Fatalf("release verifier invocation count = %d, want 3: %s", len(lines), log)
	}
	for _, required := range []string{
		"companion-manifest-verifier --artifact ", "--platform darwin --architecture arm64",
		"omp-context-lineage-verifier --lineage ", "--target darwin-arm64 --version 0.50.118",
		"omp-context-evidence-verifier --mode historical",
		"--source-repository autopus-ai/autopus-adk",
		"--candidate-repository autopus-ai/autopus-adk",
	} {
		if !bytes.Contains(log, []byte(required)) {
			t.Fatalf("release verifier invocation missing %q: %s", required, log)
		}
	}
}
