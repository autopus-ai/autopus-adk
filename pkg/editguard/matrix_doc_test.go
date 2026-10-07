package editguard

import (
	"os"
	"strings"
	"testing"
)

// REQ-EG-16 and S12: docs/edit-guard.md publishes exactly the matrix rows, in
// order, with one state per platform.
func TestEnforcementMatrixDoc_ListsExactlyTheMatrixRows(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../docs/edit-guard.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(data), "## Enforcement matrix\n")
	if !ok {
		t.Fatal("docs/edit-guard.md has no Enforcement matrix section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var got []string
	for _, line := range strings.Split(section, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || strings.TrimSpace(cells[1]) == "Platform" || strings.HasPrefix(cells[1], "---") {
			continue
		}
		got = append(got, strings.TrimSpace(cells[1])+" "+strings.TrimSpace(cells[2]))
	}
	want := []string{"Claude Code enforced", "OpenCode enforced", "OpenCode 1.x host-unverified", "Codex enforced",
		"Gemini CLI enforced",
		"Antigravity advisory-only", "OMP none"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Fatalf("docs matrix rows = %q, want %q", got, want)
	}
	var lanes []string
	for _, lane := range Lanes() {
		lanes = append(lanes, lane.Name+" "+string(lane.State))
	}
	if strings.Join(lanes, ", ") != strings.Join(got, ", ") {
		t.Fatalf("docs matrix rows %q drift from Lanes() %q", got, lanes)
	}
}
