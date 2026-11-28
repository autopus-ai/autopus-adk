package harnessfb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleReport = "## 코드 리뷰 결과\n" +
	"### Correctness/Security Findings\n" +
	"1. **C1 HIGH** [internal/cli/run.go:27] the timer is never stopped\n" +
	"2. [/work/proj/pkg/store/db.go:88] query built by string concat\n" +
	"- also internal/cli/run.go:27 again (same finding)\n" +
	"```\n$ go test ./pkg/x/y.go:12\n```\n" +
	"| C1 | 해소 | internal/cli/run.go:30 |\n" +
	"Tested with go 1.25:3 and /tmp/other/file.go:9\n"

func TestReviewFindings_ReadsFileLineFindingsOnly(t *testing.T) {
	got := reviewFindings(sampleReport, "/work/proj", nil)
	require.Len(t, got, 2)
	assert.Equal(t, Finding{File: "internal/cli/run.go", Line: 27, Summary: "C1 HIGH [internal/cli/run.go:27] the timer is never stopped"}, got[0])
	assert.Equal(t, "pkg/store/db.go", got[1].File)
	assert.Equal(t, "internal/cli", findingArea("internal/cli/run.go"))
	assert.Equal(t, "pkg/adapter", findingArea("pkg/adapter/omp/omp.go"))
	assert.Equal(t, "main.go", findingArea("main.go"))
}

func line(at time.Time, rec string) string {
	return strings.Replace(rec, "TS", at.Format(time.RFC3339), 1)
}

// A synchronous review's result and a background reviewer's message (a
// meta record) both feed the session's findings; another agent's does not.
func TestTranscript_CollectsReviewFindings(t *testing.T) {
	t0 := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	report := strings.ReplaceAll(sampleReport, "\n", `\n`)
	recs := []string{
		`{"type":"user","timestamp":"TS","cwd":"/work/proj","message":{"role":"user","content":"go"}}`,
		`{"type":"assistant","timestamp":"TS","message":{"content":[{"type":"tool_use","id":"r1","name":"Agent","input":{"subagent_type":"reviewer","description":"Review it"}}]}}`,
		`{"type":"user","timestamp":"TS","message":{"content":[{"type":"tool_result","tool_use_id":"r1","content":"` + report + `"}]}}`,
		`{"type":"assistant","timestamp":"TS","message":{"content":[{"type":"tool_use","id":"r2","name":"Agent","input":{"subagent_type":"reviewer","description":"Review again"}}]}}`,
		`{"type":"user","timestamp":"TS","message":{"content":[{"type":"tool_result","tool_use_id":"r2","content":"Async agent launched successfully.\nagentId: abc123 (internal ID)"}]}}`,
		`{"type":"user","timestamp":"TS","isMeta":true,"message":{"role":"user","content":"Another Claude session sent a message:\n<agent-message from=\"abc123\">\n2. [pkg/store/cache.go:5] same concat mistake\n</agent-message>"}}`,
		`{"type":"user","timestamp":"TS","isMeta":true,"message":{"role":"user","content":"<agent-message from=\"zzz\">\n[pkg/other/x.go:1] not a reviewer\n</agent-message>"}}`,
	}
	var lines []string
	for i, r := range recs {
		lines = append(lines, line(t0.Add(time.Duration(i)*time.Minute), r))
	}
	p := filepath.Join(t.TempDir(), "t.jsonl")
	require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	s := Session{Metrics: Metrics{SessionID: "s"}}
	require.NoError(t, ReadClaudeTranscript(p, &s))
	var files []string
	for _, f := range s.Findings {
		files = append(files, f.File)
	}
	assert.Equal(t, []string{"internal/cli/run.go", "pkg/store/db.go", "pkg/store/cache.go"}, files)
}

func findingSession(id string, files ...string) Metrics {
	m := Metrics{SessionID: id, ActiveSeconds: 600}
	for i, f := range files {
		m.Findings = append(m.Findings, Finding{File: f, Line: i + 1, Summary: "concat in " + f})
	}
	return m
}

func TestDetect_ReviewAreaRepeatsAcrossSessions(t *testing.T) {
	ps := Detect([]Metrics{
		findingSession("a", "pkg/store/db.go"),
		findingSession("b", "pkg/store/cache.go", "cmd/x.go"),
		findingSession("c", "pkg/store/db.go"),
	})
	var store *Pattern
	for i := range ps {
		if ps[i].ID == "review-area:pkg/store" {
			store = &ps[i]
		}
		if ps[i].ID == "review-area:cmd" {
			assert.False(t, ps[i].Repeated)
		}
	}
	require.NotNil(t, store)
	assert.True(t, store.Repeated)
	assert.Contains(t, store.Hits[1].Findings[0], "pkg/store/cache.go:1 concat in pkg/store/cache.go")
	prompt := DraftPrompt(*store, nil)
	assert.Contains(t, prompt, "the lesson is NONE")
	assert.Contains(t, prompt, "pkg/store/cache.go:1")
	c := Candidate{ID: "HL-001", Pattern: store.ID, Evidence: store.Hits, Lesson: "x"}
	assert.Contains(t, RefutePrompt(c, nil), "drawn from review findings")
}

// A drafter that finds no common mistake makes no candidate, and the
// pattern waits like any failed draft.
func TestPropose_ReviewAreaNoneMakesNoCandidate(t *testing.T) {
	root := gitRepo(t)
	ps := Detect([]Metrics{findingSession("a", "pkg/store/db.go"), findingSession("b", "pkg/store/c.go"), findingSession("c", "pkg/store/d.go")})
	var p Pattern
	for _, x := range ps {
		if x.ID == "review-area:pkg/store" {
			p = x
		}
	}
	ask := func(_ context.Context, _, _ string) (Reply, error) {
		return Reply{Output: "LESSON: NONE\nRATIONALE: unrelated"}, nil
	}
	created, errs := Propose(context.Background(), root, []Pattern{p}, []string{"claude", "codex"}, ask, time.Now(), Options{Auto: true})
	assert.Empty(t, created)
	require.Len(t, errs, 1)
	assert.ErrorIs(t, errs[0], ErrNoCommonMistake)
	created, _ = Propose(context.Background(), root, []Pattern{p}, []string{"claude"}, ask, time.Now(), Options{Auto: true})
	assert.Empty(t, created, fmt.Sprintf("backoff holds the pattern: %v", created))
}
