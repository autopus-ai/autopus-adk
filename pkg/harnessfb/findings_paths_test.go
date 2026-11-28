package harnessfb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func indexFor(roots []string, files ...string) *PathIndex {
	x := &PathIndex{Roots: roots, Files: map[string]bool{}, ByBase: map[string][]string{}}
	for _, f := range files {
		x.Files[f] = true
		b := filepath.Base(f)
		x.ByBase[b] = append(x.ByBase[b], f)
	}
	return x
}

func TestProjectPath_ResolvesAgainstTheRepository(t *testing.T) {
	idx := indexFor([]string{"/repo/.claude/worktrees/feat", "/repo"}, "internal/cli/doctor_text.go", "pkg/a/spec.md", "pkg/b/spec.md")
	cases := map[string]string{
		"/repo/.claude/worktrees/feat/internal/cli/run.go:3":         "internal/cli/run.go",
		"/repo/pkg/store/db.go:1":                                    "pkg/store/db.go",
		"doctor_text.go:9":                                           "internal/cli/doctor_text.go",
		"/repo/.claude/worktrees/gone/internal/cli/doctor_text.go:2": "internal/cli/doctor_text.go",
		"/repo/.claude/worktrees/gone/pkg/new/x.go:2":                "pkg/new/x.go",
		"W/internal/cli/doctor_text.go:5":                            "internal/cli/doctor_text.go",
	}
	for in, want := range cases {
		got := reviewFindings("1. ["+in+"] x", "/elsewhere", idx)
		require.Len(t, got, 1, in)
		assert.Equal(t, want, got[0].File, in)
	}
	for _, in := range []string{"spec.md:3", ".../internal/x.go:2", "/other/repo/a.go:1", "missing.go:4"} {
		assert.Empty(t, reviewFindings("1. ["+in+"] x", "/repo", idx), in)
	}
	// A new file in a subdirectory keeps its own path, even when a tracked
	// file of the same name sits higher up.
	idx2 := indexFor([]string{"/repo"}, "main.go", "src/index.ts", "apps/api/a.go")
	for _, in := range []string{"cmd/newtool/main.go", "apps/web/src/index.ts"} {
		got := reviewFindings("1. ["+in+":5] x", "", idx2)
		require.Len(t, got, 1, in)
		assert.Equal(t, in, got[0].File)
	}
	got := reviewFindings("note: mydb.go is fine but db.go:12 leaks", "", indexFor([]string{"/repo"}, "pkg/store/db.go"))
	require.Len(t, got, 1)
	assert.Equal(t, "note: mydb.go is fine but pkg/store/db.go:12 leaks", got[0].Summary)
}

func TestSummarize_RedactsAndStripsMarkup(t *testing.T) {
	got := summarize("<result>1. [a.go:1] token ghp_" + strings.Repeat("A", 36) + " &lt;leak&gt;</result>")
	assert.NotContains(t, got, "ghp_")
	assert.NotContains(t, got, "<result>")
	assert.Contains(t, got, "<leak>")
	assert.Contains(t, summarize("[a.ts:1] returns Promise<void> early"), "Promise<void>")
	f := reviewFindings("1. [/repo/pkg/store/db.go:4] concat", "", indexFor([]string{"/repo"}, "pkg/store/db.go"))
	require.Len(t, f, 1)
	assert.Equal(t, "[pkg/store/db.go:4] concat", f[0].Summary, "no home path in the summary")
}

// A rewritten (shorter) transcript is read again with the same path index.
func TestTranscript_RewriteKeepsThePathIndex(t *testing.T) {
	idx := indexFor([]string{"/repo"}, "pkg/store/db.go")
	s := Session{Metrics: Metrics{SessionID: "s"}, Paths: idx}
	s.Offset = 1 << 30
	rec := `{"type":"assistant","timestamp":"TS","message":{"content":[{"type":"tool_use","id":"r1","name":"Agent","input":{"subagent_type":"reviewer"}}]}}`
	res := `{"type":"user","timestamp":"TS","message":{"content":[{"type":"tool_result","tool_use_id":"r1","content":"1. [/repo/pkg/store/db.go:1] bug"}]}}`
	readRecords(t, &s, rec, res)
	require.Len(t, s.Findings, 1)
	assert.Equal(t, "pkg/store/db.go", s.Findings[0].File)
}

func TestAddFindings_KeepsTheLatestFivePerArea(t *testing.T) {
	var s Session
	for i := 1; i <= 8; i++ {
		s.addFindings([]Finding{{File: "pkg/store/db.go", Line: i}})
	}
	s.addFindings([]Finding{{File: "pkg/store/db.go", Line: 8}, {File: "cmd/x.go", Line: 1}})
	var lines []int
	for _, f := range s.Findings {
		if f.File == "pkg/store/db.go" {
			lines = append(lines, f.Line)
		}
	}
	assert.Equal(t, []int{4, 5, 6, 7, 8}, lines, "the latest five, once each")
	assert.Len(t, s.Findings, 6)
}

func readRecords(t *testing.T, s *Session, recs ...string) {
	t.Helper()
	t0 := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	var lines []string
	for i, r := range recs {
		lines = append(lines, strings.Replace(r, "TS", t0.Add(time.Duration(i)*time.Minute).Format(time.RFC3339), 1))
	}
	p := filepath.Join(t.TempDir(), "t.jsonl")
	require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	require.NoError(t, ReadClaudeTranscript(p, s))
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

// Every way a background reviewer's report reaches the transcript.
func TestTranscript_ReportDeliveryPaths(t *testing.T) {
	spawn := func(id, result string) []string {
		return []string{
			`{"type":"assistant","timestamp":"TS","message":{"content":[{"type":"tool_use","id":"` + id + `","name":"Agent","input":{"subagent_type":"reviewer"}}]}}`,
			`{"type":"user","timestamp":"TS","message":{"content":[{"type":"tool_result","tool_use_id":"` + id + `","content":` + quote(result) + `}]}}`,
		}
	}
	cases := map[string][]string{
		"notification result": append(spawn("r1", "Async agent launched\nagentId: a1"),
			`{"type":"user","timestamp":"TS","message":{"role":"user","content":`+quote("<task-notification>\n<task-id>a1</task-id>\n<result>1. [pkg/s/a.go:1] bug</result></task-notification>")+`}}`),
		"text block": append(spawn("r1", "Async agent launched\nagentId: a1"),
			`{"type":"user","timestamp":"TS","message":{"content":[{"type":"text","text":`+quote("<agent-message from=\"a1\">\n1. [pkg/s/a.go:1] bug")+`}]}}`),
		"meta list": append(spawn("r1", "Async agent launched\nagentId: a1"),
			`{"type":"user","timestamp":"TS","isMeta":true,"message":{"content":[{"type":"text","text":`+quote("<agent-message from=\"a1\">\n1. [pkg/s/a.go:1] bug")+`}]}}`),
		"queued command": append(spawn("r1", "Async agent launched\nagentId: a1"),
			`{"type":"attachment","timestamp":"TS","attachment":{"type":"queued_command","prompt":`+quote("<agent-message from=\"a1\">\n1. [pkg/s/a.go:1] bug")+`}}`),
		"teammate": append(spawn("r1", "Spawned successfully.\nagent_id: rev1@team\nname: rev1"),
			`{"type":"user","timestamp":"TS","message":{"role":"user","content":`+quote("<teammate-message teammate_id=\"other\">\n[pkg/x/b.go:2] not a reviewer\n</teammate-message>\n<teammate-message teammate_id=\"rev1\" color=\"blue\">\n1. [pkg/s/a.go:1] bug\n</teammate-message>")+`}}`),
	}
	for name, recs := range cases {
		t.Run(name, func(t *testing.T) {
			s := Session{Metrics: Metrics{SessionID: "s"}}
			readRecords(t, &s, recs...)
			require.Len(t, s.Findings, 1)
			assert.Equal(t, "pkg/s/a.go", s.Findings[0].File)
			assert.Equal(t, 0, s.Turns, "a delivered report is not a user turn")
		})
	}
}

// A report read again by an incremental read counts once.
func TestTranscript_IncrementalReadKeepsFindingsOnce(t *testing.T) {
	s := Session{Metrics: Metrics{SessionID: "s"}}
	rec := `{"type":"assistant","timestamp":"TS","message":{"content":[{"type":"tool_use","id":"r1","name":"Agent","input":{"subagent_type":"reviewer"}}]}}`
	res := `{"type":"user","timestamp":"TS","message":{"content":[{"type":"tool_result","tool_use_id":"r1","content":"1. [pkg/s/a.go:1] bug"}]}}`
	readRecords(t, &s, rec, res)
	s.Offset = 0
	readRecords(t, &s, rec, res)
	assert.Len(t, s.Findings, 1)
}

func TestInterleave_EverySessionRecentFirst(t *testing.T) {
	var hits []Hit
	for i := 0; i < 5; i++ {
		h := Hit{SessionID: fmt.Sprintf("s%d", i), End: time.Date(2026, 11, 1+i, 0, 0, 0, 0, time.UTC)}
		for j := 0; j < 5; j++ {
			h.Findings = append(h.Findings, fmt.Sprintf("f%d-%d", i, j))
		}
		hits = append(hits, h)
	}
	got := interleave(hits, 12)
	require.Len(t, got, 12)
	assert.Contains(t, got[0], "s4")
	for i := 0; i < 5; i++ {
		assert.Contains(t, strings.Join(got, "\n"), fmt.Sprintf("s%d", i))
	}
}

func TestSaysNone(t *testing.T) {
	for _, s := range []string{"NONE", "none.", "NONE — unrelated findings", "None: different code", "NONE (unrelated)"} {
		assert.True(t, saysNone(s), s)
	}
	for _, s := range []string{"Nonexistent keys must be checked", "Use none of the timers", "None of the review rounds may restart without an edit in between."} {
		assert.False(t, saysNone(s), s)
	}
}

func TestEffects_ReviewAreaIsNeverARetireCandidate(t *testing.T) {
	decided := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	var sessions []Metrics
	for i := 0; i < 11; i++ { // five before, one on the day, five after
		m := findingSession(fmt.Sprintf("s%d", i), "pkg/store/db.go")
		m.Start = decided.AddDate(0, 0, i-5)
		m.End = m.Start.Add(time.Hour)
		sessions = append(sessions, m)
	}
	es := Effects(sessions, []Candidate{{ID: "HL-1", Pattern: "review-area:pkg/store", Decided: &decided}})
	require.Len(t, es, 1)
	assert.Equal(t, EffectAreaStill, es[0].Verdict)
}
