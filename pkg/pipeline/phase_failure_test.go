package pipeline

import (
	"context"
	"errors"
	"fmt"
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

func TestClassifyDispatchFailure_Kinds(t *testing.T) {
	t.Parallel()

	live := context.Background()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		resp *PhaseResponse
		err  error
		kind FailureKind
	}{
		{"unmarked transport", live, nil, errors.New("connection reset"), FailureTransport},
		{"provider timeout", live, &PhaseResponse{FailureClass: "timeout"}, errors.New("timed out"), FailureTimeout},
		{"timed out response", live, &PhaseResponse{TimedOut: true, Output: "x"}, nil, FailureTimeout},
		{"provider empty output", live, &PhaseResponse{FailureClass: "empty_output"}, errors.New("empty"), FailureContractViolation},
		{"non-zero exit", live, &PhaseResponse{ExitCode: 3, Output: "x"}, nil, FailureTransport},
		{"nil response", live, nil, nil, FailureContractViolation},
		{"cancelled run", cancelled, nil, ErrRetryableTransport, FailureCancelled},
		{"deadline", live, nil, context.DeadlineExceeded, FailureCancelled},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.kind, classifyDispatchFailure(tc.ctx, tc.resp, tc.err), tc.name)
	}
}

func TestIsRetryableTransport_OnlyTheSentinelInALiveRun(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	marked := fmt.Errorf("dial: %w", ErrRetryableTransport)
	assert.True(t, isRetryableTransport(context.Background(), marked))
	assert.False(t, isRetryableTransport(cancelled, marked))
	assert.False(t, isRetryableTransport(context.Background(), errors.New("dial: connection refused")))
	assert.False(t, isRetryableTransport(context.Background(), nil))
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

	long := strings.Repeat("x", 1000)
	many := strings.Repeat("FAILED_COMMAND: go test\nACTUAL: "+long+"\n", 25)
	assert.Len(t, parseFailureEvidence(FailureTestFailure, many), 25, "parsing keeps every item untruncated")
}

// S14: the display form says where it was cut and how much was left out.
func TestBoundEvidence_MarksTruncationAndCountsOmittedItems(t *testing.T) {
	t.Parallel()

	items := make([]FailureEvidence, 25)
	for i := range items {
		items[i] = FailureEvidence{Command: fmt.Sprintf("go test ./p%d", i), Actual: strings.Repeat("y", 1000)}
	}
	bounded, omitted := boundEvidence(items)

	require.Len(t, bounded, failureEvidenceMaxItems)
	assert.Equal(t, 5, omitted)
	assert.Equal(t, strings.Repeat("y", 400)+" [truncated: 400 of 1000 bytes shown]", bounded[0].Actual)
	assert.Equal(t, "go test ./p0", bounded[0].Command, "short fields are kept whole")
	feedback := renderRemediationFeedback(&PhaseFailure{PhaseID: PhaseValidate, Kind: FailureTestFailure,
		Evidence: bounded, EvidenceOmitted: omitted})
	assert.Contains(t, feedback, "5 more not shown")
	assert.Contains(t, feedback, "[truncated: 400 of 1000 bytes shown]")
}

// S14: two failures that differ only past the display bounds are different
// failures to the breaker.
func TestFailureSignatures_UseUntruncatedValues(t *testing.T) {
	t.Parallel()

	prefix := strings.Repeat("z", 500)
	tail := func(i int, last string) []FailureEvidence {
		items := make([]FailureEvidence, 25)
		for n := range items {
			items[n] = FailureEvidence{Command: fmt.Sprintf("go test ./p%d", n), Actual: "ok"}
		}
		items[i].Actual = prefix + last
		return items
	}
	assert.NotEqual(t, failureSignatures(tail(0, "a"), ""), failureSignatures(tail(0, "b"), ""),
		"a difference after byte 400 is still a difference")
	assert.NotEqual(t, failureSignatures(tail(22, "a"), ""), failureSignatures(tail(22, "b"), ""),
		"a difference in an omitted item is still a difference")
	assert.Equal(t, failureSignatures(tail(3, "a"), ""), failureSignatures(tail(3, "a"), ""))
}

// S15: a finding is identified by what it says, not by the reviewer's ID.
func TestFailureSignatures_FindingIdentityIsContentNotID(t *testing.T) {
	t.Parallel()

	first := []FailureEvidence{{ID: "F-1", Detail: "Handler drops ctx (engine.go:42)", Fix: "return ctx.Err()"}}
	renumbered := []FailureEvidence{{ID: "F-7", Detail: "handler  drops ctx (engine.go:57)", Fix: "Return ctx.Err()"}}
	reused := []FailureEvidence{{ID: "F-1", Detail: "cache key ignores tenant (cache.go:10)", Fix: "include tenant"}}
	assert.Equal(t, failureSignatures(first, ""), failureSignatures(renumbered, ""),
		"the same problem under a new ID and moved line is the same finding")
	assert.NotEqual(t, failureSignatures(first, ""), failureSignatures(reused, ""),
		"a new problem under a reused ID is a different finding")
	bare := failureSignatures([]FailureEvidence{{ID: "F-1"}}, "")
	assert.NotEqual(t, bare, failureSignatures([]FailureEvidence{{ID: "F-2"}}, ""),
		"a finding with no text falls back to its ID")
}

func TestFailureExcerpt_MarksOmittedOutput(t *testing.T) {
	t.Parallel()

	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, "line")
	}
	lines = append(lines, "last line", "VERDICT: FAIL")
	excerpt := failureExcerpt(strings.Join(lines, "\n"))
	excerptLines := strings.Split(excerpt, "\n")
	assert.Equal(t, "[earlier output omitted: 40 of 61 lines, at most 2000 bytes, shown]", excerptLines[0])
	assert.Len(t, excerptLines, failureExcerptMaxLines+1)
	assert.True(t, strings.HasSuffix(excerpt, "last line"))
	assert.NotContains(t, excerpt, "VERDICT")
	assert.Equal(t, "short\noutput", failureExcerpt("short\noutput\nVERDICT: FAIL"), "nothing omitted, no marker")
	wide := failureExcerpt(strings.Repeat("é", 3000))
	assert.True(t, strings.HasPrefix(wide, "[earlier output omitted:"))
	assert.LessOrEqual(t, len(wide), failureExcerptMaxBytes+80)
}

// The breaker trips when a fix resolves nothing, also when the failure grew;
// a failure that dropped any earlier signature is progress.
func TestMadeNoProgress(t *testing.T) {
	t.Parallel()

	review := func(findings ...string) *PhaseFailure {
		var lines []string
		for i, finding := range findings {
			lines = append(lines, fmt.Sprintf("FINDING F-%d: %s | FIX: fix %s", i+1, finding, finding))
		}
		return newGateFailure(Phase{ID: PhaseReview, Gate: GateReview}, 1, FailureReviewRejection,
			strings.Join(append(lines, "VERDICT: REQUEST_CHANGES"), "\n"))
	}
	assert.True(t, madeNoProgress(review("a", "b"), review("b", "a")))
	assert.True(t, madeNoProgress(review("a"), review("a", "z")))
	assert.False(t, madeNoProgress(review("a", "b"), review("b", "c")))
	assert.False(t, madeNoProgress(nil, review("a")))
	free := func(text string) *PhaseFailure {
		return newGateFailure(Phase{ID: PhaseValidate, Gate: GateValidation}, 1, FailureTestFailure, text+"\nVERDICT: FAIL")
	}
	assert.True(t, madeNoProgress(free("panic:  nil map"), free("panic: nil map")))
	assert.False(t, madeNoProgress(free("panic: nil map"), free("panic: index out of range")))
}

// S13: a redacted error keeps its identity and loses its secret.
func TestRedactError_KeepsIdentityAndRedactsMessage(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("provider rejected the call")
	raw := fmt.Errorf("%w: Authorization: Bearer sk-live-0123456789abcdef", sentinel)
	redacted := redactError(raw)

	assert.ErrorIs(t, redacted, sentinel)
	assert.NotContains(t, redacted.Error(), "sk-live-0123456789abcdef")
	assert.Contains(t, redacted.Error(), "[REDACTED_SECRET]")
	clean := errors.New("phase plan: connection reset")
	assert.Same(t, clean, redactError(clean), "an error with nothing to redact is returned as is")
	assert.NoError(t, redactError(nil))
}
