package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/cireport"
)

// A feature branch whose own commits leave the workflows alone is still only
// behind as far as the workflows go, including a squash-merged branch whose
// commits never become ancestors of the default branch.
func TestLintCheckout_UnrelatedLocalCommitStillLintsRemote(t *testing.T) {
	clone, _ := staleCheckout(t)
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, clone, "add", "README.md")
	gitRun(t, clone, "commit", "-qm", "docs only")
	var r cireport.Report
	assertLintedRemote(t, lintCheckout(&r, "", clone, "o/r", "o/r", nil), r.Notes)
}

// The command is documented as read-only: a stale stat cache must not make
// it rewrite .git/index, which would hold the index lock against a
// concurrent commit.
func TestLintCheckout_LeavesTheIndexAlone(t *testing.T) {
	clone, _ := staleCheckout(t)
	wf := filepath.Join(clone, ".github", "workflows", "ci.yml")
	if err := os.Chtimes(wf, timeAgo(), timeAgo()); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(clone, ".git", "index")
	before, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	var r cireport.Report
	lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	after, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("the lint rewrote .git/index")
	}
}

// A checkout of another repository than the one measured is never compared
// with its own remote; the note says whose files were read.
func TestLintCheckout_OtherRepositoryReadsLocalWithNote(t *testing.T) {
	clone, _ := staleCheckout(t)
	var r cireport.Report
	got := lintCheckout(&r, "", clone, "o/other", "o/r", nil)
	if !strings.Contains(lintIDs(got), "concurrency-cancels-other-events") {
		t.Fatalf("local checkout not linted: %v", lintIDs(got))
	}
	notes := strings.Join(r.Notes, "\n")
	if !strings.Contains(notes, "local checkout of o/other, not o/r") || strings.Contains(notes, "behind") {
		t.Fatalf("notes = %q", r.Notes)
	}
}

func TestLintCheckout_RemoteNotNamedOriginIsNoted(t *testing.T) {
	clone, _ := staleCheckout(t)
	gitRun(t, clone, "remote", "rename", "origin", "upstream")
	var r cireport.Report
	lintCheckout(&r, "", clone, "", "o/r", nil)
	if !strings.Contains(strings.Join(r.Notes, "\n"), "no remote named origin") {
		t.Fatalf("notes = %q", r.Notes)
	}
}

func TestLintCheckout_NoRemoteAtAllHasNoComparisonNote(t *testing.T) {
	clone, _ := staleCheckout(t)
	gitRun(t, clone, "remote", "remove", "origin")
	var r cireport.Report
	lintCheckout(&r, "", clone, "", "o/r", nil)
	if notes := strings.Join(r.Notes, "\n"); strings.Contains(notes, "origin") {
		t.Fatalf("notes = %q", r.Notes)
	}
}

func TestLintCheckout_RemoteWithoutWorkflowsSkipsTheLint(t *testing.T) {
	clone, upstream := staleCheckout(t)
	gitRun(t, upstream, "rm", "-q", ".github/workflows/ci.yml")
	gitRun(t, upstream, "commit", "-qm", "drop ci")
	gitRun(t, clone, "fetch", "-q", "origin")
	var r cireport.Report
	got := lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	if len(got) != 0 {
		t.Fatalf("linted the stale checkout: %v", lintIDs(got))
	}
	notes := strings.Join(r.Notes, "\n")
	if !strings.Contains(notes, "origin/main has no workflow files") || strings.Contains(notes, "could not be read") {
		t.Fatalf("notes = %q", r.Notes)
	}
}

func TestLintCheckout_RemoteSymlinkIsNotAWorkflow(t *testing.T) {
	clone, upstream := staleCheckout(t)
	if err := os.Symlink("ci.yml", filepath.Join(upstream, ".github", "workflows", "link.yml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	gitRun(t, upstream, "add", ".")
	gitRun(t, upstream, "commit", "-qm", "link")
	gitRun(t, clone, "fetch", "-q", "origin")
	var r cireport.Report
	got := lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	assertLintedRemote(t, got, r.Notes)
	for _, f := range got {
		if f.File == "link.yml" {
			t.Fatalf("symlink linted as a workflow: %+v", f)
		}
	}
}

func timeAgo() time.Time { return time.Now().Add(-time.Hour) }
