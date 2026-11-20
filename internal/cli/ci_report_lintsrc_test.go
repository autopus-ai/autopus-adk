package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/cireport"
)

// Cancels every event: the lint flags it.
const wfCancelsAll = `on: [pull_request, push]
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  a:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps: [{run: "true"}]
`

// Cancels only pull requests, but its job has no timeout: only this version
// yields missing-timeout, so a finding proves it was the file linted.
const wfCancelsPR = `on: [pull_request, push]
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}
jobs:
  a:
    runs-on: ubuntu-latest
    steps: [{run: "true"}]
`

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeWF(t *testing.T, repo, body string) { t.Helper(); writeWFNamed(t, repo, "ci.yml", body) }

func writeWFNamed(t *testing.T, repo, name, body string) {
	t.Helper()
	dir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// staleCheckout returns a clone whose workflow still cancels every event
// while its origin's default branch already fixed it.
func staleCheckout(t *testing.T) (clone, upstream string) {
	t.Helper()
	root := t.TempDir()
	upstream = filepath.Join(root, "up")
	gitRun(t, root, "init", "-q", "-b", "main", upstream)
	writeWF(t, upstream, wfCancelsAll)
	gitRun(t, upstream, "add", ".")
	gitRun(t, upstream, "commit", "-qm", "one")
	clone = filepath.Join(root, "clone")
	gitRun(t, root, "clone", "-q", upstream, clone)
	writeWF(t, upstream, wfCancelsPR)
	gitRun(t, upstream, "commit", "-qam", "two")
	gitRun(t, clone, "fetch", "-q", "origin")
	return clone, upstream
}

func lintIDs(fs []cireport.LintFinding) string {
	var ids []string
	for _, f := range fs {
		ids = append(ids, f.Rule)
	}
	return strings.Join(ids, ",")
}

func TestLintCheckout_BehindCheckoutLintsRemoteDefaultBranch(t *testing.T) {
	clone, _ := staleCheckout(t)
	var r cireport.Report
	got := lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	assertLintedRemote(t, got, r.Notes)
	if !strings.Contains(strings.Join(r.Notes, "\n"), "1 commits behind origin/main") {
		t.Fatalf("notes = %q", r.Notes)
	}
	// An explicit directory is linted as given.
	var r2 cireport.Report
	got = lintCheckout(&r2, filepath.Join(clone, ".github", "workflows"), clone, "o/r", "o/r", nil)
	if !strings.Contains(lintIDs(got), "concurrency-cancels-other-events") {
		t.Fatalf("explicit dir not linted: %v", lintIDs(got))
	}
}

func TestLintCheckout_LocalWorkflowEditsStayLocal(t *testing.T) {
	clone, _ := staleCheckout(t)
	writeWF(t, clone, wfCancelsAll+"# local edit\n")
	var r cireport.Report
	got := lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	if !strings.Contains(lintIDs(got), "concurrency-cancels-other-events") {
		t.Fatalf("local edit not linted: %v", lintIDs(got))
	}
	if !strings.Contains(strings.Join(r.Notes, "\n"), "differs from origin/main") {
		t.Fatalf("notes = %q", r.Notes)
	}
}

func TestLintCheckout_LocalCommitsStayLocal(t *testing.T) {
	clone, _ := staleCheckout(t)
	writeWF(t, clone, wfCancelsAll+"# committed\n")
	gitRun(t, clone, "commit", "-qam", "local")
	var r cireport.Report
	got := lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	if !strings.Contains(lintIDs(got), "concurrency-cancels-other-events") {
		t.Fatalf("local commit not linted: %v", lintIDs(got))
	}
}

func TestLintCheckout_UpToDateHasNoNote(t *testing.T) {
	clone, _ := staleCheckout(t)
	gitRun(t, clone, "merge", "-q", "--ff-only", "origin/main")
	var r cireport.Report
	lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	for _, n := range r.Notes {
		if strings.Contains(n, "origin/main") {
			t.Fatalf("unexpected note %q", n)
		}
	}
}

// assertLintedRemote proves the remote file was parsed and linted: only it
// lacks timeout-minutes, and only the stale one cancels every event.
func assertLintedRemote(t *testing.T, got []cireport.LintFinding, notes []string) {
	t.Helper()
	ids := lintIDs(got)
	if !strings.Contains(ids, "missing-timeout") || strings.Contains(ids, "concurrency-cancels-other-events") {
		t.Fatalf("did not lint the remote file: %v", ids)
	}
	if strings.Contains(strings.Join(notes, "\n"), "not parsed") {
		t.Fatalf("notes = %q", notes)
	}
}

func TestLintCheckout_StrayNonYAMLFileStillLintsRemote(t *testing.T) {
	clone, _ := staleCheckout(t)
	writeWFNamed(t, clone, ".DS_Store", "x")
	var r cireport.Report
	assertLintedRemote(t, lintCheckout(&r, "", clone, "o/r", "o/r", nil), r.Notes)
}

func TestLintCheckout_UntrackedWorkflowStaysLocalEvenIfHidden(t *testing.T) {
	clone, _ := staleCheckout(t)
	gitRun(t, clone, "config", "status.showUntrackedFiles", "no")
	writeWFNamed(t, clone, "new.yml", wfCancelsAll)
	var r cireport.Report
	got := lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	if !strings.Contains(lintIDs(got), "concurrency-cancels-other-events") {
		t.Fatalf("untracked workflow not linted: %v", lintIDs(got))
	}
}

func TestLintCheckout_RemoteNonASCIIWorkflowIsRead(t *testing.T) {
	clone, upstream := staleCheckout(t)
	writeWFNamed(t, upstream, "배포.yml", wfCancelsAll)
	gitRun(t, upstream, "add", ".")
	gitRun(t, upstream, "commit", "-qm", "three")
	gitRun(t, clone, "fetch", "-q", "origin")
	var r cireport.Report
	got := lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	found := false
	for _, f := range got {
		found = found || (f.File == "배포.yml" && f.Rule == "concurrency-cancels-other-events")
	}
	if !found {
		t.Fatalf("non-ASCII remote workflow skipped: %+v", got)
	}
}

func TestLintCheckout_DanglingOriginHEADIsNoted(t *testing.T) {
	clone, _ := staleCheckout(t)
	gitRun(t, clone, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/gone")
	var r cireport.Report
	lintCheckout(&r, "", clone, "o/r", "o/r", nil)
	if !strings.Contains(strings.Join(r.Notes, "\n"), "origin/HEAD does not resolve") {
		t.Fatalf("notes = %q", r.Notes)
	}
}
