package companionmanifest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseCoordinateAutopilotBashContract replays the measured coordinate
// move one coordinate past the current tree, in a temporary copy, against a
// fixture measurement. It fails when a rule stops matching after a source
// change, when a history line moves, or when any line without a coordinate
// token changes.
func TestReleaseCoordinateAutopilotBashContract(t *testing.T) {
	root := repositoryRoot(t)
	contract := filepath.Join(root, "scripts", "companion-release", "tests",
		"release-coordinate-autopilot-test.sh")
	command := exec.CommandContext(t.Context(), "/bin/bash", contract)
	command.Dir = root
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("release coordinate autopilot contract: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "release coordinate autopilot test: PASS") {
		t.Fatalf("release coordinate autopilot contract did not report PASS:\n%s", output)
	}
}

// TestReleaseCoordinateAutopilotKeepsPublishedGuard pins the rule that a
// published FROM moves only from a measurement: the advance script hands a
// published FROM to the measured mover and refuses --measurements otherwise.
func TestReleaseCoordinateAutopilotKeepsPublishedGuard(t *testing.T) {
	source := readReleaseFile(t, "scripts/release-tools/advance-release-coordinate.sh")
	for _, required := range []string{
		`if [[ "$from_published" -eq 1 && ${#measured_args[@]} -gt 0 ]]; then`,
		"advance-release-coordinate-measured.sh",
		"--measurements is only for a published FROM",
		"measure-published-release.sh ${from_tag} --predecessor-commit SHA",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("advance-release-coordinate.sh missing %q", required)
		}
	}
	measured := readReleaseFile(t, "scripts/release-tools/advance-release-coordinate-measured.sh")
	if !strings.Contains(measured, "move the coordinate from a clean tree") {
		t.Fatal("measured mover no longer refuses a dirty tree")
	}
}
