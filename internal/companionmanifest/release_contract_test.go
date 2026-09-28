package companionmanifest

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflow_ExactA30ProtectedNormalLane(t *testing.T) {
	release := readReleaseFile(t, ".github/workflows/release.yaml")
	for _, required := range []string{
		"- 'v0.50.119'", "if: github.ref == 'refs/tags/v0.50.119'",
		"needs: [ci, security, omp-production-evidence]", "adk-companion-release",
		"Validate exact R2-signed A30 source", "COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED=1",
		"Verify exact sealed release-tag authority",
		"scripts/companion-release/verify-release-tag-ruleset.sh --sealed-runtime",
		"Build canonical policy-bearing production candidate", "OMP_CONTEXT_STATIC_POLICY_B64",
		"Verify fresh K3-signed production evidence",
		"Reverify active K3 production evidence inside protected environment",
		"name: omp-context-evidence-v0.50.119", "--mode active",
		"autopus.adk_release_reservation.v1", "release_tag:\"v0.50.119\"",
		`(.assets | type == "array" and length == 0)`, ".author.id == 204883817",
		"Verify reserved release was published", ".immutable == true",
		"COMPANION_RELEASE_ID: ${{ steps.release-reservation.outputs.release-id }}",
		"OMP_CONTEXT_LINEAGE_VERIFIER=\"$OMP_CONTEXT_LINEAGE_VERIFIER\"",
	} {
		if !strings.Contains(release, required) {
			t.Fatalf("A30 release workflow missing %q", required)
		}
	}
	protectedSource := strings.Index(release, "name: Validate exact protected release source")
	sealedAuthority := strings.Index(release, "name: Verify exact sealed release-tag authority")
	reservation := strings.Index(release, "name: Verify operator-owned empty draft release reservation")
	signingCredentials := strings.Index(release, "name: Prepare release credentials")
	publish := strings.Index(release, "goreleaser release --clean")
	if protectedSource < 0 || sealedAuthority <= protectedSource || reservation <= sealedAuthority ||
		signingCredentials <= reservation || publish <= signingCredentials {
		t.Fatalf("sealed authority gate order = source:%d sealed:%d reservation:%d credentials:%d publish:%d",
			protectedSource, sealedAuthority, reservation, signingCredentials, publish)
	}
	if strings.Count(release, "--mode active") != 2 {
		t.Fatalf("active evidence gate count = %d, want pre-protected and protected gates",
			strings.Count(release, "--mode active"))
	}
	for _, forbidden := range []string{
		"v0.50.118", "omp-canonical-bridge-candidate", "omp-context-bridge-release.v1.json",
		"adk-key-rotation-v1.json", "adk-key-rotation-v1.sig", "--expected-signing-key-id",
		"releases/assets/${asset_id}", "--armed", "- 'v*'", "- v*",
	} {
		if strings.Contains(release, forbidden) {
			t.Fatalf("A30 normal release workflow retains forbidden contract %q", forbidden)
		}
	}
}

func TestReleaseWorkflow_ExactFifteenAssetSet(t *testing.T) {
	release := readReleaseFile(t, ".github/workflows/release.yaml")
	assets := []string{
		"autopus-adk_0.50.119_darwin_amd64.tar.gz",
		"autopus-adk_0.50.119_darwin_arm64.tar.gz",
		"autopus-adk_0.50.119_linux_amd64.tar.gz",
		"autopus-adk_0.50.119_linux_arm64.tar.gz",
		"autopus-adk_0.50.119_windows_amd64.tar.gz",
		"autopus-adk_0.50.119_windows_amd64.zip",
		"autopus-adk_0.50.119_windows_arm64.tar.gz",
		"autopus-adk_0.50.119_windows_arm64.zip",
		"checksums.txt", "checksums.txt.bundle", "checksums.txt.signatures",
		"omp-context-promotion-report.v1.json",
		"omp-context-promotion-attestation.v2.json",
		"release-lineage-v1.json", "release-lineage-v1.sig",
	}
	publishedIndex := strings.Index(release, "name: Verify reserved release was published")
	if publishedIndex < 0 {
		t.Fatal("immutable release verification gate is missing")
	}
	publishedGate := release[publishedIndex:]
	for _, asset := range assets {
		if !strings.Contains(publishedGate, asset) {
			t.Fatalf("immutable release gate omits asset %q", asset)
		}
	}
	if !strings.Contains(publishedGate, "length == ($expected | length)") ||
		!strings.Contains(publishedGate, "[.assets[].name] | sort") {
		t.Fatal("immutable release gate does not enforce exact asset equality")
	}
}

func TestReleaseWorkflow_UsesOnlyImmutableActions(t *testing.T) {
	immutable := regexp.MustCompile(`^[^@[:space:]]+@[0-9a-f]{40}$`)
	for _, name := range []string{
		".github/workflows/release.yaml", ".github/workflows/ci.yaml", ".github/workflows/security.yml",
	} {
		var workflow struct {
			Jobs map[string]struct {
				Uses  string `yaml:"uses"`
				Steps []struct {
					Uses string `yaml:"uses"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal([]byte(readReleaseFile(t, name)), &workflow); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for jobName, job := range workflow.Jobs {
			if job.Uses != "" && !strings.HasPrefix(job.Uses, "./") && !immutable.MatchString(job.Uses) {
				t.Fatalf("%s job %s uses mutable action %q", name, jobName, job.Uses)
			}
			for _, step := range job.Steps {
				if step.Uses != "" && !strings.HasPrefix(step.Uses, "./") && !immutable.MatchString(step.Uses) {
					t.Fatalf("%s uses mutable action %q", name, step.Uses)
				}
			}
		}
	}
}

func TestGoReleaser_UsesValidatedSourceAndEmptyDraft(t *testing.T) {
	config := readReleaseFile(t, ".goreleaser.yaml")
	for _, required := range []string{
		`target_commitish: "{{ .Env.COMPANION_SOURCE_COMMIT }}"`,
		"use_existing_draft: true", "replace_existing_artifacts: false", "mode: replace",
		"pipelineOMPActiveStaticPolicyB64={{.Env.OMP_CONTEXT_STATIC_POLICY_B64}}",
		`glob: "{{ .Env.OMP_CONTEXT_PROMOTION_REPORT_PATH }}"`,
		`glob: "{{ .Env.OMP_CONTEXT_PROMOTION_ATTESTATION_PATH }}"`,
	} {
		if !strings.Contains(config, required) {
			t.Fatalf("GoReleaser A23 contract missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"replace_existing_artifacts: true", "OMP_CONTEXT_BRIDGE_MANIFEST_PATH",
		"ADK_KEY_ROTATION_DOCUMENT_PATH", "ADK_KEY_ROTATION_SIGNATURE_PATH",
	} {
		if strings.Contains(config, forbidden) {
			t.Fatalf("GoReleaser can replace or publish forbidden assets via %q", forbidden)
		}
	}
	workflow := readReleaseFile(t, ".github/workflows/release.yaml")
	if !strings.Contains(workflow, `COMPANION_SOURCE_COMMIT="${{ steps.release-source.outputs.source-commit }}"`) {
		t.Fatal("GoReleaser source commit is not the validated forty-hex output")
	}
	if !strings.Contains(workflow, `version: "v2.17.0"`) {
		t.Fatal("GoReleaser binary is not pinned to one exact version")
	}
}

func TestReleaseSourceValidator_PreservesImmutableAncestry(t *testing.T) {
	source := readReleaseFile(t, "scripts/companion-release/validate-source.sh")
	declarations := []string{
		"readonly A2_A1_ANCESTOR_SHA='e25e8be02b55b9385f58919c30ad1ccf92179030'",
		"readonly A2_MAIN_ANCESTOR_SHA='acb735cca0ef120cfed0d01863de09535310b5a3'",
		"readonly A3_A2_ANCESTOR_SHA='7b5b52822b0cda75bf6c971f5f1c2a713881008c'",
		"readonly A4_A3_ANCESTOR_SHA='ba5509b692a43dc8a70e0bd6173acb56166ed67f'",
		"readonly A5_A4_ANCESTOR_SHA='334b297f05942accbecdfa15b54e38e005c82f2d'",
		"readonly A6_A5_ANCESTOR_SHA='b27252cb1148192a8ae1a95195c50e5f221453a4'",
		"readonly A7_A6_ANCESTOR_SHA='902f1acfa91f1d0a2ac9471d5cd79117031a2599'",
		"readonly A8_A7_ANCESTOR_SHA='51de6030a69a8e36fcf7e5790ef157eff6fedf00'",
		"readonly A9_A8_ANCESTOR_SHA='dd0c2759ed5435d4634011e349caad62ea3df414'",
		"readonly A23_A22_ANCESTOR_SHA='67f3def5d4a0a11aadd9e103389de6cc1cafc34e'",
	}
	for _, declaration := range declarations {
		if strings.Count(source, declaration) != 1 {
			t.Fatalf("immutable ancestry pin drifted: %s", declaration)
		}
	}
	for _, required := range []string{
		`git cat-file -t "refs/tags/$GITHUB_REF_NAME"`, `[[ "$tag_object_type" == 'tag' ]]`,
		`git merge-base --is-ancestor "$A23_A22_ANCESTOR_SHA" "$GITHUB_SHA"`,
		"COMPANION_APPROVED_SOURCE_COMMIT", "COMPANION_APPROVED_SOURCE_TREE",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("release source gate missing %q", required)
		}
	}
	r2Fingerprint := strings.TrimSpace(readReleaseFile(t,
		"scripts/companion-release/release-tag-signing-2026-q3-r2.fingerprint"))
	if r2Fingerprint != "SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ" {
		t.Fatalf("R2 release-tag signer fingerprint = %q", r2Fingerprint)
	}
}

func TestReleaseWorkflow_HomebrewRunsOnlyAfterImmutableEvidence(t *testing.T) {
	release := readReleaseFile(t, ".github/workflows/release.yaml")
	releaseIndex := strings.Index(release, "goreleaser release --clean")
	publishGateIndex := strings.Index(release, "name: Verify reserved release was published")
	signingCleanupIndex := strings.Index(release, "name: Remove release signing credentials")
	evidenceIndex := strings.Index(release, "scripts/companion-release/verify-current-release.sh")
	tokenIndex := strings.Index(release, "name: Create Homebrew tap token")
	publishIndex := strings.Index(release, "scripts/companion-release/publish-homebrew-formula-bridge.sh")
	cleanupIndex := strings.Index(release, "Remove release credentials and keychain")
	if releaseIndex < 0 || publishGateIndex <= releaseIndex || signingCleanupIndex <= publishGateIndex ||
		evidenceIndex <= signingCleanupIndex || tokenIndex <= evidenceIndex ||
		publishIndex <= tokenIndex || cleanupIndex <= publishIndex {
		t.Fatalf("unsafe Homebrew order: %d %d %d %d %d %d %d", releaseIndex,
			publishGateIndex, signingCleanupIndex, evidenceIndex, tokenIndex, publishIndex, cleanupIndex)
	}
	if strings.Contains(release, "COMPANION_CHECKSUMS_PATH='dist/checksums.txt'") {
		t.Fatal("Homebrew publication can consume unverified local checksums")
	}
}
