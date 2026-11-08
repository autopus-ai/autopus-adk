package autopilot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// A CLI product has test scenarios but no browser scenarios. The real
// compiler refuses an empty scenario set; the chain must still reach the
// loop, whose run planner reads exec cases from the test-scenarios files.
func TestRun_CLIProjectWithOnlyTestScenariosReachesTheLoop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".autopus", "qa", "test-scenarios", "INTENT-x.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))

	c := &calls{}
	deps := fakeDeps(c, nil)
	deps.Compile = scenario.CompileProject
	report, err := Run(context.Background(), Options{ProjectDir: dir, Agent: agentexec.TargetClaude, Lane: "fast", Auto: true}, deps)
	require.NoError(t, err)
	assert.Equal(t, StageLoop, report.Stage)
	assert.Equal(t, 0, report.Compiled)
	assert.Equal(t, []string{"loop"}, c.order)
	assert.Equal(t, []string{".autopus/qa/test-scenarios/INTENT-x.yaml"}, c.loop.SeedPaths)
}

// With nothing at all to run, the compile error still stops the chain.
func TestRun_NoScenariosOfAnyKindStillFails(t *testing.T) {
	t.Parallel()
	c := &calls{}
	deps := fakeDeps(c, nil)
	deps.Compile = scenario.CompileProject
	_, err := Run(context.Background(), Options{ProjectDir: t.TempDir(), Agent: agentexec.TargetClaude, Lane: "fast", Auto: true}, deps)
	var stop *Error
	require.ErrorAs(t, err, &stop)
	assert.Equal(t, StageCompile, stop.Stage)
	assert.Contains(t, stop.Error(), "pass a SPEC-ID")
	assert.Equal(t, []string{"restore"}, c.order, "the loop branches were tried first")
}

// A rerun without a SPEC after a stopped loop gets the scenarios back from
// the loop branch and runs them instead of stopping at compile.
func TestRun_RerunRestoresScenariosFromTheLoopBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := &calls{}
	deps := fakeDeps(c, nil)
	deps.Compile = scenario.CompileProject
	var lines []string
	deps.RestoreSeed = func(projectDir string, dirs []string) (qaloop.Restored, error) {
		assert.Equal(t, []string{scenario.DirRel, testscenario.DirRel}, dirs)
		path := filepath.Join(projectDir, ".autopus", "qa", "test-scenarios", "INTENT-x.yaml")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))
		return qaloop.Restored{Branch: "autopus/qa-loop/qaloop-1", Files: []string{".autopus/qa/test-scenarios/INTENT-x.yaml"}}, nil
	}
	_, err := Run(context.Background(), Options{ProjectDir: dir, Agent: agentexec.TargetClaude, Lane: "fast", Auto: true,
		Progress: func(_, line string) { lines = append(lines, line) }}, deps)
	require.NoError(t, err)
	assert.Equal(t, []string{"loop"}, c.order)
	assert.Equal(t, []string{".autopus/qa/test-scenarios/INTENT-x.yaml"}, c.loop.SeedPaths)
	assert.Contains(t, lines, "restored 1 scenario file(s) from autopus/qa-loop/qaloop-1; the product fixes made there stay on that branch")
}
