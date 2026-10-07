package cli

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-SIGMABAND-001 REQ-16 / S16: the band metric store under .autopus/metrics/
// is local-only runtime state. The managed .gitignore hides it, sync never puts
// it in a commit plan, a tracked copy is grouped with the local-only runtime
// family, and the staged hygiene check blocks it even when a source-of-truth
// change is staged alongside, because no source change regenerates it.

const (
	metricsStorePrefix = ".autopus/metrics/"
	metricsStoreFile   = ".autopus/metrics/ci-runs.jsonl"
)

// metricsStorePaths are the files the band store writes (REQ-01, REQ-10, REQ-24).
var metricsStorePaths = []string{
	".autopus/metrics/ci-runs.jsonl",
	".autopus/metrics/canary-runs.jsonl",
	".autopus/metrics/band-events.jsonl",
	".autopus/metrics/band-state.json",
	".autopus/metrics/.lock",
	".autopus/metrics/pending/c1.json",
}

func TestHygieneMetricsStore_EachLocalOnlyListHoldsOneEntry(t *testing.T) {
	t.Parallel()

	for name, list := range map[string][]string{
		"gitignorePatterns":               gitignorePatterns,
		"generatedRuntimePrefixes":        generatedRuntimePrefixes,
		"trackedIgnoredLocalOnlyPrefixes": trackedIgnoredLocalOnlyPrefixes,
		"hygieneAlwaysBlockPrefixes":      hygieneAlwaysBlockPrefixes,
		"runtimeUnignoredExtraPrefixes":   runtimeUnignoredExtraPrefixes,
	} {
		count := 0
		for _, entry := range list {
			if entry == metricsStorePrefix {
				count++
			}
		}
		assert.Equal(t, 1, count, "%s must hold exactly one %s entry", name, metricsStorePrefix)
	}
}

func TestHygieneMetricsStore_ManagedGitignoreHidesEveryStoreFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	plan, err := updateGitignore(dir)
	require.NoError(t, err)
	assert.Contains(t, plan.Missing, metricsStorePrefix, "a fresh project must receive the store pattern")
	syncGit(t, dir, "init")

	for _, rel := range metricsStorePaths {
		writePolicyTestFile(t, dir, rel)
		out, err := exec.Command("git", "-C", dir, "check-ignore", "--no-index", "--quiet", rel).CombinedOutput()
		assert.NoError(t, err, "%s must be ignored by the managed .gitignore: %s", rel, out)
	}

	// The rule is the store directory only, not a broader .autopus/ rule.
	writePolicyTestFile(t, dir, ".autopus/project/metrics.md")
	err = exec.Command("git", "-C", dir, "check-ignore", "--no-index", "--quiet", ".autopus/project/metrics.md").Run()
	assert.Error(t, err, "a human-managed project doc must stay visible to git")
}

// A repository whose .gitignore predates the store pattern gets the
// status/doctor runtime_unignored warning for every store file, and only
// for those: a human-managed project doc stays out of it.
func TestHygieneMetricsStore_StatusWarnsWhenTheStoreIsNotIgnored(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	syncGit(t, dir, "init")
	for _, rel := range append([]string{".autopus/project/metrics.md"}, metricsStorePaths...) {
		writePolicyTestFile(t, dir, rel)
	}

	report := collectStatusHygiene(dir)

	assert.Equal(t, "warn", report.Status)
	assert.ElementsMatch(t, metricsStorePaths, report.RuntimeUnignored)
	assert.Equal(t, len(metricsStorePaths), report.payload().RuntimeUnignored.Count)
}

func TestHygieneMetricsStore_SyncVerifyBlocksStoreInEveryRepoRole(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		repo repoDirty
		keep string
	}{
		{name: "meta root", repo: repoDirty{Path: ".", Role: repoRoleMeta}, keep: "AGENTS.md"},
		{name: "module", repo: repoDirty{Path: "mod-a", Role: repoRoleModule}, keep: "pkg/owned.go"},
		{name: "single repo", repo: repoDirty{Path: ".", Role: repoRoleSingle}, keep: "pkg/owned.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := tc.repo
			repo.Files = []dirtyFile{{Rel: tc.keep}}
			for _, rel := range metricsStorePaths {
				repo.Files = append(repo.Files, dirtyFile{Rel: rel})
			}
			classified := classifyWorkspace([]repoDirty{repo})

			var blocked []string
			for _, item := range classified.Blocked {
				assert.Equal(t, "generated/runtime", item.Reason, "%s", item.Rel)
				blocked = append(blocked, item.Rel)
			}
			assert.ElementsMatch(t, metricsStorePaths, blocked)
			assert.Empty(t, classified.Unclassified)

			planned := classified.PhaseB.Files
			for _, group := range classified.PhaseA {
				planned = append(planned, group.Files...)
			}
			assert.Equal(t, []string{tc.keep}, planned, "only the owned file may reach a commit plan")
		})
	}
}

func TestHygieneMetricsStore_TrackedStoreIsLocalOnlyRuntimeFamily(t *testing.T) {
	t.Parallel()

	families := classifyTrackedIgnored(metricsStorePaths)

	require.Len(t, families, 1)
	assert.Equal(t, "local-only brainstorm/orchestra/runtime output", families[0].Label)
	assert.Equal(t, trackedIgnoredUntrack, families[0].Disposition)
	assert.Equal(t, metricsStorePaths, families[0].Paths)
}

func TestHygieneMetricsStore_StagedCheckBlocksStorePath(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources []string
	}{
		{name: "store alone"},
		{name: "with template source", sources: []string{"templates/codex/agents/reviewer.toml.tmpl"}},
		{name: "with content source", sources: []string{"content/agents/reviewer.md", "pkg/content/hooks.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initSyncRepo(t, dir)
			syncWrite(t, dir, metricsStoreFile, `{"schema":"autopus.metric_observation.v1"}`+"\n")
			for _, rel := range tc.sources {
				syncWrite(t, dir, rel, "source of truth\n")
			}
			syncGit(t, dir, append([]string{"add", "--", metricsStoreFile}, tc.sources...)...)

			root := NewRootCmd()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs([]string{"check", "--hygiene", "--staged", "--quiet", "--dir", dir})

			require.Error(t, root.Execute(), "a staged metric store must fail the hygiene check")
			blocked := blockedHygieneLines(out.String())
			assert.Equal(t, []string{metricsStoreFile}, blocked, "exactly the store path is reported:\n%s", out.String())
		})
	}
}

// Untracking a stale store is the remediation the tracked-ignored family
// recommends, so the staged check must let that index-only change through.
func TestHygieneMetricsStore_StagedUntrackCleanupPasses(t *testing.T) {
	dir := t.TempDir()
	initSyncRepo(t, dir)
	syncWrite(t, dir, metricsStoreFile, "{}\n")
	syncGit(t, dir, "add", "--", metricsStoreFile)
	syncGit(t, dir, "commit", "-m", "tracked store fixture")
	syncGit(t, dir, "rm", "--cached", "--", metricsStoreFile)

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"check", "--hygiene", "--staged", "--quiet", "--dir", dir})

	require.NoError(t, root.Execute(), "untracking the store must pass:\n%s", out.String())
}

// blockedHygieneLines returns the paths of the "<path> (generated/runtime
// drift ...)" lines that checkHygiene prints for blocked paths.
func blockedHygieneLines(out string) []string {
	const marker = " (generated/runtime drift without source-of-truth change)"
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		fields := strings.Fields(line[:idx])
		if len(fields) > 0 {
			paths = append(paths, fields[len(fields)-1])
		}
	}
	return paths
}
