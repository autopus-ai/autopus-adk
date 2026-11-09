package loop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// A resumed loop starts from the earlier loop branch, so the fix made there
// already holds: the lane passes without another agent call, and the caller's
// branch is still untouched.
func TestQALoopRun_BaseRefCarriesTheEarlierFix(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	repo.write(".autopus/qa/scenarios/login.yaml", "id: login\n")
	first, err := repo.seededLoop(repo.deps(triage.ClassProductDefect, writing(repo, map[string]string{"fixed.txt": "ok\n"})))
	require.NoError(t, err)
	require.Equal(t, "broken\n", repo.read("fixed.txt"), "back on main the fix is not there")
	require.Equal(t, first.Branch, LatestSeededBranch(repo.dir))

	calls := 0
	deps := repo.deps(triage.ClassProductDefect, func(ctx context.Context, req agentexec.Request) (agentexec.Response, error) {
		calls++
		return writing(repo, map[string]string{"fixed.txt": "ok\n"})(ctx, req)
	})
	deps.RunSuffix = func() string { return "cafe" }
	report, err := Run(context.Background(), Options{ProjectDir: repo.dir, Lane: "fast", Agent: agentexec.TargetClaude,
		MaxIterations: 3, BaseRef: first.Branch}, deps)

	require.NoError(t, err)
	assert.Equal(t, StopPassed, report.StopReason)
	assert.Equal(t, first.Branch, report.ResumedFrom)
	assert.Zero(t, calls, "the earlier fix carried over")
	assert.Equal(t, "broken\n", repo.read("fixed.txt"))
}

func TestQALoopRun_BaseRefMustExist(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	_, err := Run(context.Background(), Options{ProjectDir: repo.dir, Lane: "fast", Agent: agentexec.TargetClaude,
		MaxIterations: 1, BaseRef: "autopus/qa-loop/gone"}, repo.deps(triage.ClassProductDefect, writing(repo, nil)))
	require.Error(t, err)
	assert.Equal(t, CodeInvalidOptions, ErrorCode(err))
	assert.Equal(t, "main", repo.git("branch", "--show-current"))
}
