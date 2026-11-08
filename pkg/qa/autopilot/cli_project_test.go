package autopilot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
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
	assert.Contains(t, stop.Error(), "autopus/qa-loop/<run-id> branch", "the likely place the scenarios went is named")
	assert.Empty(t, c.order)
}
