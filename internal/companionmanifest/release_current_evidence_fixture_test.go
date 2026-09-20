package companionmanifest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var currentReleaseArchives = []string{
	"autopus-adk_0.50.118_darwin_amd64.tar.gz",
	"autopus-adk_0.50.118_darwin_arm64.tar.gz",
	"autopus-adk_0.50.118_linux_amd64.tar.gz",
	"autopus-adk_0.50.118_linux_arm64.tar.gz",
	"autopus-adk_0.50.118_windows_amd64.tar.gz",
	"autopus-adk_0.50.118_windows_amd64.zip",
	"autopus-adk_0.50.118_windows_arm64.tar.gz",
	"autopus-adk_0.50.118_windows_arm64.zip",
}

type currentReleaseAsset struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Size   int    `json:"size"`
	Digest string `json:"digest"`
}

type currentReleaseDocument struct {
	ID              int    `json:"id"`
	TagName         string `json:"tag_name"`
	TargetCommitish string `json:"target_commitish"`
	Author          struct {
		ID int `json:"id"`
	} `json:"author"`
	Draft      bool                  `json:"draft"`
	Prerelease bool                  `json:"prerelease"`
	Immutable  bool                  `json:"immutable"`
	Assets     []currentReleaseAsset `json:"assets"`
}

type currentReleaseFixture struct {
	root, state, output, signatureLog, verifierLog string
	openSSLVerifyFail, cosignFail                  bool
	evidenceHistoricalFail                         bool
	checksums                                      []byte
	reportSHA256, attestationSHA256                string
	release                                        currentReleaseDocument
}

func newCurrentReleaseFixture(t *testing.T) *currentReleaseFixture {
	t.Helper()
	root := t.TempDir()
	state := filepath.Join(root, "state")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(state, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	autoBody := []byte("fixture distributed auto binary\n")
	autoDigest := sha256.Sum256(autoBody)
	lineage := []byte("{\"schema\":\"autopus.omp_context_release_lineage.v1\"}\n")
	lineageSignature := bytes.Repeat([]byte{0x31}, 64)
	archive := makeCurrentReleaseArchive(t, map[string][]byte{
		"auto": autoBody,
		"adk-companion-manifest.json": []byte(fmt.Sprintf(
			"{\"artifact_digest\":\"sha256:%x\"}\n", autoDigest)),
		"adk-companion-manifest.sig":                                      bytes.Repeat([]byte{0x32}, 64),
		"adk-companion-public-key-receipt.bundle/public-key-receipt.json": []byte("{\"schema\":\"autopus.adk_companion_public_key_receipt.v1\"}\n"),
		"adk-companion-public-key-receipt.bundle/public-key-receipt.sig":  bytes.Repeat([]byte{0x33}, 64),
		"release-lineage-v1.json":                                         lineage,
		"release-lineage-v1.sig":                                          lineageSignature,
	})
	assetBodies := make(map[string][]byte, 15)
	var checksums bytes.Buffer
	for _, name := range currentReleaseArchives {
		body := []byte("fixture archive " + name + "\n")
		if name == currentReleaseArchives[1] {
			body = archive
		}
		assetBodies[name] = body
		digest := sha256.Sum256(body)
		fmt.Fprintf(&checksums, "%x  %s\n", digest, name)
	}
	assetBodies["checksums.txt"] = append([]byte(nil), checksums.Bytes()...)
	assetBodies["checksums.txt.bundle"] = []byte("fixture cosign bundle\n")
	assetBodies["checksums.txt.signatures"] = []byte("AUTOPUS-RELEASE-SIGNATURE-V1\n" +
		"e1fdfe066484c7eae8ff16fa4b1ee6237b8d06299c2b66ced485f029af77837f\tMAYCAQECAQE=\n")
	assetBodies["omp-context-promotion-report.v1.json"] = []byte("{\"candidate\":{\"artifact_sha256\":\"sha256:" + strings.Repeat("a", 64) + "\"}}\n")
	assetBodies["omp-context-promotion-attestation.v2.json"] = []byte("{\"key_id\":\"omp-context-promotion-2026-q3-k3\"}\n")
	assetBodies["release-lineage-v1.json"] = lineage
	assetBodies["release-lineage-v1.sig"] = lineageSignature
	assetNames := append(append([]string(nil), currentReleaseArchives...),
		"checksums.txt", "checksums.txt.bundle", "checksums.txt.signatures",
		"omp-context-promotion-report.v1.json", "omp-context-promotion-attestation.v2.json",
		"release-lineage-v1.json", "release-lineage-v1.sig")
	assets := make([]currentReleaseAsset, 0, len(assetNames))
	for index, name := range assetNames {
		body := assetBodies[name]
		digest := sha256.Sum256(body)
		assets = append(assets, currentReleaseAsset{ID: index + 1, Name: name,
			State: "uploaded", Size: len(body), Digest: fmt.Sprintf("sha256:%x", digest)})
		if err := os.WriteFile(filepath.Join(state, "assets", fmt.Sprint(index+1)), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reportDigest := sha256.Sum256(assetBodies["omp-context-promotion-report.v1.json"])
	attestationDigest := sha256.Sum256(assetBodies["omp-context-promotion-attestation.v2.json"])
	fixture := &currentReleaseFixture{
		root: root, state: state, output: filepath.Join(root, "verified-checksums.txt"),
		signatureLog: filepath.Join(state, "signature.log"),
		verifierLog:  filepath.Join(state, "verifier.log"), checksums: assetBodies["checksums.txt"],
		reportSHA256: fmt.Sprintf("%x", reportDigest), attestationSHA256: fmt.Sprintf("%x", attestationDigest),
		release: currentReleaseDocument{ID: 410118, TagName: "v0.50.118",
			TargetCommitish: strings.Repeat("c", 40), Immutable: true, Assets: assets},
	}
	fixture.release.Author.ID = 204883817
	fixture.writeRelease(t)
	fixture.writeCommandMocks(t)
	return fixture
}

func makeCurrentReleaseArchive(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	order := []string{"auto", "adk-companion-manifest.json", "adk-companion-manifest.sig",
		"adk-companion-public-key-receipt.bundle/public-key-receipt.json",
		"adk-companion-public-key-receipt.bundle/public-key-receipt.sig",
		"release-lineage-v1.json", "release-lineage-v1.sig"}
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range order {
		body := entries[name]
		header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func (f *currentReleaseFixture) asset(name string) *currentReleaseAsset {
	for index := range f.release.Assets {
		if f.release.Assets[index].Name == name {
			return &f.release.Assets[index]
		}
	}
	panic("missing release asset: " + name)
}

func (f *currentReleaseFixture) replaceAsset(name string, body []byte) {
	asset := f.asset(name)
	digest := sha256.Sum256(body)
	asset.Size = len(body)
	asset.Digest = fmt.Sprintf("sha256:%x", digest)
	if err := os.WriteFile(filepath.Join(f.state, "assets", fmt.Sprint(asset.ID)), body, 0o600); err != nil {
		panic(err)
	}
}

func (f *currentReleaseFixture) writeRelease(t *testing.T) {
	t.Helper()
	data, err := json.Marshal(f.release)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.state, "release.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *currentReleaseFixture) run() (string, error) {
	script := filepath.Join(repositoryRootForBridge(), "scripts/companion-release/verify-current-release.sh")
	command := exec.Command("bash", script, f.output)
	if err := f.writeSignatureMocks(); err != nil {
		return "", err
	}
	command.Env = []string{
		"PATH=" + filepath.Join(f.root, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + f.root, "TMPDIR=" + f.root, "GITHUB_TOKEN=fixture-token",
		"GH_TOKEN=fixture-fallback-token", "COMPANION_RELEASE_ID=410118",
		"COMPANION_SOURCE_COMMIT=" + strings.Repeat("c", 40),
		"COMPANION_SOURCE_TREE=" + strings.Repeat("d", 40),
		"OMP_CONTEXT_EVIDENCE_REPORT_SHA256=" + f.reportSHA256,
		"OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256=" + f.attestationSHA256,
		"OMP_CONTEXT_STATIC_POLICY_B64=fixture_policy_k3",
		"OMP_CONTEXT_EVIDENCE_VERIFIER=" + filepath.Join(f.root, "bin", "omp-context-evidence-verifier"),
		"OMP_CONTEXT_LINEAGE_VERIFIER=" + filepath.Join(f.root, "bin", "omp-context-lineage-verifier"),
		"COMPANION_MANIFEST_VERIFIER=" + filepath.Join(f.root, "bin", "companion-manifest-verifier"),
		"COMPANION_KEY_ID=fixture-release-key", "COMPANION_HANDOFF=v1",
		"COMPANION_ROLLBACK_FLOOR=5069", "COMPANION_PUBLIC_KEY_SHA256=sha256:" + strings.Repeat("e", 64),
		"MOCK_CURRENT_RELEASE_STATE=" + f.state,
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

func (f *currentReleaseFixture) writeCommandMocks(t *testing.T) {
	t.Helper()
	bin := filepath.Join(f.root, "bin")
	ghMock := `#!/usr/bin/env bash
set -euo pipefail
[[ "${1-}" == api ]]; shift
endpoint=''
while (($#)); do case "$1" in -H) shift 2 ;; *) endpoint=$1; shift ;; esac; done
case "$endpoint" in
  repos/autopus-ai/autopus-adk/releases/tags/v0.50.118) exec cat "$MOCK_CURRENT_RELEASE_STATE/release.json" ;;
  repos/autopus-ai/autopus-adk/releases/assets/*) exec cat "$MOCK_CURRENT_RELEASE_STATE/assets/${endpoint##*/}" ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(ghMock), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"omp-context-lineage-verifier", "companion-manifest-verifier"} {
		body := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
[[ -z "${GITHUB_TOKEN+x}" && -z "${GH_TOKEN+x}" ]]
printf '%%s %%s\n' "${0##*/}" "$*" >> %q
`, f.verifierLog)
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	evidenceVerifier := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
[[ -z "${GITHUB_TOKEN+x}" && -z "${GH_TOKEN+x}" ]]
printf '%%s %%s\n' "${0##*/}" "$*" >> %q
if [[ "$*" == *"--mode historical"* && %t == true ]]; then exit 93; fi
`, f.verifierLog, f.evidenceHistoricalFail)
	if err := os.WriteFile(filepath.Join(bin, "omp-context-evidence-verifier"),
		[]byte(evidenceVerifier), 0o700); err != nil {
		t.Fatal(err)
	}
}
