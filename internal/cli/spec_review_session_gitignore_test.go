package cli

import (
	"os/exec"
	"testing"
)

// SPEC-REVIEWPOLICY-001: the review session ledger and its lock are local
// runtime state of one machine's reviews. The managed .gitignore patterns that
// `auto init` and `auto update` write must ignore both in every SPEC
// directory, root or nested module, and still leave the SPEC documents
// tracked.
func TestUpdateGitignoreIgnoresReviewSessionFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := updateGitignore(dir); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	ignored := []string{
		".autopus/specs/SPEC-X-001/review-session.json",
		".autopus/specs/SPEC-X-001/.review-session.lock",
		"backend/.autopus/specs/SPEC-Y-001/review-session.json",
		"backend/.autopus/specs/SPEC-Y-001/.review-session.lock",
	}
	for _, rel := range ignored {
		writePolicyTestFile(t, dir, rel)
		if out, err := exec.Command("git", "-C", dir, "check-ignore", "--no-index", "--quiet", rel).CombinedOutput(); err != nil {
			t.Fatalf("expected %s to be ignored: %v\n%s", rel, err, out)
		}
	}
	writePolicyTestFile(t, dir, ".autopus/specs/SPEC-X-001/spec.md")
	if err := exec.Command("git", "-C", dir, "check-ignore", "--no-index", "--quiet", ".autopus/specs/SPEC-X-001/spec.md").Run(); err == nil {
		t.Fatal("the SPEC document must stay tracked")
	}
}
