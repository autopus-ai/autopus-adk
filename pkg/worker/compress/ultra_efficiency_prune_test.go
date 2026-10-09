package compress

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestDefaultCompressor_BelowThresholdSuccessfulPairs_SoftPrunesOlderPairs(t *testing.T) {
	// Given: four completed successful pairs below the hard threshold.
	input := successfulPairTrace(4)
	if ShouldCompress(input, "claude") {
		t.Fatal("fixture must remain below the hard threshold")
	}

	// When.
	result := NewDefaultCompressor(2).CompressDetailed("executor", input, "claude")

	// Then: two recent pairs remain and two older pairs become evidence records.
	if result.Blocker != "" {
		t.Fatalf("unexpected blocker: %s", result.Blocker)
	}
	if !result.Event.CompactionApplied {
		t.Fatal("soft pruning must be reported as applied")
	}
	if result.Event.PrunedPairCount != 2 || result.Event.IncompletePairCount != 0 {
		t.Fatalf("event counts = pruned:%d incomplete:%d", result.Event.PrunedPairCount, result.Event.IncompletePairCount)
	}
	if !containsString(result.Event.ReasonCodes, ReasonSoftPrune) || !containsString(result.Event.ReasonCodes, ReasonToolPairPruned) {
		t.Fatalf("missing soft-prune reasons: %#v", result.Event.ReasonCodes)
	}
	if containsString(result.Event.ReasonCodes, ReasonThresholdExceeded) {
		t.Fatalf("soft prune must not claim hard threshold: %#v", result.Event.ReasonCodes)
	}
	for _, pair := range []string{"pair-1", "pair-2"} {
		pattern := regexp.MustCompile(`\[tool_pair pruned: status=success pair=` + pair + ` .*digest=sha256:[a-f0-9]{64} artifact_ref=phase-output:sha256:[a-f0-9]{64} .*excerpt="[^"]+"\]`)
		if !pattern.MatchString(result.Output) {
			t.Fatalf("missing complete evidence record for %s:\n%s", pair, result.Output)
		}
	}
	for _, pair := range []string{"pair-3", "pair-4"} {
		if !strings.Contains(result.Output, `"pair_id":"`+pair+`"`) {
			t.Fatalf("recent pair %s was not preserved:\n%s", pair, result.Output)
		}
	}
}

func TestDefaultCompressor_SoftPruneProtectedAndIncomplete_FailsClosed(t *testing.T) {
	// Given: protected context, an explicit failed pair, an incomplete pair, and
	// enough successful pairs to activate soft pruning.
	input := strings.Join([]string{
		"Open security finding SEC-17 must remain.",
		"User correction: do not edit migration history.",
		"Migration invariant: schema version stays monotonic.",
		"Acceptance criterion AC-UTE-12 must pass.",
		"Decision: preserve pkg/worker/compress/compressor.go.",
		"reasoning_signature=sig-required-123",
		`<tool_call>{"pair_id":"failed-1","ordinal":8,"command":"failed command"}</tool_call>`,
		`<tool_result>{"pair_id":"failed-1","ordinal":8,"status":"failure","body":"failure evidence"}</tool_result>`,
		successfulPairTrace(4),
		`<tool_call>{"pair_id":"pending-1","ordinal":9,"command":"unfinished"}</tool_call>`,
	}, "\n")

	// When.
	result := NewDefaultCompressor(2).CompressDetailed("reviewer", input, "claude")

	// Then: integrity uncertainty blocks the next phase without pruning protected evidence.
	if result.Blocker != ReasonIncompleteToolPair {
		t.Fatalf("blocker = %q, want %q", result.Blocker, ReasonIncompleteToolPair)
	}
	if result.Event.IncompletePairCount != 1 || !containsString(result.Event.ReasonCodes, ReasonIncompleteToolPair) {
		t.Fatalf("incomplete evidence missing: %#v", result.Event)
	}
	for _, protected := range []string{
		"Open security finding SEC-17", "User correction", "Migration invariant",
		"AC-UTE-12", "pkg/worker/compress/compressor.go", "reasoning_signature=sig-required-123",
		"failed command", "failure evidence", "pending-1",
	} {
		if !strings.Contains(result.Output, protected) {
			t.Fatalf("protected evidence %q was lost:\n%s", protected, result.Output)
		}
	}
}

func TestDefaultCompressor_IncompletePairWithoutPruneEligibility_FailsClosed(t *testing.T) {
	for _, successfulPairs := range []int{0, 2} {
		t.Run(fmt.Sprintf("successful_pairs_%d", successfulPairs), func(t *testing.T) {
			input := successfulPairTrace(successfulPairs) + `
<tool_call>{"pair_id":"pending","ordinal":9,"command":"unfinished"}</tool_call>`

			result := NewDefaultCompressor(2).CompressDetailed("executor", input, "claude")

			if result.Blocker != ReasonIncompleteToolPair {
				t.Fatalf("blocker = %q, want %q", result.Blocker, ReasonIncompleteToolPair)
			}
			if result.Event.IncompletePairCount != 1 {
				t.Fatalf("incomplete count = %d, want 1", result.Event.IncompletePairCount)
			}
		})
	}
}

func TestDefaultCompressor_SoftPruneEvent_RedactsSecretsAndAbsolutePaths(t *testing.T) {
	// Joined from fragments so no provider-format literal is committed; the
	// value is byte-identical to the literal it replaced.
	const secret = "sk-" + "test-1234567890abcdef"
	const localPath = "/Users/alice/private/provider.json"
	input := strings.Replace(successfulPairTrace(4),
		`read pkg/example/file-1.go`,
		`read pkg/worker/compress/compressor.go at `+localPath+` token=`+secret,
		1,
	)

	result := NewDefaultCompressor(2).CompressDetailed("executor", input, "claude")
	eventJSON, err := json.Marshal(result.Event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	for _, forbidden := range []string{secret, localPath} {
		if strings.Contains(result.Output, forbidden) || strings.Contains(string(eventJSON), forbidden) {
			t.Fatalf("unsafe value %q leaked through soft pruning", forbidden)
		}
	}
	if !strings.Contains(result.Output, "pkg/worker/compress/compressor.go") {
		t.Fatalf("safe source reference missing from evidence record:\n%s", result.Output)
	}
	for _, reason := range []string{ReasonSecretRedacted, ReasonLocalPathRedacted} {
		if !containsString(result.Event.ReasonCodes, reason) {
			t.Fatalf("missing redaction reason %q in %#v", reason, result.Event.ReasonCodes)
		}
	}
}

func TestDefaultCompressor_AboveThreshold_RetainsHardSummarization(t *testing.T) {
	oldWindow, ok := ModelWindows["t7-hard"]
	ModelWindows["t7-hard"] = 4000
	t.Cleanup(func() {
		if ok {
			ModelWindows["t7-hard"] = oldWindow
		} else {
			delete(ModelWindows, "t7-hard")
		}
	})

	input := "## Goal\nRetain hard fallback.\n\n" + strings.Repeat("large trace ", 900)
	result := NewDefaultCompressor(2).CompressDetailed("tester", input, "t7-hard")

	if !strings.Contains(result.Output, "## Phase Summary: tester") {
		t.Fatalf("hard summary missing:\n%s", result.Output)
	}
	if !containsString(result.Event.ReasonCodes, ReasonThresholdExceeded) || containsString(result.Event.ReasonCodes, ReasonSoftPrune) {
		t.Fatalf("hard reasons incorrect: %#v", result.Event.ReasonCodes)
	}
}

func successfulPairTrace(count int) string {
	parts := make([]string, 0, count*2)
	for i := 1; i <= count; i++ {
		parts = append(parts,
			fmt.Sprintf(`<tool_call>{"pair_id":"pair-%d","ordinal":%d,"command":"read pkg/example/file-%d.go"}</tool_call>`, i, i, i),
			fmt.Sprintf(`<tool_result>{"pair_id":"pair-%d","ordinal":%d,"status":"success","body":"completed evidence %d"}</tool_result>`, i, i, i),
		)
	}
	return strings.Join(parts, "\n")
}
