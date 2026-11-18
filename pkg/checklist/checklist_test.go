package checklist

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

var testNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

func gitRepo(t *testing.T) (string, func(args ...string)) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "-q")
	writeT(t, dir, "src/a.go", "package a\n")
	run("add", "-A")
	run("commit", "-qm", "init")
	return dir, run
}

func writeT(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

func commitCheck(t *testing.T, root string, git func(...string), c Check) {
	t.Helper()
	data, err := c.Format()
	require.NoError(t, err)
	writeT(t, root, filepath.Join(ChecksDir, c.ID+".md"), string(data))
	git("add", "-A")
	git("commit", "-qm", "check "+c.ID)
}

func fakeRunner(exit int) Runner {
	return func(context.Context, string, string) (int, string, error) { return exit, "out\n", nil }
}

func TestParseCheck_RejectsMalformed(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"no front matter", "missing front matter"},
		{"---\nid: CHK-001\nmode: strict\nwhen: {paths: [a]}\nsources: [L-001]\n---\nbody", "unknown mode: strict"},
		{"---\nid: CHK-001\nmode: observe\nsources: [L-001]\n---\nbody", "when needs paths or keywords"},
		{"---\nid: X-1\nmode: observe\nwhen: {paths: [a]}\nsources: [L-001]\n---\nbody", "is not CHK-NNN"},
	} {
		_, err := ParseCheck([]byte(tc.body))
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.want)
	}
	c := Check{ID: "CHK-007", Mode: ModeObserve, When: When{Paths: []string{"src/**"}}, Sources: []string{"L-1"}, Created: "2026-11-18", Body: "criterion"}
	data, err := c.Format()
	require.NoError(t, err)
	back, err := ParseCheck(data)
	require.NoError(t, err)
	assert.Equal(t, c, back)
}

func TestMatchCheck_Semantics(t *testing.T) {
	cs := ChangeSet{
		Paths: []string{"internal/a/b.go", "web/x.ts"},
		Added: map[string][]string{"internal/a/b.go": {"p := filepath.EvalSymlinks(x)"}, "web/x.ts": {"os.Symlink"}},
	}
	cases := []struct {
		when When
		ok   bool
	}{
		{When{Paths: []string{"internal/**/*.go"}}, true},
		{When{Paths: []string{"**"}}, true},
		{When{Paths: []string{"cmd/**"}}, false},
		{When{Keywords: []string{"EvalSymlinks"}}, true},
		{When{Keywords: []string{"evalsymlinks"}}, false},
		// Both set: the keyword must be in a file that matched the paths.
		{When{Paths: []string{"internal/**"}, Keywords: []string{"os.Symlink"}}, false},
		{When{Paths: []string{"web/**"}, Keywords: []string{"os.Symlink"}}, true},
	}
	for i, tc := range cases {
		_, ok := MatchCheck(tc.when, cs)
		assert.Equal(t, tc.ok, ok, "case %d %+v", i, tc.when)
	}
}

// AC-12, AC-13, AC-14, AC-15: skip on mismatch, observe never fails, enforce
// fails, and the checks come from the base tree, not from the change.
func TestRun_ModesAndBaseTree(t *testing.T) {
	root, git := gitRepo(t)
	check := Check{ID: "CHK-001", Mode: ModeObserve, When: When{Paths: []string{"src/**"}}, Verify: "./guard.sh", Sources: []string{"L-001"}, Created: "2026-11-18"}
	commitCheck(t, root, git, check)

	writeT(t, root, "docs/x.md", "x\n")
	rep, err := Run(context.Background(), root, "HEAD", fakeRunner(1), testNow)
	require.NoError(t, err)
	require.Len(t, rep.Outcomes, 1)
	assert.Equal(t, ResultSkipped, rep.Outcomes[0].Result)
	assert.False(t, rep.Failed())

	writeT(t, root, "src/a.go", "package a\n// BUG\n")
	rep, err = Run(context.Background(), root, "HEAD", fakeRunner(1), testNow)
	require.NoError(t, err)
	assert.Equal(t, ResultFlagged, rep.Outcomes[0].Result)
	assert.False(t, rep.Failed(), "observe never fails the run")

	// The change itself promotes the check: not applied until committed.
	check.Mode = ModeEnforce
	data, _ := check.Format()
	writeT(t, root, filepath.Join(ChecksDir, "CHK-001.md"), string(data))
	rep, err = Run(context.Background(), root, "HEAD", fakeRunner(1), testNow)
	require.NoError(t, err)
	assert.False(t, rep.Failed())
	assert.Equal(t, map[string]string{".autopus/checklist/CHK-001.md": "modified"}, rep.ChangedChecks)

	git("add", ChecksDir)
	git("commit", "-qm", "promote")
	rep, err = Run(context.Background(), root, "HEAD", fakeRunner(1), testNow)
	require.NoError(t, err)
	assert.True(t, rep.Failed())

	hits, err := ReadHits(root)
	require.NoError(t, err)
	assert.Len(t, hits, 3, "the skipped run records no hit")
}

func escapeEntry(id, pattern, commit string, ev *learn.Evidence) learn.LearningEntry {
	return learn.LearningEntry{ID: id, Type: learn.EntryTypeEscape, Pattern: pattern, Evidence: ev,
		Escape: &learn.Escape{MissedBy: "unit tests", WhenPaths: []string{"src/**"}, Commit: commit, Provider: "claude"}}
}

func exits(cmd string, before, after int) *learn.Evidence {
	kind := learn.EvidenceLLMOnly
	if before != 0 && after == 0 {
		kind = learn.EvidenceDeterministic
	}
	return &learn.Evidence{Kind: kind, Command: cmd, BeforeExit: &before, AfterExit: &after}
}

// AC-2, AC-3, AC-4, AC-5: once without repro is not eligible, a repeat on
// two commits is one candidate, a reproduced repro is a candidate, a repro
// that did not reproduce is reported.
func TestPropose_Eligibility(t *testing.T) {
	root := t.TempDir()
	entries := []learn.LearningEntry{
		escapeEntry("L-001", "Once only", "c1", nil),
		escapeEntry("L-002", "Repeated  defect", "c1", nil),
		escapeEntry("L-003", "repeated defect", "c2", nil),
		escapeEntry("L-004", "Reproduced", "c3", exits("./r.sh", 1, 0)),
		escapeEntry("L-005", "Did not reproduce", "c4", exits("./n.sh", 0, 0)),
	}
	created, lessons, err := Propose(root, entries, testNow)
	require.NoError(t, err)
	require.Len(t, created, 2)
	assert.Equal(t, []string{"L-002", "L-003"}, created[0].Lessons)
	assert.Equal(t, 2, created[0].Repeats)
	assert.Equal(t, "./r.sh", created[1].Check.Verify)
	for _, c := range created {
		assert.Equal(t, StatePendingRefutation, c.State)
		assert.Equal(t, ModeObserve, c.Check.Mode)
	}
	byID := map[string]LessonStatus{}
	for _, l := range lessons {
		byID[l.ID] = l
	}
	assert.Equal(t, LessonNotEligible, byID["L-001"].Status)
	assert.Equal(t, LessonReproFailed, byID["L-005"].Status)
	assert.Contains(t, byID["L-005"].Detail, "수정 전 exit 0")

	again, _, err := Propose(root, entries, testNow)
	require.NoError(t, err)
	assert.Empty(t, again, "an existing candidate is not proposed twice")
}

// AC-6, AC-7, AC-8, AC-9, AC-10, AC-11: refutation by a different provider,
// refuted candidates leave the queue, no other provider keeps it pending,
// and only approval writes a check.
func TestRefuteApproveReject(t *testing.T) {
	root, _ := gitRepo(t)
	entries := []learn.LearningEntry{
		escapeEntry("L-001", "a", "c1", exits("./a.sh", 1, 0)),
		escapeEntry("L-002", "b", "c2", exits("./b.sh", 1, 0)),
	}
	created, _, err := Propose(root, entries, testNow)
	require.NoError(t, err)
	require.Len(t, created, 2)

	_, err = Refute(context.Background(), root, created[0], []string{"claude"}, nil, testNow)
	assert.ErrorIs(t, err, ErrNoRefuter)

	var asked []string
	ask := func(verdict string) Asker {
		return func(_ context.Context, p, _ string) (string, error) {
			asked = append(asked, p)
			return "VERDICT: " + verdict + "\nREASON: because", nil
		}
	}
	got, err := Refute(context.Background(), root, created[0], []string{"claude", "codex"}, ask("refuted"), testNow)
	require.NoError(t, err)
	assert.Equal(t, StateRefuted, got.State)
	assert.Equal(t, []string{"codex"}, asked, "the drafter is never asked")

	got, err = Refute(context.Background(), root, created[1], []string{"codex", "gemini"}, ask("survived"), testNow)
	require.NoError(t, err)
	assert.Equal(t, StateAwaitingApproval, got.State)

	st, err := BuildStatus(root, entries, 0)
	require.NoError(t, err)
	require.Len(t, st.ApprovalQueue(), 1)
	assert.Equal(t, created[1].ID, st.ApprovalQueue()[0].ID)
	_, err = os.Stat(filepath.Join(root, ChecksDir, created[1].ID+".md"))
	assert.True(t, os.IsNotExist(err), "nothing is added before approval")

	_, err = Approve(root, created[0].ID, testNow)
	assert.Error(t, err, "a refuted candidate cannot be approved")
	check, err := Approve(root, created[1].ID, testNow)
	require.NoError(t, err)
	assert.Equal(t, ModeObserve, check.Mode)
	require.NoError(t, Reject(root, created[0].ID, testNow))
	st, err = BuildStatus(root, entries, 0)
	require.NoError(t, err)
	assert.Empty(t, st.ApprovalQueue())
}

// AC-16, AC-17, AC-18: retirement counts applications, reports the last
// flag, and never edits the check.
func TestStatus_RetireAndPromote(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"CHK-001", "CHK-002", "CHK-003"} {
		c := Check{ID: id, Mode: ModeObserve, When: When{Paths: []string{"**"}}, Verify: "./guard.sh", Sources: []string{"L-1"}, Created: "2026-11-18", Body: "b"}
		data, _ := c.Format()
		writeT(t, root, filepath.Join(ChecksDir, id+".md"), string(data))
	}
	before, _ := os.ReadFile(filepath.Join(root, ChecksDir, "CHK-001.md"))
	var rep Report
	rep.Base = "b"
	for i := 0; i < 3; i++ {
		rep.Outcomes = []Outcome{{ID: "CHK-001", Mode: ModeObserve, Result: ResultClean}, {ID: "CHK-002", Mode: ModeObserve, Result: ResultFlagged}}
		require.NoError(t, RecordHits(root, rep, testNow.Add(time.Duration(i)*time.Minute)))
	}
	st, err := BuildStatus(root, nil, 3)
	require.NoError(t, err)
	byID := map[string]CheckStatus{}
	for _, c := range st.Checks {
		byID[c.ID] = c
	}
	assert.True(t, byID["CHK-001"].Retire)
	assert.Nil(t, byID["CHK-001"].LastFlagged)
	assert.Equal(t, 3, byID["CHK-001"].Applied)
	assert.False(t, byID["CHK-002"].Retire)
	assert.True(t, byID["CHK-002"].Promote)
	assert.False(t, byID["CHK-003"].Retire, "never applied is not enough applications")
	after, _ := os.ReadFile(filepath.Join(root, ChecksDir, "CHK-001.md"))
	assert.Equal(t, before, after)
}

func TestWrites_RefuseSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".autopus"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".autopus", "runtime")))
	rep := Report{Base: "b", Outcomes: []Outcome{{ID: "CHK-001", Result: ResultClean}}}
	assert.Error(t, RecordHits(root, rep, testNow))
	entries, _ := os.ReadDir(outside)
	assert.Empty(t, entries)
}
