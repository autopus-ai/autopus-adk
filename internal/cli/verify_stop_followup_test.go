package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// largeStopChange is a passing edit of more than followupFullLines code lines.
func largeStopChange() string {
	var body strings.Builder
	body.WriteString("package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return a + b }\n")
	for i := 0; i < 45; i++ {
		fmt.Fprintf(&body, "\nvar filler%d = %d\n", i, i)
	}
	return body.String()
}

// The verification round is on by default; AUTOPUS_FOLLOWUP=off turns it off.
func TestVerifyStop_FollowupIsOnByDefault(t *testing.T) {
	optInFollowup(t, "")
	session := "followup-default"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, largeStopChange())

	assert.Equal(t, "block", runStop(t, dir, session).Decision)
}

func TestVerifyStop_FollowupEnabled(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{
		{"", true}, {"on", true}, {"ON", true}, {"off", false}, {"Off", false}, {"0", false}, {"false", false}, {"maybe", true},
	} {
		t.Setenv("AUTOPUS_FOLLOWUP", tc.env)
		assert.Equal(t, tc.want, followupEnabled(false), "env=%q", tc.env)
	}
}

func TestVerifyStop_LargePassingChangeGetsTheFullRoundOnce(t *testing.T) {
	optInFollowup(t, "")
	session := "followup"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	body := largeStopChange()
	dir := newStopRepo(t, body)

	first := runStop(t, dir, session)
	assert.Equal(t, "block", first.Decision)
	assert.Contains(t, first.Reason, "full verification round")
	assert.Contains(t, first.Reason, "qa-explorer")
	assert.Contains(t, first.Reason, "AskUserQuestion")
	assert.Contains(t, first.Reason, "auto qa intent", "user scenarios come before the hunt")
	assert.Contains(t, first.Reason, "--no-loop", "the tree is dirty when the hook fires, and the loop refuses a dirty tree")
	assert.Contains(t, first.Reason, "policy_mismatch")

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session), "the same tree is not sent round again")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body+"\nvar more = 1\n"), 0o644))
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session), "a fix after the round does not start another")
}

// A one-line logic change is exactly where a written policy breaks, so it is
// verified too, only less deeply.
func TestVerifyStop_SmallBehaviorChangeGetsTheLightRound(t *testing.T) {
	optInFollowup(t, "")
	session := "small"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")

	out := runStop(t, dir, session)
	assert.Equal(t, "block", out.Decision)
	assert.Contains(t, out.Reason, "light verification round")
	assert.Contains(t, out.Reason, "auto qa exec --all")
	assert.NotContains(t, out.Reason, "auto qa intent", "a small change does not generate new scenarios")
}

func TestVerifyStop_CommentOnlyChangeEndsAtGreen(t *testing.T) {
	optInFollowup(t, "")
	session := "comment-only"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums two numbers.\nfunc Add(a, b int) int { return a + b }\n")

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session), "a documented function behaves the same")
}

func TestVerifyStop_FollowupOffEndsAtGreen(t *testing.T) {
	optInFollowup(t, "off")
	session := "followup-off"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, largeStopChange())

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session))
}

func TestVerifyStop_QAOutputAndLogsDoNotGrowTheChange(t *testing.T) {
	optInFollowup(t, "")
	session := "qa-output"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")
	big := strings.Repeat("line\n", 500)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "qa", "hunt"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".autopus", "qa", "hunt", "memory.md"), []byte(big), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.log"), []byte(big), 0o644))

	assert.Contains(t, runStop(t, dir, session).Reason, "light verification round", "only product code counts toward the round")
}

var (
	noBehavior    = verifygate.Behavior{}
	smallBehavior = verifygate.Behavior{Lines: 4}
	largeBehavior = verifygate.Behavior{Lines: 60}
)

func TestVerifyStop_FollowupRoundsAreBounded(t *testing.T) {
	useStopStateDir(t, t.TempDir())
	root := t.TempDir()
	assert.Equal(t, followupNone, nextFollowup("rounds", root, "fp1", noBehavior), "no behavior change, no round, and no round used up")
	assert.Equal(t, followupFull, nextFollowup("rounds", root, "fp1", largeBehavior))
	assert.Equal(t, followupNone, nextFollowup("rounds", root, "fp2", verifygate.Behavior{Lines: 500, NewFiles: []string{"x.go"}}), "nothing follows a full round")
}

// A session whose light round came early is escalated once when its change
// grows into a feature; fixing the light round's findings alone is not
// growth.
func TestVerifyStop_LightRoundEscalatesOnceOnRealGrowth(t *testing.T) {
	useStopStateDir(t, t.TempDir())
	root := t.TempDir()
	assert.Equal(t, followupLight, nextFollowup("grow", root, "fp1", smallBehavior))
	assert.Equal(t, followupNone, nextFollowup("grow", root, "fp2", verifygate.Behavior{Lines: 9}), "the light round's fixes")
	assert.Equal(t, followupNone, nextFollowup("grow", root, "fp3", verifygate.Behavior{Lines: 43}), "past 40 lines, but only 39 more than at the light round")
	assert.Equal(t, followupFull, nextFollowup("grow", root, "fp4", verifygate.Behavior{Lines: 44}), "40 more lines than at the light round")
	assert.Equal(t, followupNone, nextFollowup("grow", root, "fp5", verifygate.Behavior{Lines: 400}), "one escalation only")

	assert.Equal(t, followupLight, nextFollowup("newfile", root, "fp6", smallBehavior))
	assert.Equal(t, followupFull, nextFollowup("newfile", root, "fp7", verifygate.Behavior{Lines: 6, NewFiles: []string{"cmd/new.go"}}), "a new product file escalates")
}

// A record written by the one-round format counts as a round already run.
func TestReadSessionRound_OldFormatCountsAsDone(t *testing.T) {
	useStopStateDir(t, t.TempDir())
	writeStopState("old", "followup", "1 light")
	assert.Equal(t, followupFull, readSessionRound("old").tier)
	writeStopState("new", "followup", sessionRound{tier: followupLight, lines: 7, newFiles: 1}.String())
	assert.Equal(t, sessionRound{tier: followupLight, lines: 7, newFiles: 1}, readSessionRound("new"))
	assert.Equal(t, sessionRound{}, readSessionRound("none"))
}

// A tree one session already sent round is not sent again by the next
// session, so a session that only asks a question is not blocked by
// uncommitted work it did not make; a changed tree is.
func TestVerifyStop_FollowupSkipsATreeAnotherSessionVerified(t *testing.T) {
	useStopStateDir(t, t.TempDir())
	root := t.TempDir()
	assert.Equal(t, followupFull, nextFollowup("session-a", root, "fp1", largeBehavior))
	assert.Equal(t, followupNone, nextFollowup("session-b", root, "fp1", largeBehavior))
	assert.Equal(t, followupLight, nextFollowup("session-c", root, "fp2", smallBehavior))
}

// When the round cannot be recorded it would block every stop forever, so
// the hook fails open.
func TestVerifyStop_FollowupFailsOpenWhenStateCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	unsafe := filepath.Join(dir, "state")
	require.NoError(t, os.Symlink(t.TempDir(), unsafe))
	useStopStateDir(t, unsafe)
	assert.Equal(t, followupNone, nextFollowup("ro", t.TempDir(), "fp", largeBehavior))
}

func TestFollowupTierFor(t *testing.T) {
	t.Parallel()
	assert.Equal(t, followupNone, followupTierFor(verifygate.Behavior{}))
	assert.Equal(t, followupLight, followupTierFor(verifygate.Behavior{Lines: 1}))
	assert.Equal(t, followupFull, followupTierFor(verifygate.Behavior{Lines: followupFullLines}))
	assert.Equal(t, followupFull, followupTierFor(verifygate.Behavior{Lines: 2, NewFiles: []string{"cmd/new.go"}}))
	assert.Equal(t, followupFull, followupTierFor(verifygate.Behavior{Unmeasured: true}), "a diff git could not produce is not nothing")
}

// A shell script has no build toolchain, but its logic is still behavior.
func TestVerifyStop_UnbuiltLanguageStillGetsTheRound(t *testing.T) {
	optInFollowup(t, "")
	session := "unbuilt"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "refund.sh"), []byte("#!/usr/bin/env bash\n[ \"$1\" -le 14 ] && echo approved\n"), 0o755))

	out := runStop(t, dir, session)
	assert.Equal(t, "block", out.Decision)
	assert.Contains(t, out.Reason, "full verification round", "a new product file")
	assert.Contains(t, out.Reason, "No build or test toolchain covers these files")
}

// The round tells the session to fix what it found, so the fixed tree is the
// verified one: a later session that only asks a question is not sent round
// on it again.
func TestVerifyStop_FixedTreeIsNotSentRoundAgainByTheNextSession(t *testing.T) {
	optInFollowup(t, "")
	body := largeStopChange()
	dir := newStopRepo(t, body)
	assert.Equal(t, "block", runStop(t, dir, "fix-a").Decision)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body+"\nvar fixed = 1\n"), 0o644))
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, "fix-a"), "the fix does not start a second round")
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, "fix-b"), "nor does the next session on the fixed tree")
}

// End to end: a small first turn gets the light round, the feature grown
// over later turns gets the full round once, and its fixes get nothing.
func TestVerifyStop_InteractiveSessionIsEscalatedToTheFullRound(t *testing.T) {
	optInFollowup(t, "")
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")
	first := runStop(t, dir, "interactive")
	assert.Contains(t, first.Reason, "light verification round")

	body := largeStopChange()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body), 0o644))
	grown := runStop(t, dir, "interactive")
	assert.Contains(t, grown.Reason, "full verification round")
	assert.Contains(t, grown.Reason, "grown since this session's light round")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body+"\nvar fixed = 1\n"), 0o644))
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, "interactive"))
}
