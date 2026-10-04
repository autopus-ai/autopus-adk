package autopilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/generate"
	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
	"github.com/insajin/autopus-adk/pkg/qa/promote"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

type calls struct {
	order []string
	loop  qaloop.Options
}

func fakeDeps(c *calls, genErr error) Deps {
	return Deps{
		Generate: func(_ context.Context, o generate.Options) (generate.Report, error) {
			c.order = append(c.order, "generate:"+o.SpecID)
			return generate.Report{Spec: o.SpecID, Written: []string{"x"}}, genErr
		},
		Promote: func(string, promote.Options) (promote.Report, error) {
			c.order = append(c.order, "promote")
			return promote.Report{}, nil
		},
		Compile: func(string, bool) (scenario.Result, error) {
			c.order = append(c.order, "compile")
			return scenario.Result{TestDir: "e2e", Compiled: []scenario.Compiled{{ScenarioID: "login"}}}, nil
		},
		Loop: func(_ context.Context, o qaloop.Options) (qaloop.Report, error) {
			c.order = append(c.order, "loop")
			c.loop = o
			return qaloop.Report{StopReason: qaloop.StopPassed}, nil
		},
	}
}

func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range []string{".autopus/qa/scenarios/login.yaml", ".autopus/qa/scenarios/candidates/skipped.yaml", ".autopus/qa/test-scenarios/SPEC-X.yaml"} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))
	}
	return dir
}

func TestRun_AutoChainsEveryStageAndSeedsOnlyActiveFiles(t *testing.T) {
	t.Parallel()
	c := &calls{}
	dir := project(t)
	report, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: "SPEC-X", Agent: agentexec.TargetClaude, Lane: "browser-staging", Auto: true}, fakeDeps(c, nil))
	require.NoError(t, err)
	assert.Equal(t, []string{"generate:SPEC-X", "promote", "compile", "loop"}, c.order)
	assert.Equal(t, StageLoop, report.Stage)
	assert.Equal(t, 1, report.Compiled)
	assert.Equal(t, []string{".autopus/qa/scenarios/login.yaml", ".autopus/qa/test-scenarios/SPEC-X.yaml", "e2e/autopus-generated"}, c.loop.SeedPaths)
	assert.Equal(t, "test(qa): SPEC-X 시나리오를 추가한다", c.loop.SeedMessage)
}

// Without --auto and without a yes, nothing is promoted: an unattended
// invocation can never proceed by accident.
func TestRun_StopsAtConfirmationUnlessAccepted(t *testing.T) {
	t.Parallel()
	c := &calls{}
	_, err := Run(context.Background(), Options{ProjectDir: project(t), SpecID: "SPEC-X", Agent: agentexec.TargetClaude, Lane: "fast"}, fakeDeps(c, nil))
	var stop *Error
	require.ErrorAs(t, err, &stop)
	assert.Equal(t, CodeDeclined, stop.Code)
	assert.Equal(t, []string{"generate:SPEC-X"}, c.order)

	c = &calls{}
	_, err = Run(context.Background(), Options{ProjectDir: project(t), SpecID: "SPEC-X", Agent: agentexec.TargetClaude, Lane: "fast",
		Confirm: func(generate.Report) bool { return true }}, fakeDeps(c, nil))
	require.NoError(t, err)
	assert.Equal(t, []string{"generate:SPEC-X", "promote", "compile", "loop"}, c.order)
}

func TestRun_NoSpecRunsTheLoopOverExistingScenarios(t *testing.T) {
	t.Parallel()
	c := &calls{}
	report, err := Run(context.Background(), Options{ProjectDir: project(t), Agent: agentexec.TargetCodex, Lane: "fast"}, fakeDeps(c, nil))
	require.NoError(t, err)
	assert.Equal(t, []string{"compile", "loop"}, c.order)
	assert.Nil(t, report.Generate)
	assert.Equal(t, "test(qa): QA 시나리오를 갱신한다", c.loop.SeedMessage)
}

func TestRun_GenerationFailureStopsBeforePromotion(t *testing.T) {
	t.Parallel()
	c := &calls{}
	_, err := Run(context.Background(), Options{ProjectDir: project(t), SpecID: "SPEC-X", Agent: agentexec.TargetClaude, Lane: "fast", Auto: true},
		fakeDeps(c, &generate.Error{Code: "qa_generate_no_criteria", Message: "none"}))
	var stop *Error
	require.ErrorAs(t, err, &stop)
	assert.Equal(t, StageGenerate, stop.Stage)
	assert.Equal(t, "qa_generate_no_criteria", stop.Code)
	assert.Equal(t, []string{"generate:SPEC-X"}, c.order)
	assert.True(t, errors.Is(err, stop.Err))
}

func TestDetectLane(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "fast", DetectLane(t.TempDir()))
}
