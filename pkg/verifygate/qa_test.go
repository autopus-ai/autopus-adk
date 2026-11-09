package verifygate

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const execScenarios = `schema_version: qamesh.test-scenarios.v1
spec: INTENT-x
cases:
  - id: runs
    ac: AC-1
    kind: happy
    title: runs
    given: built
    when: run
    then: ok
    automation:
      type: exec
      exec:
        build: [go, build, -o, $BIN/x, .]
        argv: [$BIN/x]
        expect: {exit_code: 0}
`

// A product change in a project with exec cases runs them after the build
// and tests; a case that cannot run (exit 2) does not fail the gate.
func TestPlan_QAExecCasesFollowAProductChange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod": "module x\n", "main.go": "package main\n",
		".autopus/qa/test-scenarios/x.yaml": execScenarios,
	})

	steps := Plan(root, []string{"main.go"})
	require.Len(t, steps, 3)
	qa := steps[2]
	assert.Equal(t, "qa exec --all", qa.Name)
	assert.Equal(t, []string{"auto", "qa", "exec", "--all", "--project-dir", root}, qa.Argv)
	assert.Equal(t, []int{2}, qa.PassExit)

	assert.Empty(t, Plan(root, []string{"README.md"}), "a docs change runs nothing")
}

func TestPlan_NoQAStepWithoutExecCases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module x\n", "main.go": "package main\n"})
	for _, step := range Plan(root, []string{"main.go"}) {
		assert.NotEqual(t, "qa exec --all", step.Name)
	}
}

func TestRun_PassExitIsNotAFailure(t *testing.T) {
	t.Parallel()
	exit := func(code string, pass []int) *Failure {
		failure, _ := Run(context.Background(), []Step{{Name: "s", Argv: []string{"sh", "-c", "exit " + code}, Dir: t.TempDir(), PassExit: pass}}, time.Minute)
		return failure
	}
	assert.Nil(t, exit("2", []int{2}))
	assert.NotNil(t, exit("1", []int{2}))
	assert.NotNil(t, exit("2", nil))
}
