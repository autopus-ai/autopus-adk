package companionmanifest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const exactGoReleaserModule = "github.com/goreleaser/goreleaser/v2@v2.17.0"

type executableLineageAsset struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Digest string `json:"digest"`
}

type executableLineageRelease struct {
	TagName         string                   `json:"tag_name"`
	TargetCommitish string                   `json:"target_commitish"`
	Draft           bool                     `json:"draft"`
	Prerelease      bool                     `json:"prerelease"`
	Immutable       bool                     `json:"immutable"`
	Assets          []executableLineageAsset `json:"assets"`
}

func produceGoReleaserA0FixtureEvidence(
	t *testing.T,
	tools executableLineageTools,
) *goReleaserA0Evidence {
	return produceGoReleaserFixtureEvidence(t, tools, publicKeyReceiptA0Tag, lineageA0Version, false)
}

func produceGoReleaserA1FixtureEvidence(
	t *testing.T,
	tools executableLineageTools,
) *goReleaserA0Evidence {
	return produceGoReleaserFixtureEvidence(t, tools, publicKeyReceiptA1Tag, lineageA1Version, true)
}

func produceUncachedGoReleaserFixtureEvidence(
	t *testing.T,
	tools executableLineageTools,
	releaseTag, releaseVersion string,
	annotated bool,
) *goReleaserA0Evidence {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repository")
	copyLineageRepository(t, root)
	runLineageCommand(t, root, "git", "init", "-q")
	runLineageCommand(t, root, "git", "config", "user.name", "F07 Release Test")
	runLineageCommand(t, root, "git", "config", "user.email", "f07@example.invalid")
	runLineageCommand(t, root, "git", "remote", "add", "origin", "https://example.invalid/Insajin/autopus-adk.git")
	runLineageCommand(t, root, "git", "add", ".")
	runLineageCommand(t, root, "git", "commit", "-qm", "F07 exact GoReleaser lineage fixture")
	if annotated {
		runLineageCommand(t, root, "git", "tag", "-am", "F07 annotated predecessor", releaseTag)
	} else {
		runLineageCommand(t, root, "git", "tag", releaseTag)
	}
	commit := strings.TrimSpace(runLineageCommand(t, root, "git", "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runLineageCommand(t, root, "git", "rev-parse", "HEAD^{tree}"))
	tagObject := ""
	if annotated {
		tagObject = strings.TrimSpace(runLineageCommand(t, root, "git", "rev-parse", releaseTag))
	}

	credentialRoot := filepath.Join(filepath.Dir(root), "credentials")
	if err := os.Mkdir(credentialRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeReleaseCanaryFixture(t, credentialRoot)
	seed := sha256.Sum256([]byte("autopus-f07-lineage-ed25519-seed"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	keyPath := filepath.Join(credentialRoot, "release-key")
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(privateKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	tmpDir := filepath.Join(filepath.Dir(root), "tmp")
	if err := os.Mkdir(tmpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	toolEnv := darwinReleaseToolEnv(t, credentialRoot)
	evidenceDir := filepath.Join(root, ".autopus", "runtime", "release-evidence")
	if err := os.MkdirAll(evidenceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bridgeManifestPath := filepath.Join(".autopus", "runtime", "release-evidence",
		"omp-context-bridge-release.v1.json")
	rotationDocumentPath := filepath.Join(".autopus", "runtime", "release-evidence",
		"adk-key-rotation-v1.json")
	rotationSignaturePath := filepath.Join(".autopus", "runtime", "release-evidence",
		"adk-key-rotation-v1.sig")
	for path, body := range map[string][]byte{
		bridgeManifestPath:    []byte("{}\n"),
		rotationDocumentPath:  []byte("{}"),
		rotationSignaturePath: []byte(strings.Repeat("A", 64)),
	} {
		if err := os.WriteFile(filepath.Join(root, path), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	environment := append(os.Environ(), toolEnv...)
	environment = append(environment,
		"GO_WANT_COMPANION_SIGNER_HELPER=1", "TMPDIR="+tmpDir,
		"GITHUB_REF_NAME="+releaseTag, "COMPANION_SOURCE_COMMIT="+commit,
		"COMPANION_SOURCE_TREE="+tree,
		"OMP_CONTEXT_BRIDGE_MANIFEST_PATH="+bridgeManifestPath,
		"ADK_KEY_ROTATION_DOCUMENT_PATH="+rotationDocumentPath,
		"ADK_KEY_ROTATION_SIGNATURE_PATH="+rotationSignaturePath,
		"OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256="+strings.Repeat("0", 64),
		"OMP_CONTEXT_STATIC_POLICY_B64=e30",
		"OMP_CONTEXT_RELEASE_CANARY_ROOT="+filepath.Join(credentialRoot, "omp-release-canary-root"),
		"OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE="+filepath.Join(credentialRoot, "omp-release-canary-root", "omp-darwin-arm64"),
		"COMPANION_BUILD_PROVENANCE=github-actions:Insajin/autopus-adk@"+commit,
		"COMPANION_HANDOFF="+lineageHandoff, "COMPANION_ROLLBACK_FLOOR=5069",
		"COMPANION_ISSUED_AT=2026-07-15T00:00:00Z",
		"COMPANION_EXPIRES_AT=2026-07-16T00:00:00Z", "COMPANION_KEY_ID="+lineageKeyID,
		"COMPANION_SIGNING_KEY_FILE="+keyPath, "COMPANION_SIGNER="+tools.signer,
		"COMPANION_RECEIPT_VERIFIER="+tools.verifier,
		"COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT="+lineageIssuedAt,
		"COMPANION_PUBLIC_KEY_RECEIPT_EXPIRES_AT="+lineageExpiresAt,
		"COMPANION_PUBLIC_KEY_RECEIPT_MINIMUM_LIFETIME_SECONDS=31536000",
		"HOMEBREW_TAP_TOKEN=fixture-token",
	)
	args := []string{"run", exactGoReleaserModule, "release", "--clean", "--parallelism=2",
		"--skip=announce,publish,sign,homebrew"}
	command := exec.Command("go", args...)
	command.Dir, command.Env = root, environment
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact GoReleaser %s output: %v\n%s", releaseTag, err, output)
	}

	evidence := &goReleaserA0Evidence{
		tag: releaseTag, version: releaseVersion, commit: commit, archives: map[string]string{},
	}
	evidence.pins.tagObject = tagObject
	cacheDirectory := filepath.Join(lineageFixtureCacheRoot, releaseTag)
	if err := os.Mkdir(cacheDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheGoReleaserLineageArchives(t, root, cacheDirectory, evidence)
	evidence.checksums = readLineageFile(t, filepath.Join(root, "dist", "checksums.txt"))
	evidence.pins = captureGoReleaserPins(t, evidence, privateKey, tagObject)
	evidence.pins.tree = tree
	return evidence
}

func captureGoReleaserPins(
	t *testing.T,
	evidence *goReleaserA0Evidence,
	privateKey ed25519.PrivateKey,
	tagObject string,
) executableLineagePins {
	t.Helper()
	receiptDigest, signatureDigest := lineageDigest(evidence.receipt), lineageDigest(evidence.signature)
	pins := executableLineagePins{
		commit: evidence.commit, tagObject: tagObject,
		receipt: receiptDigest, signature: signatureDigest,
		record:        lineageRecordDigest(receiptDigest, signatureDigest),
		publicKey:     lineageDigest(privateKey.Public().(ed25519.PublicKey)),
		checksums:     lineageDigest(evidence.checksums),
		amd64Manifest: evidence.pins.amd64Manifest,
		arm64Manifest: evidence.pins.arm64Manifest,
	}
	captureGoReleaserArchivePins(t, evidence, &pins)
	return pins
}

func (fixture *executableLineageFixture) writeMockGitHub(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(fixture.mockToolsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixture.mockToolsDir, "gh")
	source := `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1-}" == 'api' ]]; then
  case "${2-}" in
    */releases/tags/*) exec /bin/cat ` + shellQuote(fixture.releaseJSON) + ` ;;
    */git/ref/tags/*) exec /bin/cat ` + shellQuote(fixture.tagJSON) + ` ;;
    */git/tags/*) exec /bin/cat ` + shellQuote(fixture.annotatedTagJSON) + ` ;;
    */commits/*) exec /bin/cat ` + shellQuote(fixture.commitJSON) + ` ;;
    *) exit 64 ;;
  esac
fi
if [[ "${1-}" == 'release' && "${2-}" == 'download' ]]; then
  pattern=''; destination=''; shift 2
  while (($#)); do
    case "$1" in
      --pattern) pattern="$2"; shift 2 ;;
      --dir) destination="$2"; shift 2 ;;
      *) shift ;;
    esac
  done
  [[ -n "$pattern" && -n "$destination" ]] || exit 64
  exec /bin/cp ` + shellQuote(fixture.assetsDir) + `/"$pattern" "$destination/$pattern"
fi
exit 64
`
	if err := os.WriteFile(path, []byte(source), 0o700); err != nil {
		t.Fatal(err)
	}
}

func (fixture *executableLineageFixture) writeProvisionedProductionScript(t *testing.T) {
	t.Helper()
	source := string(releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage.sh"))
	pinsSource := string(releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage-pins.sh"))
	replacements := map[string]string{
		"A0_COMMIT_SHA": fixture.pins.commit, "A0_RECEIPT_SHA256": fixture.pins.receipt,
		"A0_SIGNATURE_SHA256": fixture.pins.signature, "A0_RECORD_SHA256": fixture.pins.record,
		"A0_PUBLIC_KEY_SHA256": fixture.pins.publicKey, "A0_CHECKSUMS_SHA256": fixture.pins.checksums,
		"A0_AMD64_MANIFEST_SHA256": fixture.pins.amd64Manifest,
		"A0_ARM64_MANIFEST_SHA256": fixture.pins.arm64Manifest,
	}
	for name, value := range directPredecessorPinReplacements(fixture) {
		replacements[name] = value
	}
	for name, value := range replacements {
		productionValue, ok := immutableProductionLineagePin(name)
		if !ok {
			t.Fatalf("production lineage pin %s is not defined", name)
		}
		production := "readonly " + name + "='" + productionValue + "'"
		if strings.Count(pinsSource, production) != 1 {
			t.Fatalf("production lineage pin declaration %s is not exact", name)
		}
		pinsSource = strings.Replace(pinsSource, production, "readonly "+name+"='"+value+"'", 1)
	}
	if err := os.WriteFile(fixture.provisionedScriptPath, []byte(source), 0o700); err != nil {
		t.Fatal(err)
	}
	pinsPath := filepath.Join(filepath.Dir(fixture.provisionedScriptPath), "verify-public-key-lineage-pins.sh")
	if err := os.WriteFile(pinsPath, []byte(pinsSource), 0o600); err != nil {
		t.Fatal(err)
	}
	shippedPins := releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage-shipped-pins.sh")
	shippedPinsPath := filepath.Join(filepath.Dir(fixture.provisionedScriptPath), "verify-public-key-lineage-shipped-pins.sh")
	if err := os.WriteFile(shippedPinsPath, shippedPins, 0o600); err != nil {
		t.Fatal(err)
	}
	coordinates := releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage-coordinates.sh")
	coordinatesPath := filepath.Join(filepath.Dir(fixture.provisionedScriptPath), "verify-public-key-lineage-coordinates.sh")
	if err := os.WriteFile(coordinatesPath, coordinates, 0o600); err != nil {
		t.Fatal(err)
	}
	helper := releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage-archive.sh")
	helperPath := filepath.Join(filepath.Dir(fixture.provisionedScriptPath), "verify-public-key-lineage-archive.sh")
	if err := os.WriteFile(helperPath, helper, 0o600); err != nil {
		t.Fatal(err)
	}
	assetsHelper := releaseSourceFile(t, "scripts/companion-release/verify-public-key-lineage-assets.sh")
	assetsHelperPath := filepath.Join(filepath.Dir(fixture.provisionedScriptPath), "verify-public-key-lineage-assets.sh")
	if err := os.WriteFile(assetsHelperPath, assetsHelper, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeLineageJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
