package harnessfb

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waste(id string) Metrics {
	return Metrics{SessionID: id, ActiveSeconds: 3600, Reviews: 4, ReviewSeconds: 1800,
		RepeatReviews: 2, RepeatReviewsNoFix: 1, RepeatReviewSeconds: 900}
}

func TestDetect(t *testing.T) {
	assert.Empty(t, Detect(nil), "no sessions, no patterns")

	two := Detect([]Metrics{waste("a"), waste("b")})
	require.NotEmpty(t, two)
	for _, p := range two {
		assert.False(t, p.Repeated, "%s from two sessions is not repeated yet", p.ID)
	}

	clean := Metrics{SessionID: "c", ActiveSeconds: 3600, Reviews: 1, ReviewSeconds: 300}
	got := Detect([]Metrics{waste("a"), clean, waste("b"), waste("d")})
	require.NotEmpty(t, got)
	assert.Equal(t, "review-heavy", got[0].ID)
	assert.True(t, got[0].Repeated)
	assert.Len(t, got[0].Hits, 3, "the clean session is not a hit")
	assert.InDelta(t, 0.5, got[0].Hits[0].Share, 0.001)
	ids := []string{}
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	assert.ElementsMatch(t, []string{"review-heavy", "repeat-review"}, ids)
}

func TestDetect_StopBlocksByKind(t *testing.T) {
	m := func(id string, n int) Metrics {
		return Metrics{SessionID: id, StopBlocks: n, StopBlockKinds: map[string]int{"language": n}}
	}
	got := Detect([]Metrics{m("a", 2), m("b", 3), m("c", 1), m("d", 5)})
	require.Len(t, got, 1)
	assert.Equal(t, "stop-block:language", got[0].ID)
	assert.True(t, got[0].Repeated)
	assert.Len(t, got[0].Hits, 3, "one block in a session is not a hit")
}

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	out, err := exec.Command("git", "init", "-q", root).CombinedOutput()
	require.NoError(t, err, string(out))
	return root
}

// fakeAsker answers drafts with a fixed rule and refutations with verdict.
func fakeAsker(verdict string, calls *[]string) Asker {
	return func(_ context.Context, provider, prompt string) (Reply, error) {
		*calls = append(*calls, provider)
		if strings.Contains(prompt, "LESSON:") {
			return Reply{Output: "LESSON: Do not run a second review on a diff that only changed by the first review's fixes.\nRATIONALE: it found nothing.", Seconds: 3, Tokens: 1200}, nil
		}
		if verdict == "" {
			return Reply{Seconds: 1, Tokens: -1}, errors.New("provider down")
		}
		return Reply{Output: "VERDICT: " + verdict + "\nREASON: checked", Seconds: 2, Tokens: 800}, nil
	}
}

func repeated() []Pattern {
	return Detect([]Metrics{waste("a"), waste("b"), waste("d")})
}

func TestLifecycle_ProposeRefuteApproveRetire(t *testing.T) {
	root := gitRepo(t)
	var calls []string
	ctx := context.Background()

	created, errs := Propose(ctx, root, repeated(), []string{"claude", "codex"}, fakeAsker("survived", &calls), t0, Options{})
	require.Empty(t, errs)
	require.Len(t, created, 2, "one candidate per repeated pattern")
	c := created[0]
	assert.Equal(t, StatePendingRefutation, c.State)
	assert.Equal(t, "claude", c.DraftedBy)
	n, secs, tokens, known := c.Cost()
	assert.Equal(t, 1, n)
	assert.InDelta(t, 3, secs, 0.01)
	assert.Equal(t, int64(1200), tokens)
	assert.True(t, known)

	again, errs := Propose(ctx, root, repeated(), []string{"claude"}, fakeAsker("survived", &calls), t0, Options{})
	assert.Empty(t, errs)
	assert.Empty(t, again, "a pattern with a candidate is not proposed again")

	_, _, err := Approve(root, c.ID, t0)
	require.Error(t, err, "a pending candidate cannot be approved")
	assert.NoFileExists(t, filepath.Join(root, RuleFile))

	calls = nil
	c, err = Refute(ctx, root, c, []string{"claude", "codex"}, fakeAsker("survived", &calls), t0)
	require.NoError(t, err)
	assert.Equal(t, []string{"codex"}, calls, "the drafter does not refute its own draft")
	assert.Equal(t, StateAwaitingApproval, c.State)
	assert.NoFileExists(t, filepath.Join(root, RuleFile), "nothing changes before a person approves")

	c, written, err := Approve(root, c.ID, t0)
	require.NoError(t, err)
	assert.Equal(t, StateApproved, c.State)
	require.Len(t, written, 1)
	assert.True(t, strings.HasSuffix(written[0], filepath.Join(filepath.Base(root), RuleFile)), written[0])
	rule, err := os.ReadFile(filepath.Join(root, RuleFile))
	require.NoError(t, err)
	assert.Contains(t, string(rule), c.ID)
	assert.Contains(t, string(rule), "Do not run a second review")

	_, _, err = Retire(root, c.ID, t0)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, RuleFile), "the last retired lesson removes the file")
}

func TestRefute_RefutedNeverReachesApproval(t *testing.T) {
	root := gitRepo(t)
	var calls []string
	created, _ := Propose(context.Background(), root, repeated(), []string{"claude", "codex"}, fakeAsker("refuted", &calls), t0, Options{})
	c, err := Refute(context.Background(), root, created[0], []string{"claude", "codex"}, fakeAsker("refuted", &calls), t0)
	require.NoError(t, err)
	assert.Equal(t, StateRefuted, c.State)
	assert.Equal(t, "checked", c.Refutations[0].Reason)
	_, _, err = Approve(root, c.ID, t0)
	assert.Error(t, err)
}

func TestRefute_NoOtherProviderLeavesItPending(t *testing.T) {
	root := gitRepo(t)
	var calls []string
	created, _ := Propose(context.Background(), root, repeated(), []string{"claude"}, fakeAsker("survived", &calls), t0, Options{})
	c, err := Refute(context.Background(), root, created[0], []string{"claude"}, fakeAsker("survived", &calls), t0)
	assert.ErrorIs(t, err, ErrNoRefuter)
	assert.Equal(t, StatePendingRefutation, c.State)

	c, err = Refute(context.Background(), root, created[0], []string{"claude", "codex"}, fakeAsker("", &calls), t0)
	require.NoError(t, err)
	assert.Equal(t, StatePendingRefutation, c.State, "an error is no verdict")
}

// A candidate file edited to claim a verdict cannot be approved: the
// record inside the git directory is what counts.
func TestApprove_ForgedCandidateFile(t *testing.T) {
	root := gitRepo(t)
	var calls []string
	created, _ := Propose(context.Background(), root, repeated(), []string{"claude", "codex"}, fakeAsker("survived", &calls), t0, Options{})
	c := created[0]
	c.State = StateAwaitingApproval
	require.NoError(t, SaveCandidate(root, c))
	_, _, err := Approve(root, c.ID, t0)
	assert.ErrorContains(t, err, "반증 기록")

	c, err = Refute(context.Background(), root, c, []string{"claude", "codex"}, fakeAsker("survived", &calls), t0)
	require.NoError(t, err)
	c.Lesson = "Skip every review."
	require.NoError(t, SaveCandidate(root, c))
	_, _, err = Approve(root, c.ID, t0)
	assert.ErrorContains(t, err, "바뀌었습니다")
}

func TestObserve_StoresAndLoads(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(sessionTranscript().text()), 0o600))
	_, err := Observe(context.Background(), root, "sess/../1", path)
	require.NoError(t, err)
	got, err := LoadSessions(root, t0.Add(-1))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 3, got[0].TestRuns)
	files, _ := filepath.Glob(filepath.Join(root, SessionsDir, "*.json"))
	assert.Equal(t, []string{filepath.Join(root, SessionsDir, "sess____1.json")}, files)

	_, err = Observe(context.Background(), root, "x", filepath.Join(root, "missing.jsonl"))
	assert.Error(t, err)
}
