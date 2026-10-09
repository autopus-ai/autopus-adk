package companionmanifest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Which coordinates the lineage verifier admits is a different question from
// what the source validator accepts, so the phase matrix lives on its own. A0
// bootstraps without prior evidence; every later live phase must reach the
// token check, and every failed or burned coordinate must be refused by the
// frozen policy rather than silently treated as a new phase.
func TestLineageVerifier_A0BootstrapsWhileA1ThroughA35WithoutLiveEvidenceFailClosed(t *testing.T) {
	script := filepath.Join(repositoryRoot(t), "scripts/companion-release/verify-public-key-lineage.sh")
	cases := []struct {
		name    string
		tag     string
		wantOK  bool
		message string
	}{
		{name: "A0", tag: "v0.50.69", wantOK: true, message: "bootstrap accepted"},
		{name: "A1", tag: "v0.50.70", message: "missing GITHUB_TOKEN"},
		{name: "A2", tag: "v0.50.71", message: "missing GITHUB_TOKEN"},
		{name: "A3", tag: "v0.50.72", message: "missing GITHUB_TOKEN"},
		{name: "A4", tag: "v0.50.73", message: "missing GITHUB_TOKEN"},
		{name: "A5", tag: "v0.50.74", message: "missing GITHUB_TOKEN"},
		{name: "A6", tag: "v0.50.77", message: "missing GITHUB_TOKEN"},
		{name: "A7", tag: "v0.50.78", message: "missing GITHUB_TOKEN"},
		{name: "A8", tag: "v0.50.79", message: "missing GITHUB_TOKEN"},
		{name: "A9", tag: "v0.50.80", message: "missing GITHUB_TOKEN"},
		{name: "A10", tag: "v0.50.81", message: "missing GITHUB_TOKEN"},
		{name: "A11", tag: "v0.50.82", message: "missing GITHUB_TOKEN"},
		{name: "A12", tag: "v0.50.83", message: "missing GITHUB_TOKEN"},
		{name: "A13", tag: "v0.50.84", message: "missing GITHUB_TOKEN"},
		{name: "A14", tag: "v0.50.85", message: "missing GITHUB_TOKEN"},
		{name: "A15", tag: "v0.50.86", message: "missing GITHUB_TOKEN"},
		{name: "A16", tag: "v0.50.87", message: "missing GITHUB_TOKEN"},
		{name: "A17", tag: "v0.50.88", message: "missing GITHUB_TOKEN"},
		{name: "A18", tag: "v0.50.89", message: "missing GITHUB_TOKEN"},
		{name: "A19", tag: "v0.50.90", message: "missing GITHUB_TOKEN"},
		{name: "A20", tag: "v0.50.91", message: "missing GITHUB_TOKEN"},
		{name: "A21", tag: "v0.50.92", message: "missing GITHUB_TOKEN"},
		{name: "A22", tag: "v0.50.109", message: "missing GITHUB_TOKEN"},
		{name: "A23", tag: "v0.50.111", message: "missing GITHUB_TOKEN"},
		{name: "A24", tag: "v0.50.113", message: "missing GITHUB_TOKEN"},
		{name: "A25", tag: "v0.50.114", message: "missing GITHUB_TOKEN"},
		{name: "A26", tag: "v0.50.115", message: "missing GITHUB_TOKEN"},
		{name: "A27", tag: "v0.50.116", message: "missing GITHUB_TOKEN"},
		{name: "A28", tag: "v0.50.117", message: "missing GITHUB_TOKEN"},
		{name: "A29", tag: "v0.50.118", message: "missing GITHUB_TOKEN"},
		{name: "A30", tag: "v0.50.119", message: "missing GITHUB_TOKEN"},
		{name: "A31", tag: "v0.50.120", message: "missing GITHUB_TOKEN"},
		{name: "A32", tag: "v0.50.121", message: "missing GITHUB_TOKEN"},
		{name: "A33", tag: "v0.50.122", message: "missing GITHUB_TOKEN"},
		{name: "A34", tag: "v0.50.123", message: "missing GITHUB_TOKEN"},
		{name: "A35", tag: "v0.50.124", message: "missing GITHUB_TOKEN"},
		{name: "failed_A6_tag_75", tag: "v0.50.75", message: frozenReleasePhasePolicy},
		{name: "failed_A6_tag_76", tag: "v0.50.76", message: frozenReleasePhasePolicy},
		{name: "failed_A22_tag_93", tag: "v0.50.93", message: frozenReleasePhasePolicy},
		{name: "failed_A22_tag_94", tag: "v0.50.94", message: frozenReleasePhasePolicy},
		{name: "failed_A22_tag_95", tag: "v0.50.95", message: frozenReleasePhasePolicy},
		{name: "failed_A22_tag_96", tag: "v0.50.96", message: frozenReleasePhasePolicy},
		{name: "failed_A22_tag_97", tag: "v0.50.97", message: frozenReleasePhasePolicy},
		{name: "failed_A22_tag_98", tag: "v0.50.98", message: frozenReleasePhasePolicy},
		{name: "failed_A22_tag_99", tag: "v0.50.99", message: frozenReleasePhasePolicy},
		{name: "failed_A22_tag_100", tag: "v0.50.100", message: frozenReleasePhasePolicy},
		{name: "burned_A23_tag_110", tag: "v0.50.110", message: frozenReleasePhasePolicy},
		{name: "burned_A24_tag_112", tag: "v0.50.112", message: frozenReleasePhasePolicy},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("bash", script)
			command.Env = []string{"GITHUB_REF_NAME=" + test.tag, "PATH=" + os.Getenv("PATH")}
			output, err := command.CombinedOutput()
			if test.wantOK && err != nil {
				t.Fatalf("A0 bootstrap failed: %v\n%s", err, output)
			}
			if !test.wantOK && err == nil {
				t.Fatalf("%s unexpectedly passed\n%s", test.name, output)
			}
			if !strings.Contains(string(output), test.message) {
				t.Fatalf("%s output = %q, want %q", test.name, output, test.message)
			}
		})
	}
}
