package companionmanifest

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const formulaRecoveryWorkflow = ".github/workflows/homebrew-formula-bridge-recovery.yaml"

type recoveryDispatch struct {
	Inputs map[string]any `yaml:"inputs"`
}

type recoveryStep struct {
	Name string         `yaml:"name"`
	Uses string         `yaml:"uses"`
	With map[string]any `yaml:"with"`
	Env  map[string]any `yaml:"env"`
	Run  string         `yaml:"run"`
}

type recoveryJob struct {
	Environment struct {
		Name string `yaml:"name"`
	} `yaml:"environment"`
	Env         map[string]any    `yaml:"env"`
	Permissions map[string]string `yaml:"permissions"`
	RunsOn      string            `yaml:"runs-on"`
	Steps       []recoveryStep    `yaml:"steps"`
}

type recoveryWorkflow struct {
	On          map[string]recoveryDispatch `yaml:"on"`
	Permissions map[string]string           `yaml:"permissions"`
	Jobs        map[string]recoveryJob      `yaml:"jobs"`
}

func readRecoveryWorkflow(t *testing.T) (string, recoveryWorkflow) {
	t.Helper()
	raw := readReleaseFile(t, formulaRecoveryWorkflow)
	var workflow recoveryWorkflow
	if err := yaml.Unmarshal([]byte(raw), &workflow); err != nil {
		t.Fatalf("parse recovery workflow: %v", err)
	}
	return raw, workflow
}

func recoveryStepNamed(t *testing.T, workflow recoveryWorkflow, name string) recoveryStep {
	t.Helper()
	for _, step := range workflow.Jobs["recover-formula-bridge"].Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("recovery step %q is missing", name)
	return recoveryStep{}
}

func recoveryStepIndex(t *testing.T, workflow recoveryWorkflow, name string) int {
	t.Helper()
	for index, step := range workflow.Jobs["recover-formula-bridge"].Steps {
		if step.Name == name {
			return index
		}
	}
	t.Fatalf("recovery step %q is missing", name)
	return -1
}

func TestFormulaRecoveryWorkflow_ManualExactA35LeastPrivilege(t *testing.T) {
	raw, workflow := readRecoveryWorkflow(t)
	dispatch, ok := workflow.On["workflow_dispatch"]
	if len(workflow.On) != 1 || !ok || len(dispatch.Inputs) != 0 {
		t.Fatalf("recovery trigger is not input-free workflow_dispatch: %#v", workflow.On)
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatalf("global recovery permissions = %#v", workflow.Permissions)
	}
	job, ok := workflow.Jobs["recover-formula-bridge"]
	if len(workflow.Jobs) != 1 || !ok {
		t.Fatalf("recovery jobs = %#v", workflow.Jobs)
	}
	if len(job.Permissions) != 1 || job.Permissions["contents"] != "read" ||
		job.Environment.Name != "adk-companion-release" || job.RunsOn != "macos-14" || len(job.Env) != 0 {
		t.Fatalf("recovery job boundary = %#v", job)
	}
	for _, forbidden := range []string{
		"id-token:", "pull_request:", "repository_dispatch:", "schedule:", "${{ inputs.",
		"ADK_RELEASE_ECDSA_PRIVATE_KEY", "COMPANION_SIGNING_KEY", "COMPANION_SIGNER",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("recovery workflow contains forbidden authority %q", forbidden)
		}
	}
	for _, version := range regexp.MustCompile(`v?[0-9]+\.[0-9]+\.[0-9]+`).FindAllString(raw, -1) {
		if version != "v0.50.124" && version != "v4.1.2" && version != "v3.1.2" {
			t.Fatalf("recovery workflow references non-A35 version %q", version)
		}
	}
	if strings.Count(raw, "v0.50.124") != 2 {
		t.Fatalf("recovery workflow must name v0.50.124 only in invocation guidance and exact job guard")
	}
}

func TestFormulaRecoveryWorkflow_PinsActionsSourceAndR2Tag(t *testing.T) {
	raw, workflow := readRecoveryWorkflow(t)
	job := workflow.Jobs["recover-formula-bridge"]
	wantUses := []string{
		"actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10",
		"sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6",
		"actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16",
		"actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1",
	}
	var gotUses []string
	for _, step := range job.Steps {
		if step.Uses != "" {
			gotUses = append(gotUses, step.Uses)
		}
	}
	if strings.Join(gotUses, "\n") != strings.Join(wantUses, "\n") {
		t.Fatalf("recovery actions = %v, want %v", gotUses, wantUses)
	}
	for _, required := range []string{
		"ref: ${{ github.ref }}", "fetch-depth: 0", "persist-credentials: false",
		"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED=1", "COMPANION_SOURCE_PIN_REQUIRED=1",
		"ADK_COMPANION_APPROVED_SOURCE_COMMIT", "ADK_COMPANION_APPROVED_SOURCE_TREE",
		"scripts/companion-release/validate-source.sh",
	} {
		if !strings.Contains(raw, required) {
			t.Fatalf("recovery source contract missing %q", required)
		}
	}
	if strings.Contains(raw, "ADK_KEY_ROTATION_VERIFIED") || strings.Contains(raw, "materialize-key-rotation-authority") {
		t.Fatal("normal A35 recovery retains the A22 bridge rotation gate")
	}
}

func TestFormulaRecoveryWorkflow_RequiresExactSealedTagAuthorityBeforeTapToken(t *testing.T) {
	_, workflow := readRecoveryWorkflow(t)
	sealed := recoveryStepNamed(t, workflow, "Verify exact sealed release-tag authority")
	if len(sealed.Env) != 1 || sealed.Env["GITHUB_TOKEN"] != "${{ github.token }}" {
		t.Fatalf("sealed tag authority environment = %#v", sealed.Env)
	}
	for _, required := range []string{
		`env -i PATH="$PATH" HOME="$HOME" GITHUB_TOKEN="$GITHUB_TOKEN"`,
		"scripts/companion-release/verify-release-tag-ruleset.sh --sealed-runtime",
	} {
		if !strings.Contains(sealed.Run, required) {
			t.Fatalf("sealed tag authority step missing %q", required)
		}
	}
	ruleset := readReleaseFile(t, "scripts/companion-release/verify-release-tag-ruleset.sh")
	for _, required := range []string{
		`readonly release_ref='refs/tags/v0.50.124'`, `--sealed-runtime) mode=sealed-runtime`,
		`elif $mode == "sealed-runtime"`, `(.bypass_actors == [] or .bypass_actors == null)`,
		`if [[ "$mode" != 'sealed-runtime' ]]`, `["creation","deletion","update"]`,
	} {
		if !strings.Contains(ruleset, required) {
			t.Fatalf("sealed tag authority verifier missing %q", required)
		}
	}
	sealedIndex := recoveryStepIndex(t, workflow, sealed.Name)
	evidenceIndex := recoveryStepIndex(t, workflow, "Verify exact immutable release and historical K3 proof")
	tokenIndex := recoveryStepIndex(t, workflow, "Create Homebrew tap token")
	if sealedIndex >= evidenceIndex || evidenceIndex >= tokenIndex {
		t.Fatalf("sealed/release/token order = %d/%d/%d", sealedIndex, evidenceIndex, tokenIndex)
	}
}

func TestFormulaRecoveryWorkflow_VerifiesExactNormalReleaseBeforeTapCredential(t *testing.T) {
	raw, workflow := readRecoveryWorkflow(t)
	build := recoveryStepNamed(t, workflow, "Build trusted current-release verifiers")
	for _, required := range []string{
		"./scripts/companion-release/ompcontextverify",
		"./scripts/companion-release/ompcontextlineageverify",
		"./scripts/companion-release/manifestverify",
		"chmod 0700", "stat -f '%Lp'",
	} {
		if !strings.Contains(build.Run, required) {
			t.Fatalf("recovery verifier build missing %q", required)
		}
	}
	evidence := recoveryStepNamed(t, workflow, "Verify exact immutable release and historical K3 proof")
	for _, name := range []string{
		"GITHUB_TOKEN", "COMPANION_SOURCE_COMMIT", "COMPANION_SOURCE_TREE",
		"OMP_CONTEXT_EVIDENCE_REPORT_SHA256", "OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256",
		"OMP_CONTEXT_STATIC_POLICY_B64", "COMPANION_KEY_ID", "COMPANION_HANDOFF",
		"COMPANION_ROLLBACK_FLOOR", "COMPANION_PUBLIC_KEY_SHA256",
	} {
		if evidence.Env[name] == nil {
			t.Fatalf("recovery evidence env missing %s: %#v", name, evidence.Env)
		}
	}
	for _, required := range []string{
		`releases/tags/$GITHUB_REF_NAME`, `.id | select(type == "number" and . > 0)`,
		`COMPANION_RELEASE_ID="$release_id"`, "OMP_CONTEXT_EVIDENCE_VERIFIER=",
		"OMP_CONTEXT_LINEAGE_VERIFIER=", "COMPANION_MANIFEST_VERIFIER=",
		`scripts/companion-release/verify-current-release.sh "$checksums_path"`,
	} {
		if !strings.Contains(evidence.Run, required) {
			t.Fatalf("recovery immutable gate missing %q", required)
		}
	}
	verifyIndex := recoveryStepIndex(t, workflow, evidence.Name)
	tokenIndex := recoveryStepIndex(t, workflow, "Create Homebrew tap token")
	publishIndex := recoveryStepIndex(t, workflow, "Reconcile Homebrew Cask")
	if verifyIndex >= tokenIndex || tokenIndex >= publishIndex {
		t.Fatalf("recovery verification/token/publication order = %d/%d/%d", verifyIndex, tokenIndex, publishIndex)
	}
	app := recoveryStepNamed(t, workflow, "Create Homebrew tap token")
	if len(app.With) != 5 || app.With["client-id"] != "${{ vars.HOMEBREW_APP_CLIENT_ID }}" ||
		app.With["private-key"] != "${{ secrets.HOMEBREW_APP_PRIVATE_KEY }}" ||
		app.With["owner"] != "Insajin" || app.With["repositories"] != "homebrew-autopus" ||
		app.With["permission-contents"] != "write" {
		t.Fatalf("Homebrew App token scope = %#v, want tap contents:write only", app.With)
	}
	if strings.Contains(raw[:strings.Index(raw, "- name: Create Homebrew tap token")], "HOMEBREW_APP_PRIVATE_KEY") {
		t.Fatal("Homebrew credential is referenced before immutable release verification")
	}
}

func TestFormulaRecoveryWorkflow_SharedVerifierOwnsExact15AssetK3Proof(t *testing.T) {
	helper := readReleaseFile(t, "scripts/companion-release/verify-current-release.sh")
	for _, required := range []string{
		"COMPANION_RELEASE_ID", "OMP_CONTEXT_STATIC_POLICY_B64", "OMP_CONTEXT_EVIDENCE_VERIFIER",
		"--mode historical", "--static-policy-b64",
		"omp-context-promotion-report.v1.json", "omp-context-promotion-attestation.v2.json",
		"release-lineage-v1.json", "release-lineage-v1.sig",
		"checksums.txt", "checksums.txt.bundle", "checksums.txt.signatures",
		"exactly fifteen A35 normal release assets",
	} {
		if !strings.Contains(helper, required) {
			t.Fatalf("shared A35 release verifier missing %q", required)
		}
	}
	for _, platform := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		if !strings.Contains(helper, "autopus-adk_${RELEASE_VERSION}_"+platform+".tar.gz") {
			t.Fatalf("shared A35 verifier missing archive %s", platform)
		}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, extension := range []string{"tar.gz", "zip"} {
			if !strings.Contains(helper, "autopus-adk_${RELEASE_VERSION}_windows_"+arch+"."+extension) {
				t.Fatalf("shared A35 verifier missing windows %s.%s", arch, extension)
			}
		}
	}
	for _, forbidden := range []string{
		"omp-context-bridge-release.v1.json", "adk-key-rotation-v1.json", "adk-key-rotation-v1.sig",
		"--mode active", "--expected-signing-key-id",
	} {
		if strings.Contains(helper, forbidden) {
			t.Fatalf("shared A35 verifier retains forbidden bridge/external authority %q", forbidden)
		}
	}
}
