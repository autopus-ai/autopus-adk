package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGit runs git in dir and fails the test on error; setup must be exact or the
// merge assertions below would test the wrong repository state.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.test",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.test",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// newMergeRepo builds a repo with one executor worktree under
// .claude/worktrees/<runID>-1 holding an uncommitted new file.
func newMergeRepo(t *testing.T, runID string) (repo, worktree string) {
	t.Helper()
	repo = t.TempDir()
	runGit(t, repo, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(repo, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "seed.txt")
	runGit(t, repo, "commit", "-m", "seed")

	worktree = filepath.Join(repo, ".claude", "worktrees", runID+"-1")
	runGit(t, repo, "worktree", "add", "-b", "worktree-"+runID+"-1", worktree, "main")
	return repo, worktree
}

func runMergeCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newWorkflowMergeCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

// Without a run id the command would have to guess which worktrees to destroy,
// so it must refuse instead of defaulting.
func TestWorkflowMergeRequiresRunID(t *testing.T) {
	t.Parallel()
	_, _, err := runMergeCmd(t)
	if err == nil || !contains(err.Error(), "--run is required") {
		t.Fatalf("expected --run requirement, got %v", err)
	}
}

// An ownership plan the operator named but that cannot be read must abort:
// proceeding would merge with no ownership enforcement at all.
func TestWorkflowMergeFailsOnUnreadableOwnershipFile(t *testing.T) {
	t.Parallel()
	repo, _ := newMergeRepo(t, "run-a")
	_, _, err := runMergeCmd(t, "--run", "run-a", "--working-dir", repo,
		"--ownership", filepath.Join(repo, "absent.json"))
	if err == nil || !contains(err.Error(), "read ownership file") {
		t.Fatalf("expected read failure naming the ownership file, got %v", err)
	}
}

// A malformed plan must be reported as a parse failure, not silently treated as
// "no ownership", which would re-open the executor-overlap hole.
func TestWorkflowMergeFailsOnMalformedOwnershipPlan(t *testing.T) {
	t.Parallel()
	repo, _ := newMergeRepo(t, "run-b")
	plan := filepath.Join(repo, "plan.json")
	if err := os.WriteFile(plan, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runMergeCmd(t, "--run", "run-b", "--working-dir", repo, "--ownership", plan)
	if err == nil || !contains(err.Error(), "parse ownership plan") {
		t.Fatalf("expected parse failure, got %v", err)
	}
}

// A run with no matching worktree is a no-op that must still emit a parseable
// result: the JS bridge consumes stdout unconditionally.
func TestWorkflowMergeEmitsEmptyResultWhenNoWorktreeMatches(t *testing.T) {
	t.Parallel()
	repo, _ := newMergeRepo(t, "run-c")
	stdout, _, err := runMergeCmd(t, "--run", "other-run", "--working-dir", repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		RunID           string   `json:"run_id"`
		MergedWorktrees []string `json:"merged_worktrees"`
		MergedFiles     []string `json:"merged_files"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a merge result: %v\n%s", err, stdout)
	}
	if result.RunID != "other-run" || len(result.MergedWorktrees) != 0 || len(result.MergedFiles) != 0 {
		t.Fatalf("expected an empty result for an unmatched run, got %+v", result)
	}
}

// The gate only sees staged files, and the worktrees are throwaway: a merge must
// copy, stage, and then remove both the worktree and its branch.
func TestWorkflowMergeStagesFilesAndRemovesWorktree(t *testing.T) {
	t.Parallel()
	repo, worktree := newMergeRepo(t, "run-d")
	if err := os.MkdirAll(filepath.Join(worktree, "pkg", "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "pkg", "new", "impl.go"), []byte("package new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runMergeCmd(t, "--run", "run-d", "--working-dir", repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		MergedFiles     []string `json:"merged_files"`
		MergedWorktrees []string `json:"merged_worktrees"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a merge result: %v\n%s", err, stdout)
	}
	if len(result.MergedFiles) != 1 || result.MergedFiles[0] != "pkg/new/impl.go" {
		t.Fatalf("expected the executor file to be merged, got %+v", result.MergedFiles)
	}
	if len(result.MergedWorktrees) != 1 {
		t.Fatalf("expected one merged worktree, got %+v", result.MergedWorktrees)
	}

	if _, err := os.Stat(filepath.Join(repo, "pkg", "new", "impl.go")); err != nil {
		t.Fatalf("merged file missing from the working dir: %v", err)
	}
	staged := runGit(t, repo, "diff", "--cached", "--name-only")
	if !contains(staged, "pkg/new/impl.go") {
		t.Fatalf("merged file was not staged for the gate, staged:\n%s", staged)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree was not removed: %v", err)
	}
	if branches := runGit(t, repo, "branch", "--list"); contains(branches, "worktree-run-d-1") {
		t.Fatalf("worktree branch was not deleted:\n%s", branches)
	}
}

// A cleanup failure must not block the gate: the merge result still has to be
// emitted, with the failure reported on stderr.
func TestWorkflowMergeWarnsButSucceedsWhenCleanupFails(t *testing.T) {
	t.Parallel()
	repo, worktree := newMergeRepo(t, "run-e")
	if err := os.WriteFile(filepath.Join(worktree, "touched.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A locked worktree resists `git worktree remove --force`, so cleanup fails
	// while the merge itself still sees the file.
	runGit(t, repo, "worktree", "lock", worktree)

	stdout, stderr, err := runMergeCmd(t, "--run", "run-e", "--working-dir", repo)
	if err != nil {
		t.Fatalf("cleanup failure must not fail the command: %v", err)
	}
	if !contains(stdout, "touched.txt") {
		t.Fatalf("merge result missing the merged file:\n%s", stdout)
	}
	if !contains(stderr, "cleanup warning") {
		t.Fatalf("expected a cleanup warning on stderr, got:\n%s", stderr)
	}
}

// --force removal is destructive, so a path outside the executor worktree tree
// must be refused even if a caller bug hands one over.
func TestRemoveWorktreeRefusesPathOutsideContainmentRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outside := filepath.Join(dir, "important")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	err := removeWorktree(context.Background(), dir, outside)
	if err == nil || !contains(err.Error(), "refusing to remove worktree outside") {
		t.Fatalf("expected refusal, got %v", err)
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("refused path must be untouched: %v", statErr)
	}
}

// Failure detection is driven by the exit code, so a failing git invocation must
// report a nonzero code rather than an empty success.
func TestLiveGitOutputRunnerReportsExitCode(t *testing.T) {
	t.Parallel()
	repo, _ := newMergeRepo(t, "run-f")
	runner := liveGitOutputRunner{}

	out, exit, err := runner.Run(context.Background(), repo, "rev-parse", "--is-inside-work-tree")
	if err != nil || exit != 0 {
		t.Fatalf("expected success, got exit=%d err=%v", exit, err)
	}
	if !contains(out, "true") {
		t.Fatalf("expected captured stdout, got %q", out)
	}

	plain := t.TempDir()
	_, exit, err = runner.Run(context.Background(), plain, "rev-parse", "--git-dir")
	if err == nil || exit == 0 {
		t.Fatalf("expected a nonzero exit outside a repo, got exit=%d err=%v", exit, err)
	}
}

// An unstartable command has no exit code to report, so the runner must still
// classify it as failure instead of returning zero.
func TestLiveGitOutputRunnerFailsOnUnusableDir(t *testing.T) {
	t.Parallel()
	_, exit, err := liveGitOutputRunner{}.Run(context.Background(),
		filepath.Join(t.TempDir(), "absent"), "status")
	if err == nil || exit == 0 {
		t.Fatalf("expected start failure, got exit=%d err=%v", exit, err)
	}
}

// With an ownership plan, a file an executor created outside its assigned set
// must be reported and never copied — that is the hard overlap guarantee.
func TestWorkflowMergeEnforcesOwnershipFromPlan(t *testing.T) {
	t.Parallel()
	repo, worktree := newMergeRepo(t, "run-g")
	for _, name := range []string{"owned.go", "strayed.go"} {
		if err := os.WriteFile(filepath.Join(worktree, name), []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plan := filepath.Join(repo, "plan.json")
	body := `{"plan":{"tasks":[{"id":"t1","files":["owned.go"]},{"id":"t2","files":["strayed.go"]}]}}`
	if err := os.WriteFile(plan, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runMergeCmd(t, "--run", "run-g", "--working-dir", repo, "--ownership", plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		MergedFiles       []string `json:"merged_files"`
		SkippedOutOfScope []string `json:"skipped_out_of_scope"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a merge result: %v\n%s", err, stdout)
	}
	if len(result.MergedFiles) != 1 {
		t.Fatalf("expected exactly the owned file to merge, got %+v", result.MergedFiles)
	}
	merged := result.MergedFiles[0]
	skipped := result.SkippedOutOfScope
	if len(skipped) != 1 || skipped[0] == merged {
		t.Fatalf("expected the other task's file to be skipped, merged=%q skipped=%+v", merged, skipped)
	}
	if _, statErr := os.Stat(filepath.Join(repo, skipped[0])); !os.IsNotExist(statErr) {
		t.Fatalf("out-of-scope file must not be copied into the working dir: %v", statErr)
	}
}

func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}
