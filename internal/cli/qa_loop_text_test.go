package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
)

// The lane runs once more after the last fix to check it, so the summary
// counts lane runs and fix attempts apart instead of "2 of max 1".
func TestWriteQALoopText_CountsFixAttemptsAgainstTheCap(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	writeQALoopText(&out, qaloop.Report{
		RunID: "qaloop-1", Branch: "autopus/qa-loop/qaloop-1", StopReason: qaloop.StopPassed, MaxIterations: 1,
		Iterations: []qaloop.Iteration{
			{N: 1, RunStatus: "failed", Agent: &qaloop.AgentRecord{}, Commit: "abc"},
			{N: 2, RunStatus: "passed"},
		},
	})
	assert.Contains(t, out.String(), "lane runs: 2; fix attempts: 1 of max 1\n")
	assert.Contains(t, out.String(), "(1 fix commit(s))")
}
