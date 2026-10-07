package harneval_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// coverageFloors are the REQ-HE-13 breadth floors of the golden set.
type coverageFloors struct {
	surface, agent, categories, platforms int
	multiRatio                            float64
}

var specFloors = coverageFloors{surface: 20, agent: 12, categories: 4, platforms: 5, multiRatio: 0.60}

// surfaceCategories is the REQ-HE-13 category list for surface tasks.
var surfaceCategories = []string{"routing", "hooks_settings", "prompt_contract", "agent_skill_exposure", "generated_root_hygiene"}

// coverageFindings returns one line per floor the active tasks miss. Only
// active tasks count; categories, platforms, and breadth come from the
// surface tasks, whose assertions are what the PR lane evaluates.
func coverageFindings(tasks []harneval.Task, floors coverageFloors) []string {
	var surface []harneval.Task
	agents := 0
	for _, task := range tasks {
		switch {
		case task.Status.State != harneval.StateActive:
		case task.Kind == harneval.KindSurface:
			surface = append(surface, task)
		case task.Kind == harneval.KindAgent:
			agents++
		}
	}
	categories, platforms, multi := map[string]bool{}, map[string]bool{}, 0
	for _, task := range surface {
		categories[task.Category] = true
		for _, platform := range harneval.TaskPlatforms(task) {
			platforms[platform] = true
		}
		if harneval.IsMulti(task) {
			multi++
		}
	}
	var findings []string
	below := func(name string, got, floor int) {
		if got < floor {
			findings = append(findings, fmt.Sprintf("%s=%d floor=%d", name, got, floor))
		}
	}
	below("active_surface_tasks", len(surface), floors.surface)
	below("active_agent_tasks", agents, floors.agent)
	below("categories", len(categories), floors.categories)
	below("platforms", len(platforms), floors.platforms)
	ratio := 0.0
	if len(surface) > 0 {
		ratio = float64(multi) / float64(len(surface))
	}
	if ratio < floors.multiRatio-1e-9 {
		findings = append(findings, fmt.Sprintf("multi_ratio=%.2f floor=%.2f", ratio, floors.multiRatio))
	}
	return findings
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

// TestGoldenSet_S14_CommittedSetMeetsCoverageFloors is the S14 coverage test
// over the committed evals/harness set.
func TestGoldenSet_S14_CommittedSetMeetsCoverageFloors(t *testing.T) {
	t.Parallel()
	set, err := harneval.LoadSet(repoRoot(t))
	require.NoError(t, err)

	assert.Empty(t, coverageFindings(set.Tasks, specFloors))
	assert.GreaterOrEqual(t, set.Manifest.Floors.SurfaceTasks, specFloors.surface, "vacuity trips at the SPEC floor")
	assert.GreaterOrEqual(t, set.Manifest.Floors.AgentTasks, specFloors.agent, "vacuity trips at the SPEC floor")
	multi := 0
	for _, task := range set.Tasks {
		if task.Kind == harneval.KindSurface {
			assert.Contains(t, surfaceCategories, task.Category, task.ID)
			if harneval.IsMulti(task) {
				multi++
			}
		}
	}
	t.Logf("active surface %d (multi %d), active agent %d",
		set.ActiveCount(harneval.KindSurface), multi, set.ActiveCount(harneval.KindAgent))
}

// corpusTask is the part of a benchmark corpus entry an agent task pins.
type corpusTask struct {
	ID     string `json:"id"`
	Oracle struct {
		Command []string `json:"command"`
	} `json:"oracle"`
}

// TestGoldenSet_S14_AgentTasksPinTheCorpusOracle: every agent task pins the
// raw bytes of its corpus file, names an existing corpus task, and lists
// expected tests that the oracle's -run pattern selects.
func TestGoldenSet_S14_AgentTasksPinTheCorpusOracle(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	set, err := harneval.LoadSet(root)
	require.NoError(t, err)
	for _, task := range set.Tasks {
		if task.Kind != harneval.KindAgent {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(task.CorpusRef.File)))
		require.NoError(t, err, task.ID)
		sum := sha256.Sum256(raw)
		assert.Equal(t, hex.EncodeToString(sum[:]), task.CorpusRef.FileSHA256, task.ID)
		var corpus []corpusTask
		require.NoError(t, json.Unmarshal(raw, &corpus), task.ID)
		index := slices.IndexFunc(corpus, func(c corpusTask) bool { return c.ID == task.CorpusRef.TaskID })
		require.GreaterOrEqual(t, index, 0, "%s names corpus task %s", task.ID, task.CorpusRef.TaskID)
		command := corpus[index].Oracle.Command
		run := slices.Index(command, "-run")
		require.True(t, run >= 0 && run+1 < len(command), task.ID)
		pattern := regexp.MustCompile(command[run+1])
		require.NotEmpty(t, task.ExpectedTests, task.ID)
		for _, name := range task.ExpectedTests {
			assert.True(t, pattern.MatchString(name), "%s: %s is outside %s", task.ID, name, command[run+1])
		}
	}
}

func breadthTask(id string, platforms ...string) harneval.Task {
	task := harneval.Task{ID: id, Kind: harneval.KindSurface, Category: "routing",
		Status: harneval.TaskStatus{State: harneval.StateActive}}
	for _, platform := range platforms {
		task.Assertions = append(task.Assertions, harneval.Assertion{
			Kind: harneval.AssertFileExists, Platform: platform, Path: "AGENTS.md"})
	}
	return task
}

// TestGoldenSetCoverage_S14_MultiRatioFloorIsInclusive: three multi tasks of
// five meet the 0.60 floor exactly; two fall short and are named.
func TestGoldenSetCoverage_S14_MultiRatioFloorIsInclusive(t *testing.T) {
	t.Parallel()
	ratioOnly := coverageFloors{multiRatio: specFloors.multiRatio}
	set := func(multi int) []harneval.Task {
		var tasks []harneval.Task
		for index := range 5 {
			id := fmt.Sprintf("GT-FIX-%c", 'A'+index)
			if index < multi {
				tasks = append(tasks, breadthTask(id, "codex", "omp"))
			} else {
				tasks = append(tasks, breadthTask(id, "codex"))
			}
		}
		return tasks
	}

	assert.Empty(t, coverageFindings(set(3), ratioOnly))
	assert.Equal(t, []string{"multi_ratio=0.40 floor=0.60"}, coverageFindings(set(2), ratioOnly))
	assert.Equal(t, []string{"active_surface_tasks=5 floor=20", "active_agent_tasks=0 floor=12", "categories=1 floor=4",
		"platforms=2 floor=5"}, coverageFindings(set(3), specFloors))
}

// TestGoldenSetCoverage_S14_CorpusByteTamperIsInvalid: one changed corpus byte
// makes the committed set fail to load with corpus_digest_mismatch, and the
// run reports reason invalid before any generation.
func TestGoldenSetCoverage_S14_CorpusByteTamperIsInvalid(t *testing.T) {
	t.Parallel()
	root, tree := repoRoot(t), t.TempDir()
	require.NoError(t, os.CopyFS(filepath.Join(tree, harneval.EvalRoot), os.DirFS(filepath.Join(root, harneval.EvalRoot))))
	corpusDir := filepath.Join("scripts", "benchmarks", "harness")
	require.NoError(t, os.MkdirAll(filepath.Join(tree, corpusDir), 0o755))
	for _, name := range []string{"corpus_a.json", "corpus_b.json"} {
		raw, err := os.ReadFile(filepath.Join(root, corpusDir, name))
		require.NoError(t, err)
		if name == "corpus_a.json" {
			raw[len(raw)-2] ^= 0x01
		}
		require.NoError(t, os.WriteFile(filepath.Join(tree, corpusDir, name), raw, 0o644))
	}

	_, err := harneval.LoadSet(tree)
	var invalid *harneval.InvalidError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, harneval.DetailCorpusDigestMismatch, invalid.Detail)

	result, err := harneval.Run(context.Background(), tree, harneval.RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{harneval.ReasonInvalid}, result.FailureReasons)
	assert.Equal(t, []string{harneval.DetailCorpusDigestMismatch}, result.Details)
}
