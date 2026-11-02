package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyGateFailure_TypedVerdictsOnly(t *testing.T) {
	t.Parallel()

	assert.Equal(t, FailureTestFailure, classifyGateFailure(GateValidation, "x\nVERDICT: FAIL"))
	assert.Equal(t, FailureReviewRejection, classifyGateFailure(GateReview, "VERDICT: REQUEST_CHANGES"))
	for _, output := range []string{"looks fine", "VERDICT: FAIL\nVERDICT: FAIL", "VERDICT: maybe", "BYPASS"} {
		assert.Equal(t, FailureContractViolation, classifyGateFailure(GateValidation, output), output)
	}
	assert.Equal(t, FailureContractViolation, classifyGateFailure(GateReview, "VERDICT: FAIL"))
}

func TestClassifyDispatchFailure_OnlyTheSentinelIsRetryable(t *testing.T) {
	t.Parallel()

	live := context.Background()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name      string
		ctx       context.Context
		resp      *PhaseResponse
		err       error
		kind      FailureKind
		retryable bool
	}{
		{"marked transport", live, nil, errors.Join(errors.New("dial"), ErrRetryableTransport), FailureTransport, true},
		{"unmarked transport", live, nil, errors.New("connection reset"), FailureTransport, false},
		{"provider timeout", live, &PhaseResponse{FailureClass: "timeout"}, errors.New("timed out"), FailureTimeout, false},
		{"timed out response", live, &PhaseResponse{TimedOut: true, Output: "x"}, nil, FailureTimeout, false},
		{"provider empty output", live, &PhaseResponse{FailureClass: "empty_output"}, errors.New("empty"), FailureContractViolation, false},
		{"non-zero exit", live, &PhaseResponse{ExitCode: 3, Output: "x"}, nil, FailureTransport, false},
		{"nil response", live, nil, nil, FailureContractViolation, false},
		{"cancelled run", cancelled, nil, ErrRetryableTransport, FailureCancelled, false},
		{"deadline", live, nil, context.DeadlineExceeded, FailureCancelled, false},
	}
	for _, tc := range cases {
		kind, retryable := classifyDispatchFailure(tc.ctx, tc.resp, tc.err)
		assert.Equal(t, tc.kind, kind, tc.name)
		assert.Equal(t, tc.retryable, retryable, tc.name)
	}
}

func TestParseFailureEvidence_ReportLines(t *testing.T) {
	t.Parallel()

	tests := parseFailureEvidence(FailureTestFailure, "EXPECTED: 1\nACTUAL: 2\n"+
		"FAILED_COMMAND: go vet ./...\nACTUAL: exit 1\nnoise")
	require.Len(t, tests, 2)
	assert.Equal(t, FailureEvidence{Expected: "1", Actual: "2"}, tests[0])
	assert.Equal(t, FailureEvidence{Command: "go vet ./...", Actual: "exit 1"}, tests[1])

	findings := parseFailureEvidence(FailureReviewRejection,
		"FINDING SEC-2: token logged at log.go:9 |FIX:   drop the field\nFINDING F-3: no fix given\nFINDING : bad")
	require.Len(t, findings, 2)
	assert.Equal(t, FailureEvidence{ID: "SEC-2", Detail: "token logged at log.go:9", Fix: "drop the field"}, findings[0])
	assert.Equal(t, FailureEvidence{ID: "F-3", Detail: "no fix given"}, findings[1])

	many := strings.Repeat("FAILED_COMMAND: x\n", failureEvidenceMaxItems+5)
	assert.Len(t, parseFailureEvidence(FailureTestFailure, many), failureEvidenceMaxItems)
}

func TestFailureExcerpt_KeepsTheTailWithoutTheVerdict(t *testing.T) {
	t.Parallel()

	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, "line")
	}
	lines = append(lines, "last line", "VERDICT: FAIL")
	excerpt := failureExcerpt(strings.Join(lines, "\n"))
	assert.Equal(t, failureExcerptMaxLines, len(strings.Split(excerpt, "\n")))
	assert.True(t, strings.HasSuffix(excerpt, "last line"))
	assert.NotContains(t, excerpt, "VERDICT")
	assert.LessOrEqual(t, len(failureExcerpt(strings.Repeat("é", 3000))), failureExcerptMaxBytes)
}

// The breaker trips when a fix resolves nothing, also when the failure grew;
// a failure that dropped any earlier signature is progress.
func TestMadeNoProgress(t *testing.T) {
	t.Parallel()

	failure := func(ids ...string) *PhaseFailure {
		f := &PhaseFailure{PhaseID: PhaseReview, Kind: FailureReviewRejection}
		for _, id := range ids {
			f.Evidence = append(f.Evidence, FailureEvidence{ID: id})
		}
		f.Fingerprint = f.fingerprint()
		return f
	}
	assert.True(t, madeNoProgress(failure("F-1", "F-2"), failure("F-2", "F-1")))
	assert.True(t, madeNoProgress(failure("F-1"), failure("F-1", "F-9")))
	assert.False(t, madeNoProgress(failure("F-1", "F-2"), failure("F-2", "F-3")))
	assert.False(t, madeNoProgress(nil, failure("F-1")))
	free := func(text string) *PhaseFailure {
		f := &PhaseFailure{PhaseID: PhaseValidate, Kind: FailureTestFailure, Excerpt: text}
		f.Fingerprint = f.fingerprint()
		return f
	}
	assert.True(t, madeNoProgress(free("panic:  nil map"), free("panic: nil map")))
	assert.False(t, madeNoProgress(free("panic: nil map"), free("panic: index out of range")))
}
