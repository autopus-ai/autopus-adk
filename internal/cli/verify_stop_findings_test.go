package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const passingEdit = "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n"

func writeFindings(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, reviewFindingsPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func findingsSession(t *testing.T) string {
	session := "findings-" + strings.ReplaceAll(t.Name(), "/", "_")
	resetStopState(session)
	t.Cleanup(func() { resetStopState(session) })
	return session
}

func TestVerifyStopFindings_NoFileChangesNothing(t *testing.T) {
	dir := newStopRepo(t, passingEdit)
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, findingsSession(t)))
}

func TestVerifyStopFindings_OpenFindingBlocksOnTheSameTree(t *testing.T) {
	session := findingsSession(t)
	dir := newStopRepo(t, passingEdit)
	writeFindings(t, dir, `[{"id":"F1","severity":"high","summary":"add.go:4 drops negatives",
		"repro":"go run ./cmd/x -n -1","expected":"-1","actual":"0","status":"open","evidence":""}]`)

	for i := 0; i < 2; i++ {
		got := runStop(t, dir, session)
		require.Equal(t, "block", got.Decision, "stop %d", i)
		assert.Contains(t, got.Reason, "F1 [open]: add.go:4 drops negatives")
		assert.Contains(t, got.Reason, "repro: go run ./cmd/x -n -1")
		assert.Contains(t, got.Reason, reviewFindingsPath)
	}
}

func TestVerifyStopFindings_AllFixedWithEvidencePasses(t *testing.T) {
	session := findingsSession(t)
	dir := newStopRepo(t, passingEdit)
	writeFindings(t, dir, `{"findings":[
		{"id":"F1","summary":"a","repro":"true","status":"fixed","evidence":"$ true\nexit 0"},
		{"id":"F2","summary":"b","repro":"true","status":"rebutted","evidence":"$ git diff shows no such path"}]}`)

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session))
}

func TestVerifyStopFindings_FixedWithoutEvidenceBlocks(t *testing.T) {
	session := findingsSession(t)
	dir := newStopRepo(t, passingEdit)
	writeFindings(t, dir, `[{"id":"F1","summary":"a","repro":"true","status":"fixed","evidence":"  "},
		{"id":"F2","summary":"b","repro":"true","status":"fixed","evidence":"ran it"}]`)

	got := runStop(t, dir, session)
	require.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "F1 [status fixed without evidence]")
	assert.NotContains(t, got.Reason, "F2")
}

func TestVerifyStopFindings_CapTurnsTheBlockIntoANote(t *testing.T) {
	session := findingsSession(t)
	dir := newStopRepo(t, passingEdit)
	writeFindings(t, dir, `[{"id":"F1","summary":"a","repro":"false","status":"open"}]`)

	for i := 0; i < verifyStopMaxBlocks; i++ {
		require.Equal(t, "block", runStop(t, dir, session).Decision, "stop %d", i)
	}
	last := runStop(t, dir, session)
	assert.Empty(t, last.Decision, "the session may end after the block limit")
	assert.Contains(t, last.SystemMessage, "still unresolved")
	assert.Contains(t, last.SystemMessage, "F1 [open]")
}

func TestVerifyStopFindings_BuildFailureComesFirst(t *testing.T) {
	session := findingsSession(t)
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")
	writeFindings(t, dir, `[{"id":"F1","summary":"a","repro":"false","status":"open"}]`)

	got := runStop(t, dir, session)
	require.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "Add(1, 2) != 3")
	assert.NotContains(t, got.Reason, "F1")
}

func TestReviewFindings_EnforcesTheFiveMostSevereAndRejectsBadJSON(t *testing.T) {
	dir := t.TempDir()
	writeFindings(t, dir, `[{"id":"L1","severity":"low","status":"open"},
		{"id":"H1","severity":"high","status":"fixed","evidence":"x"},
		{"id":"C1","severity":"critical","status":"open"},
		{"id":"M1","severity":"medium","status":"open"},
		{"id":"H2","severity":"high","status":"open"},
		{"id":"M2","severity":"medium","status":"open"}]`)
	got := strings.Join(unresolvedFindings(dir), "\n")
	for _, id := range []string{"C1", "H2", "M1", "M2"} {
		assert.Contains(t, got, id)
	}
	assert.NotContains(t, got, "L1", "the sixth most severe is not enforced")

	writeFindings(t, dir, `{not json`)
	bad := unresolvedFindings(dir)
	require.Len(t, bad, 1)
	assert.Contains(t, bad[0], "cannot be read")
}

func TestFollowupReasons_AskForTheFindingsFile(t *testing.T) {
	for _, reason := range []string{followupLightReason, followupFullReason} {
		assert.Contains(t, reason, reviewFindingsPath)
		assert.Contains(t, reason, `"status": "fixed"`)
		assert.Contains(t, reason, `"evidence"`)
	}
}
