package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harnessfb"
)

// findingsTranscript is a session whose reviewer flags one file in pkg/store.
func findingsTranscript(t *testing.T, dir, name, file string) string {
	t.Helper()
	at := func(s int) string {
		return time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC).Add(time.Duration(s) * time.Second).Format(time.RFC3339)
	}
	lines := []string{
		fmt.Sprintf(`{"type":"user","timestamp":%q,"cwd":"/work/proj","message":{"role":"user","content":"fix it"}}`, at(0)),
		fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"r1","name":"Agent","input":{"subagent_type":"reviewer"}}]}}`, at(1)),
		fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"r1","content":"1. [%s:10] SQL built by string concat"}]}}`, at(60), file),
	}
	p := filepath.Join(dir, name+".jsonl")
	require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return p
}

// Review findings in one area across three sessions become a pattern the
// report shows with an example finding, and a draft is asked for it.
func TestHarnessCommands_ReviewFindingsBecomeAPattern(t *testing.T) {
	harnessRepo(t)
	t.Setenv(harnessFeedbackEnv, "")
	calls := stubHarnessProviders(t, "survived")
	tdir := t.TempDir()
	for i, f := range []string{"pkg/store/db.go", "pkg/store/cache.go", "pkg/store/db.go"} {
		id := fmt.Sprintf("s%d", i)
		_, err := runAuto(t, "harness", "observe", "--session-id", id, "--transcript", findingsTranscript(t, tdir, id, f))
		require.NoError(t, err)
	}
	out, err := runAuto(t, "harness", "report")
	require.NoError(t, err)
	assert.Contains(t, out, "review-area:pkg/store: 같은 영역(pkg/store)에서 리뷰 지적이 반복됨 — 세션 3개")
	assert.Contains(t, out, "s1  1회 — [pkg/store/cache.go:10] SQL built by string concat")
	assert.NotEmpty(t, *calls, "a repeated review area is drafted like any pattern")
}

// The background run ends 0 when a draft fails or no provider exists: the
// failure is logged (a failed draft also backs off) and nothing reads the
// detached process's exit status. A person's propose still fails.
func TestHarnessObserve_BackgroundRunEndsCleanlyOnDraftFailure(t *testing.T) {
	dir := harnessRepo(t)
	t.Setenv(harnessFeedbackEnv, "")
	oldP, oldA := harnessProviders, harnessAsker
	harnessProviders = func() ([]string, error) { return []string{"claude"}, nil }
	harnessAsker = func(string) harnessfb.Asker {
		return func(context.Context, string, string) (harnessfb.Reply, error) {
			return harnessfb.Reply{}, errors.New("claude: command not found")
		}
	}
	t.Cleanup(func() { harnessProviders, harnessAsker = oldP, oldA })
	tdir := t.TempDir()
	for i, f := range []string{"pkg/store/db.go", "pkg/store/cache.go", "pkg/store/db.go"} {
		id := fmt.Sprintf("s%d", i)
		_, err := runAuto(t, "harness", "observe", "--session-id", id, "--transcript", findingsTranscript(t, tdir, id, f))
		require.NoError(t, err)
		t.Setenv(harnessFeedbackEnv, "")
	}
	log, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(harnessfb.LogRel)))
	require.NoError(t, err)
	assert.Contains(t, string(log), "후보 작성 실패")
	_, err = runAuto(t, "harness", "propose")
	require.Error(t, err, "a person's propose reports the failure")

	harnessProviders = func() ([]string, error) { return nil, nil }
	_, err = runAuto(t, "harness", "observe", "--session-id", "s9", "--transcript", findingsTranscript(t, tdir, "s9", "pkg/store/x.go"))
	require.NoError(t, err, "no provider: the patterns wait")
}
