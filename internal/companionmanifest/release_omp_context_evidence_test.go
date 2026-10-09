package companionmanifest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflow_A35PublishesFreshPolicyOwnedK3Evidence(t *testing.T) {
	release := readReleaseFile(t, ".github/workflows/release.yaml")
	config := readReleaseFile(t, ".goreleaser.yaml")
	current := readReleaseFile(t, "scripts/companion-release/verify-current-release.sh")
	combined := release + "\n" + config + "\n" + current
	for _, required := range []string{
		"refs/tags/v0.50.124",
		"RELEASE_VERSION='0.50.124'",
		"omp-production-evidence",
		"omp-context-evidence-v0.50.124",
		"OMP_CONTEXT_STATIC_POLICY_B64",
		"omp-context-promotion-report.v1.json",
		"omp-context-promotion-attestation.v2.json",
		"--mode active",
		"--mode historical",
		"release-lineage-v1.json",
		"release-lineage-v1.sig",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("A35 normal evidence contract missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"omp-context-bridge-release.v1.json",
		"adk-key-rotation-v1.json",
		"adk-key-rotation-v1.sig",
		"--expected-signing-key-id",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("A35 normal evidence contract contains bridge authority %q", forbidden)
		}
	}
	for _, required := range []string{
		`glob: "{{ .Env.OMP_CONTEXT_PROMOTION_REPORT_PATH }}"`,
		`glob: "{{ .Env.OMP_CONTEXT_PROMOTION_ATTESTATION_PATH }}"`,
		`glob: "{{ .Env.OMP_CONTEXT_RELEASE_LINEAGE_PATH }}"`,
		`glob: "{{ .Env.OMP_CONTEXT_RELEASE_LINEAGE_SIGNATURE_PATH }}"`,
		"dst: release-lineage-v1.json",
		"dst: release-lineage-v1.sig",
	} {
		if !strings.Contains(config, required) {
			t.Fatalf("GoReleaser A35 evidence asset missing %q", required)
		}
	}
	for _, asset := range []string{
		"_darwin_amd64.tar.gz",
		"_darwin_arm64.tar.gz",
		"_linux_amd64.tar.gz",
		"_linux_arm64.tar.gz",
		"_windows_amd64.tar.gz",
		"_windows_amd64.zip",
		"_windows_arm64.tar.gz",
		"_windows_arm64.zip",
		"checksums.txt",
		"omp-context-promotion-report.v1.json",
		"omp-context-promotion-attestation.v2.json",
		"release-lineage-v1.json",
		"release-lineage-v1.sig",
		"checksums.txt.bundle",
		"checksums.txt.signatures",
	} {
		if !strings.Contains(current, asset) {
			t.Fatalf("current A35 verifier missing exact asset %q", asset)
		}
	}
}

func TestReleaseSourceValidator_A22RotationSidecarRemainsHistoricalOnly(t *testing.T) {
	release := readReleaseFile(t, ".github/workflows/release.yaml")
	source := readReleaseFile(t, "scripts/companion-release/validate-source.sh")
	for _, required := range []string{
		`if [[ "$release_phase" == 'A22' &&`,
		`"${COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED-0}" == '1'`,
		"ADK_KEY_ROTATION_VERIFIED",
		"A22 R2 tag verification requires an independently verified rotation sidecar",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("A22 historical source gate missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"refs/tags/v0.50.109",
		"verify-key-rotation-sidecar.sh",
		"canonical-full-bridge",
	} {
		if strings.Contains(release, forbidden) {
			t.Fatalf("A22 bridge authority escaped into active A35 workflow: %q", forbidden)
		}
	}
}

func TestReleaseWorkflow_ProtectedMacOSSelectsHomebrewOpenSSL3BeforeVerificationTools(t *testing.T) {
	release := readReleaseFile(t, ".github/workflows/release.yaml")
	var workflow struct {
		Jobs map[string]struct {
			Env    map[string]any `yaml:"env"`
			RunsOn string         `yaml:"runs-on"`
			Steps  []struct {
				Name string         `yaml:"name"`
				Env  map[string]any `yaml:"env"`
				Run  string         `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(release), &workflow); err != nil {
		t.Fatalf("parse release workflow: %v", err)
	}
	protected := workflow.Jobs["release"]
	if protected.RunsOn != "macos-15" {
		t.Fatalf("protected release runner = %q, want macos-15", protected.RunsOn)
	}
	if len(protected.Env) != 0 {
		t.Fatalf("protected release credentials escaped step scope: %#v", protected.Env)
	}
	selectionIndex, toolsIndex := -1, -1
	for index, step := range protected.Steps {
		switch step.Name {
		case "Select Homebrew OpenSSL 3":
			selectionIndex = index
			assertHomebrewOpenSSL3Selection(t, step.Run, step.Env)
		case "Build companion release verification tools":
			toolsIndex = index
		}
	}
	if selectionIndex < 0 || toolsIndex < 0 || selectionIndex >= toolsIndex {
		t.Fatalf("protected OpenSSL/tool build order = %d/%d", selectionIndex, toolsIndex)
	}
	if strings.Count(release, "- name: Select Homebrew OpenSSL 3") != 1 {
		t.Fatal("Homebrew OpenSSL selection escaped the protected macOS release job")
	}
}

func TestOMPContextEvidenceVerifier_UsesPolicyOwnedTrustAPIs(t *testing.T) {
	source := readReleaseFile(t, "scripts/companion-release/ompcontextverify/main.go")
	for _, required := range []string{
		"promptlayer.VerifyOMPContextPromotionArtifactV2",
		"promptlayer.VerifyOMPContextPromotionHistoricalArtifactV2",
		"promptlayer.MarshalOMPContextPromotionStaticPolicyV3",
		"promptlayer.ValidateOMPContextPromotionActiveStaticPolicyV3",
		"policy.PromotionSigningKeyID",
		"promptlayer.OMPContextPromotionProviderAuthorityDigestV1",
		"policy.ProviderAuthorityDigest",
		"candidate artifact digest differs from runtime binary digest",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("OMP evidence verifier missing %q", required)
		}
	}
	for _, forbidden := range []string{"expectedSigningKeyID", "expected-signing-key-id"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("OMP evidence verifier retains split signer authority %q", forbidden)
		}
	}
}

func TestOMPContextLineageCleanup_RemovesOnlyOwnedEnabledOutputs(t *testing.T) {
	root := repositoryRoot(t)
	lineage := filepath.Join(t.TempDir(), "release-lineage-v1.json")
	signature := filepath.Join(filepath.Dir(lineage), "release-lineage-v1.sig")
	for _, path := range []string{lineage, signature} {
		if err := os.WriteFile(path, []byte("preexisting"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	helper := filepath.Join(root, "scripts", "companion-release", "produce-omp-context-lineage.sh")
	script := `source "$1"
omp_context_lineage_enabled=0 lineage_path=$2 lineage_signature_path=$3
cleanup_omp_context_release_lineage
test -f "$2" && test -f "$3"
omp_context_lineage_enabled=1
cleanup_omp_context_release_lineage
test ! -e "$2" && test ! -e "$3"`
	command := exec.Command("/bin/bash", "-c", script, "bash", helper, lineage, signature)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("lineage cleanup ownership: %v\n%s", err, output)
	}
}

func TestOMPContextEvidenceTagVerifier_NegativeMatrix(t *testing.T) {
	root := repositoryRoot(t)
	contract := filepath.Join(root, "scripts", "companion-release", "tests",
		"release-omp-context-evidence-hardening-test.sh")
	command := exec.Command("/bin/bash", contract)
	command.Dir = root
	command.Env = os.Environ()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OMP context evidence hardening: %v\n%s", err, output)
	}
}
