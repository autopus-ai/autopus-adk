package companionmanifest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type upgradeCanaryStep struct {
	Name string         `yaml:"name"`
	Uses string         `yaml:"uses"`
	With map[string]any `yaml:"with"`
	Env  map[string]any `yaml:"env"`
	Run  string         `yaml:"run"`
}

type upgradeCanaryJob struct {
	Permissions map[string]string   `yaml:"permissions"`
	RunsOn      string              `yaml:"runs-on"`
	Steps       []upgradeCanaryStep `yaml:"steps"`
}

type upgradeCanaryWorkflow struct {
	On          map[string]recoveryDispatch `yaml:"on"`
	Permissions map[string]string           `yaml:"permissions"`
	Jobs        map[string]upgradeCanaryJob `yaml:"jobs"`
}

func readUpgradeCanaryWorkflow(t *testing.T) (string, upgradeCanaryWorkflow) {
	t.Helper()
	raw := readReleaseFile(t, ".github/workflows/upgrade-canary.yaml")
	var workflow upgradeCanaryWorkflow
	if err := yaml.Unmarshal([]byte(raw), &workflow); err != nil {
		t.Fatalf("parse upgrade canary workflow: %v", err)
	}
	return raw, workflow
}

func upgradeCanaryStepNamed(t *testing.T, workflow upgradeCanaryWorkflow, name string) upgradeCanaryStep {
	t.Helper()
	for _, step := range workflow.Jobs["public-asset-admission"].Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("upgrade canary step %q is missing", name)
	return upgradeCanaryStep{}
}

func TestUpgradeCanary_ManualExactPublicSignedA34ToA35(t *testing.T) {
	raw, workflow := readUpgradeCanaryWorkflow(t)
	dispatch, ok := workflow.On["workflow_dispatch"]
	if len(workflow.On) != 1 || !ok || len(dispatch.Inputs) != 0 {
		t.Fatalf("upgrade canary trigger is not input-free workflow_dispatch: %#v", workflow.On)
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatalf("upgrade canary permissions = %#v, want contents: read", workflow.Permissions)
	}
	job := workflow.Jobs["public-asset-admission"]
	if job.RunsOn != "macos-15" || len(job.Permissions) != 0 {
		t.Fatalf("upgrade canary job boundary = runner %q permissions %#v", job.RunsOn, job.Permissions)
	}
	admission := upgradeCanaryStepNamed(t, workflow, "Admit exact public signed A34 to A35 upgrade")
	for _, required := range []string{
		"posix-upgrade-canary.sh live 0.50.123 0.50.124",
		`.previous_public_version == "0.50.123"`,
		`.candidate_public_version == "0.50.124"`,
		`.actual_release_executables_verified == true`,
		`.public_signed_installers_verified == true`,
	} {
		if !strings.Contains(admission.Run, required) {
			t.Fatalf("upgrade canary admission missing %q", required)
		}
	}
	for _, forbidden := range []string{"${{ inputs.", "github.event.inputs", "PREVIOUS_VERSION", "CANDIDATE_VERSION"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("upgrade canary retains variable version authority %q", forbidden)
		}
	}
	if len(admission.Env) != 2 ||
		admission.Env["UPGRADE_CANARY_RECEIPT_DIR"] == nil ||
		admission.Env["UPGRADE_CANARY_GITHUB_TOKEN"] != "${{ github.token }}" {
		t.Fatalf("upgrade canary admission env = %#v", admission.Env)
	}
}

func TestUpgradeCanary_BindsAvailablePublicPredecessorAndCandidateProvenance(t *testing.T) {
	_, workflow := readUpgradeCanaryWorkflow(t)
	step := upgradeCanaryStepNamed(t, workflow, "Bind public release and workflow provenance")
	for _, required := range []string{
		"releases/402971620",
		"8ac711f82b6879b4bbf582de88658315b037a1b7",
		"c447badc28e393b19984d2eeea159a81d609acd9",
		"13dcb08d81c59ad0677e824b0785bc7e1fffcbe7",
		"sha256:4c8809fda4bede93601711892fc91cbcc334cf59a7e0200848b3d4d9b16b2d94",
		"releases/tags/v0.50.124", "git/ref/tags/v0.50.123",
		`schema_version:"autopus-upgrade-canary-provenance.v1"`,
		"GITHUB_WORKFLOW_REF", "GITHUB_WORKFLOW_SHA", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT",
	} {
		if !strings.Contains(step.Run, required) {
			t.Fatalf("upgrade canary provenance missing %q", required)
		}
	}
	if len(step.Env) != 2 || step.Env["GITHUB_TOKEN"] != "${{ github.token }}" || step.Env["RECEIPT_DIR"] == nil {
		t.Fatalf("upgrade canary provenance env = %#v", step.Env)
	}
	for _, unavailable := range []string{"aws s3", "gs://", "azure", "rekor upload", "cosign attest"} {
		if strings.Contains(strings.ToLower(step.Run), unavailable) {
			t.Fatalf("upgrade canary invents external receipt authority %q", unavailable)
		}
	}
}

func TestUpgradeCanary_PinsActionsAndUploadsDiagnosticReceipt(t *testing.T) {
	raw, workflow := readUpgradeCanaryWorkflow(t)
	job := workflow.Jobs["public-asset-admission"]
	if len(job.Steps) != 4 {
		t.Fatalf("upgrade canary step count = %d, want 4", len(job.Steps))
	}
	if job.Steps[0].Uses != "actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10" ||
		job.Steps[0].With["persist-credentials"] != false {
		t.Fatalf("upgrade canary checkout = %#v", job.Steps[0])
	}
	upload := upgradeCanaryStepNamed(t, workflow, "Upload public asset admission receipt")
	if upload.Uses != "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02" ||
		upload.With["name"] != "upgrade-canary-0.50.123-to-0.50.124" || upload.With["retention-days"] != 30 {
		t.Fatalf("upgrade canary receipt upload = %#v", upload)
	}
	if !strings.Contains(raw, "not a\n      # substitute for the immutable public release") {
		t.Fatal("upgrade canary receipt does not disclaim immutable authority")
	}
}

func TestUpgradeCanary_ModelsPublicV109WithoutManagedOMPConfig(t *testing.T) {
	raw := readReleaseFile(t, "scripts/release-signing/tests/posix-upgrade-canary.sh")
	absence := `assert_absent "$PROJECT/.omp/config.yml"`
	creation := `printf 'theme:\n  dark: canary-user\n' > "$PROJECT/.omp/config.yml"`
	if !strings.Contains(raw, absence) {
		t.Fatalf("upgrade canary does not verify the v0.50.123 OMP config absence")
	}
	if !strings.Contains(raw, creation) {
		t.Fatalf("upgrade canary does not create the user-owned OMP config fixture")
	}
	if strings.Index(raw, absence) > strings.Index(raw, creation) {
		t.Fatal("upgrade canary verifies the legacy fixture after mutating it")
	}
	if strings.Contains(raw, `cat "$PROJECT/.omp/config.yml"`) {
		t.Fatal("upgrade canary still reads the absent v0.50.123 OMP config")
	}
	if !strings.Contains(
		raw,
		`AUTOPUS_GITHUB_TOKEN=${UPGRADE_CANARY_GITHUB_TOKEN:-} "$AUTO" update`,
	) {
		t.Fatal("upgrade canary does not scope authenticated freshness lookup to candidate update")
	}
}
