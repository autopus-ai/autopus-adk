package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// experimentRepo creates a committed git repo and makes it the working
// directory, because the experiment commands resolve the repo from os.Getwd.
func experimentRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Experiment", "GIT_AUTHOR_EMAIL=exp@example.invalid",
			"GIT_COMMITTER_NAME=Experiment", "GIT_COMMITTER_EMAIL=exp@example.invalid",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init", "-q", "-b", "main")
	// The commands under test invoke git themselves, so the identity has to
	// live in the repo rather than in this helper's environment.
	run("config", "user.email", "exp@example.invalid")
	run("config", "user.name", "Experiment")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644))
	run("add", ".")
	run("commit", "-q", "-m", "seed")
	t.Chdir(dir)
	return dir
}

func runExperimentCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"experiment"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err, "git %v", args)
	return strings.TrimSpace(string(out))
}

// init must branch off a clean worktree and name the branch after the session,
// so an XLOOP run is recoverable by coordinate rather than by memory.
func TestExperimentInit_CreatesSessionBranchFromCleanWorktree(t *testing.T) {
	dir := experimentRepo(t)

	out, err := runExperimentCmd(t, "init", "--session-id", "s7")

	require.NoError(t, err)
	assert.Contains(t, out, "experiment/XLOOP-s7")
	assert.Equal(t, "experiment/XLOOP-s7", gitOutput(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
}

// A dirty worktree is refused: branching over uncommitted work would fold
// unrelated edits into the experiment's first iteration.
func TestExperimentInit_RefusesDirtyWorktree(t *testing.T) {
	dir := experimentRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("wip\n"), 0o644))

	_, err := runExperimentCmd(t, "init", "--session-id", "s8")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree must be clean")
	assert.Equal(t, "main", gitOutput(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
}

// commit stages the whole tree and prints the resulting hash, which is the only
// handle a later reset has on this iteration.
func TestExperimentCommit_CommitsIterationAndPrintsResolvableHash(t *testing.T) {
	dir := experimentRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "change.txt"), []byte("iteration\n"), 0o644))

	out, err := runExperimentCmd(t, "commit", "--iteration", "3", "--description", "tighten loop")

	require.NoError(t, err)
	hash := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "committed:"))
	require.NotEmpty(t, hash)
	assert.Equal(t, hash, gitOutput(t, dir, "rev-parse", hash))
	assert.Empty(t, gitOutput(t, dir, "status", "--porcelain"))
}

func TestExperimentCommit_RequiresIteration(t *testing.T) {
	experimentRepo(t)

	_, err := runExperimentCmd(t, "commit", "--description", "no iteration")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "iteration")
}

// reset returns the tree to a named commit, discarding the iteration under
// test; without the flag it refuses rather than guessing a target.
func TestExperimentReset_RestoresNamedCommitAndRequiresTarget(t *testing.T) {
	dir := experimentRepo(t)
	baseline := gitOutput(t, dir, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "change.txt"), []byte("iteration\n"), 0o644))
	_, err := runExperimentCmd(t, "commit", "--iteration", "1")
	require.NoError(t, err)

	out, err := runExperimentCmd(t, "reset", "--commit", baseline)

	require.NoError(t, err)
	assert.Contains(t, out, baseline)
	assert.Equal(t, baseline, gitOutput(t, dir, "rev-parse", "HEAD"))
	_, statErr := os.Stat(filepath.Join(dir, "change.txt"))
	assert.True(t, os.IsNotExist(statErr), "reset left the iteration's file behind")

	_, err = runExperimentCmd(t, "reset")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--commit is required")
}

func TestExperimentReset_RejectsUnknownCommit(t *testing.T) {
	experimentRepo(t)

	_, err := runExperimentCmd(t, "reset", "--commit", "0000000000000000000000000000000000000000")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reset")
}

// metric prints the extracted value and unit; a missing --metric is refused
// instead of reporting a metric nobody measured.
func TestExperimentMetric_ExtractsValueAndRequiresCommand(t *testing.T) {
	out, err := runExperimentCmd(t, "metric",
		"--metric", `printf '{"metric": 42, "unit": "ms"}'`, "--timeout", "5s")

	require.NoError(t, err)
	assert.Contains(t, out, "metric=42")

	_, err = runExperimentCmd(t, "metric")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--metric is required")
}

// A named metric key that the command never emitted must fail extraction
// rather than reporting a value nobody measured.
func TestExperimentMetric_FailsWhenNamedKeyIsAbsent(t *testing.T) {
	_, err := runExperimentCmd(t, "metric",
		"--metric", `printf '{"metric": 1, "other": 2}'`,
		"--metric-key", "p95", "--timeout", "5s")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "p95")
}

// A named key present in metadata wins over the top-level metric value.
func TestExperimentMetric_PrefersNamedMetadataKey(t *testing.T) {
	out, err := runExperimentCmd(t, "metric",
		"--metric", `printf '{"metric": 1, "p95": 7}'`,
		"--metric-key", "p95", "--timeout", "5s")

	require.NoError(t, err)
	assert.Contains(t, out, "metric=7")
}

// record emits the iteration as indented JSON, which is what the XLOOP ledger
// consumes; every flag it echoes must survive the round trip.
func TestExperimentRecord_EmitsIndentedIterationJSON(t *testing.T) {
	out, err := runExperimentCmd(t, "record",
		"--iteration", "4", "--status", "discard",
		"--metric-value", "12.5", "--description", "slower")

	require.NoError(t, err)
	assert.Contains(t, out, "\n  \"Iteration\": 4")
	assert.Contains(t, out, "\"Status\": \"discard\"")
	assert.Contains(t, out, "\"MetricValue\": 12.5")
	assert.Contains(t, out, "\"Description\": \"slower\"")
}
