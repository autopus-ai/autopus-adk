package verifygate

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// onPath makes lookPath see exactly these tools. Tests that call it must not
// run in parallel: lookPath is package state.
func onPath(t *testing.T, tools ...string) {
	t.Helper()
	previous := lookPath
	lookPath = func(name string) (string, error) {
		if slices.Contains(tools, name) {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPath = previous })
}

// Before `pnpm install` there is no lockfile, but the project still declares
// pnpm (#300). With pnpm off PATH it runs through corepack.
func TestPlan_NodeManagerFromDeclarationThroughCorepack(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"packageManager field": {"package.json": `{"packageManager":"pnpm@10.18.0","scripts":{"typecheck":"pnpm -r typecheck"}}`},
		"pnpm workspace file":  {"package.json": `{"scripts":{"typecheck":"tsc"}}`, "pnpm-workspace.yaml": "packages: []\n"},
	} {
		onPath(t, "corepack", "npm")
		root := t.TempDir()
		writeFiles(t, root, files)
		steps := Plan(root, []string{"src/a.ts"})
		require.Len(t, steps, 1, name)
		assert.Equal(t, []string{"corepack", "pnpm", "run", "typecheck"}, steps[0].Argv, name)
		assert.Equal(t, []string{filepath.Join(root, "node_modules")}, steps[0].Requires, name)
	}

	onPath(t, "pnpm", "corepack")
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"package.json": `{"packageManager":"pnpm@10","scripts":{"test":"vitest"}}`})
	assert.Equal(t, []string{"pnpm", "run", "test"}, Plan(root, []string{"a.ts"})[0].Argv, "pnpm on PATH runs directly")

	onPath(t, "npm")
	root = t.TempDir()
	writeFiles(t, root, map[string]string{"package.json": `{"scripts":{"test":"vitest"}}`})
	assert.Equal(t, []string{"npm", "run", "test"}, Plan(root, []string{"a.ts"})[0].Argv, "no declaration stays npm")
}

// A step that cannot run is skipped with its reason, never reported as a
// failure: dependencies not installed, or a command inside it not found.
func TestRun_EnvironmentProblemsAreSkipsNotFailures(t *testing.T) {
	dir := t.TempDir()
	steps := []Step{
		{Name: "needs deps", Argv: []string{"sh", "-c", "exit 0"}, Dir: dir, Requires: []string{filepath.Join(dir, "node_modules")}},
		{Name: "pnpm missing inside", Argv: []string{"sh", "-c", "echo 'sh: pnpm: command not found' >&2; exit 127"}, Dir: dir, UnavailableExit: []int{127}},
		{Name: "no tool", Argv: []string{"definitely-not-installed-tool"}, Dir: dir},
	}
	failure, skipped := Run(context.Background(), steps, time.Minute)
	require.Nil(t, failure)
	require.Len(t, skipped, 3)
	assert.Contains(t, skipped[0].Reason, "dependencies are not installed")
	assert.Contains(t, skipped[1].Reason, "exit 127")
	assert.Contains(t, skipped[1].Reason, "pnpm: command not found")
	assert.Contains(t, skipped[2].Reason, "is not installed")

	// A real failure still fails, and 127 is only special where a step says so.
	failure, _ = Run(context.Background(), []Step{{Name: "broken", Argv: []string{"sh", "-c", "exit 127"}, Dir: dir}}, time.Minute)
	require.NotNil(t, failure)
}
