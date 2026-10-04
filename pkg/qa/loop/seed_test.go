package loop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

var seedPaths = []string{".autopus/qa/scenarios", "e2e/autopus-generated"}

func (r *repo) seededLoop(deps Deps) (Report, error) {
	return Run(context.Background(), Options{ProjectDir: r.dir, Lane: "fast", Agent: agentexec.TargetClaude,
		MaxIterations: 3, SeedPaths: seedPaths, SeedMessage: "test(qa): SPEC-X 시나리오를 추가한다"}, deps)
}

// Generated QA files land as the loop branch's first commit, ahead of the
// fix, and the caller's branch is left untouched.
func TestQALoopRun_SeedCommitPrecedesTheFix(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	repo.write(".autopus/qa/scenarios/login.yaml", "id: login\n")
	repo.write("e2e/autopus-generated/login.spec.ts", "// generated\n")

	report, err := repo.seededLoop(repo.deps(triage.ClassProductDefect, writing(repo, map[string]string{"fixed.txt": "ok\n"})))

	require.NoError(t, err)
	assert.Equal(t, StopPassed, report.StopReason)
	require.NotEmpty(t, report.SeedCommit)
	assert.False(t, report.BranchDeleted)
	subjects := repo.git("log", "--format=%s", "main.."+report.Branch)
	assert.Equal(t, "fix(qa): product_defect login\ntest(qa): SPEC-X 시나리오를 추가한다", subjects)
	assert.Equal(t, "", repo.git("ls-tree", "--name-only", "main", ".autopus/qa/scenarios/login.yaml"), "main stays untouched")
	assert.Equal(t, ".autopus/qa/scenarios/login.yaml", repo.git("ls-tree", "-r", "--name-only", report.Branch, ".autopus/qa/scenarios"))
}

// A run that passes at once still keeps its branch when it seeded files:
// the branch carries the new scenarios to review.
func TestQALoopRun_SeedOnlyBranchIsKept(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	repo.write("fixed.txt", "ok\n")
	repo.git("add", "-A")
	repo.git("commit", "-q", "-m", "chore: seed")
	repo.write(".autopus/qa/scenarios/login.yaml", "id: login\n")

	report, err := repo.seededLoop(repo.deps(triage.ClassProductDefect, writing(repo, nil)))

	require.NoError(t, err)
	assert.Equal(t, StopPassed, report.StopReason)
	assert.NotEmpty(t, report.SeedCommit)
	assert.False(t, report.BranchDeleted)
	assert.Equal(t, "1", repo.git("rev-list", "--count", "main.."+report.Branch))
}

// Tracked edits under a seed path are part of the seed; tracked edits
// anywhere else still refuse the run.
func TestQALoopRun_SeedPathsDoNotExcuseOtherDirtyFiles(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	repo.write("e2e/autopus-generated/login.spec.ts", "// v1\n")
	repo.git("add", "-A")
	repo.git("commit", "-q", "-m", "chore: seed")
	repo.write("e2e/autopus-generated/login.spec.ts", "// v2\n")

	report, err := repo.seededLoop(repo.deps(triage.ClassProductDefect, writing(repo, map[string]string{"fixed.txt": "ok\n"})))
	require.NoError(t, err)
	assert.NotEmpty(t, report.SeedCommit)

	repo.write("src/app.ts", "export const greeting = 'wip'\n")
	_, err = repo.seededLoop(repo.deps(triage.ClassProductDefect, writing(repo, nil)))
	require.Error(t, err)
	assert.Equal(t, CodeDirtyWorktree, ErrorCode(err))
}
