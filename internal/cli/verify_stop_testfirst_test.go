package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review B3: a test-first block is not the tree's one block. Without a test
// or a TDD-exempt line the next stop on the same tree blocks again, with the
// same reason and without running the tests (go is off PATH); the exempt
// line then lets it through, still without a run.
func TestVerifyStop_TestFirstBlockStandsUntilAnswered(t *testing.T) {
	session := "tdd-stands"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, addSwapped)
	tddProject(t, dir, "tdd", true)

	first := runStopReply(t, dir, session, "바꿨습니다.")
	require.Equal(t, "block", first.Decision)
	require.Contains(t, first.Reason, "no test changed")

	withoutGo(t)
	again := runStopReply(t, dir, session, "완료했습니다.")
	assert.Equal(t, "block", again.Decision, again.SystemMessage)
	assert.Contains(t, again.Reason, "no test changed")

	exempt := runStopReply(t, dir, session, "정리했습니다.\n\nTDD-exempt: 동작이 같은 순서 변경")
	assert.Equal(t, stopHookOutput{}, exempt, "the recorded run passed; the exempt line is the only thing missing")
	assert.Equal(t, stopHookOutput{}, runStopReply(t, dir, session, "끝"), "the tree is now recorded as passing")
}

// The block limit still ends a session that never answers test-first.
func TestVerifyStop_TestFirstReuseHonoursTheBlockLimit(t *testing.T) {
	session := "tdd-limit"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, addSwapped)
	tddProject(t, dir, "tdd", true)
	for i := 0; i < verifyStopMaxBlocks; i++ {
		require.Equal(t, "block", runStopReply(t, dir, session, "done").Decision, i)
	}
	last := runStopReply(t, dir, session, "done")
	assert.Empty(t, last.Decision)
	assert.Contains(t, last.SystemMessage, "test-first still not shown")
}
