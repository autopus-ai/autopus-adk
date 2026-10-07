package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandTrackedArgv is the read-only check that the store is not committed.
const bandTrackedArgv = "git -c core.fsmonitor=false ls-files -z -- :(icase,glob).autopu*/metric* :(icase,glob).autopu*/metric*/**"

// Security L3: a metric store that git tracks came with the repository, not
// from this machine's runs, so band refuses to read or write it: the run
// fails closed with reason store_tracked, takes no lock, changes no file,
// and runs no gh, with or without --dry-run.
func TestReactBand_RefusesATrackedMetricsStore(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--no-fetch"}, {"--dry-run"}, nil} {
		p := newBandCmdProject(t)
		p.store(healthband.CIRunsFile, bandO2CI())
		before := bandTreeHashes(t, p.dir)
		runner := emptyRunList()
		runner.answers[bandTrackedArgv] = fakeBandAnswer{stdout: ".autopus/metrics/ci-runs.jsonl\x00"}

		run := p.run(runner, nil, append(args, "--format", "json")...)

		require.Error(t, run.err, args)
		envelope := decodeBandEnvelope(t, run.stdout)
		assert.Equal(t, "error", envelope.Status, args)
		assert.Contains(t, envelope.Data.Reasons, healthband.ReasonStoreTracked, args)
		assert.Equal(t, before, bandTreeHashes(t, p.dir), args)
		assert.NoFileExists(t, p.metrics(healthband.LockFile), args)
		assert.Equal(t, []string{bandTrackedArgv}, runner.argvs("git"), args)
		assert.Empty(t, runner.argvs("gh"), args)
	}
}

// Review round 2: a listed store path counts whatever git's exit status, and
// a listing above the output bound counts as a whole; only a failure that
// listed nothing counts as untracked. Neither the listed directory nor the
// store exists here, so identity cannot clear the listed path (round 3).
func TestReactBandGH_StoreTrackedCountsAnyListing(t *testing.T) {
	t.Parallel()
	for name, answer := range map[string]fakeBandAnswer{
		"listing then a failure":  {stdout: ".autopus/metrics/ci-runs.jsonl\x00", err: errors.New("exit status 128")},
		"listing above the bound": {stdout: strings.Repeat(".autopus/metrics/x.jsonl\x00", bandTextOutputCap/24+1)},
	} {
		runner := &fakeBandRunner{answers: map[string]fakeBandAnswer{bandTrackedArgv: answer}}
		assert.True(t, newBandGHClient(runner).storeTracked(context.Background(), t.TempDir()), name)
	}
	failing := &fakeBandRunner{answers: map[string]fakeBandAnswer{bandTrackedArgv: {err: errors.New("exit status 128")}}}
	assert.False(t, newBandGHClient(failing).storeTracked(context.Background(), t.TempDir()), "a failure without a listing")
}

// bandGitRepo makes a fresh repository and returns it with a helper that
// runs git in it, failing the test on any git error.
func bandGitRepo(t *testing.T) (string, func(args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).Output()
		require.NoError(t, err, "git %v", args)
		return string(out)
	}
	git("init", "-q")
	return repo, git
}

// bandStoreFile writes one file under repo/<dir> and returns its path.
func bandStoreFile(t *testing.T, repo, dir, name string) string {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(dir), name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o600))
	return path
}

// The check asks git itself: an untracked store is fine, a store file in the
// index is tracked, and a directory outside any repository is not tracked.
func TestReactBandGH_StoreTrackedAsksGit(t *testing.T) {
	t.Parallel()
	client := newBandGHClient(execBandRunner{})
	repo, git := bandGitRepo(t)
	bandStoreFile(t, repo, ".autopus/metrics", healthband.CIRunsFile)

	assert.False(t, client.storeTracked(context.Background(), repo), "untracked")
	git("add", "-f", ".autopus/metrics/ci-runs.jsonl")
	assert.True(t, client.storeTracked(context.Background(), repo), "in the index")
	assert.False(t, client.storeTracked(context.Background(), t.TempDir()), "no repository")
}

// Review round 2: 401 tracked files with long names list more than the
// output bound, and that store is still tracked.
func TestReactBandGH_StoreTrackedCountsAListingAboveTheOutputBound(t *testing.T) {
	t.Parallel()
	repo, git := bandGitRepo(t)
	bandStoreFile(t, repo, ".autopus/metrics", healthband.CIRunsFile)
	pad := strings.Repeat("p", 200)
	for i := range 400 {
		bandStoreFile(t, repo, ".autopus/metrics", fmt.Sprintf("%s-%04d", pad, i))
	}
	git("add", "-f", ".autopus/metrics")
	require.Greater(t, len(git("ls-files", "-z", "--", ".autopus/metrics")), bandTextOutputCap, "the listing exceeds the bound")

	assert.True(t, newBandGHClient(execBandRunner{}).storeTracked(context.Background(), repo))
}

// Review round 2: a case variant of the store directory in the index is
// tracked as well. On a case-insensitive file system (APFS, NTFS)
// .autopus/METRICS/ci-runs.jsonl is the store file band would read.
func TestReactBandGH_StoreTrackedMatchesACaseVariantOfTheStore(t *testing.T) {
	t.Parallel()
	repo, git := bandGitRepo(t)
	bandStoreFile(t, repo, ".autopus/METRICS", healthband.CIRunsFile)
	git("add", "-f", ".autopus/METRICS/ci-runs.jsonl")

	assert.True(t, newBandGHClient(execBandRunner{}).storeTracked(context.Background(), repo))
}

// Security N1: the check never starts a command the repository configures;
// git ls-files would otherwise run core.fsmonitor from .git/config.
func TestReactBandGH_StoreTrackedNeverStartsTheRepositoryFSMonitor(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fsmonitor stand-in is a POSIX shell script")
	}
	repo, git := bandGitRepo(t)
	bandStoreFile(t, repo, ".autopus/metrics", healthband.CIRunsFile)
	git("add", "-f", ".autopus/metrics/ci-runs.jsonl")
	scratch := t.TempDir()
	marker, hook := filepath.Join(scratch, "fsmonitor-ran"), filepath.Join(scratch, "fsmonitor.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\nexit 1\n"), 0o700))
	git("config", "core.fsmonitor", hook)

	assert.True(t, newBandGHClient(execBandRunner{}).storeTracked(context.Background(), repo))
	assert.NoFileExists(t, marker, "the configured fsmonitor ran")
}

// Without git on PATH there is no index that could have brought store files
// in, so the store counts as untracked (docs/health-band.md).
func TestReactBandGH_StoreTrackedWithoutGitCountsAsUntracked(t *testing.T) {
	repo, git := bandGitRepo(t)
	bandStoreFile(t, repo, ".autopus/metrics", healthband.CIRunsFile)
	git("add", "-f", ".autopus/metrics/ci-runs.jsonl")
	t.Setenv("PATH", t.TempDir())

	assert.False(t, newBandGHClient(execBandRunner{}).storeTracked(context.Background(), repo))
}
