package compress

import (
	"fmt"
	"strings"
	"testing"
)

func TestContextCompress_SevenSectionSchema_RequiresContentOrFallback(t *testing.T) {
	input := `## Goal
Ship SPEC-CONTEXT-COMPRESS-001 compaction.

## Constraints
- Preserve user changes outside pkg/worker/compress.

## Progress
- Added RED tests for acceptance criteria.

## Decision
- D1: Use pair-aware pruning before summarization.

## Relevant Files
- pkg/worker/compress/compressor.go

## Next Steps
- Implement schema extraction.

## Critical Context
- Do not silently drop constraints or decisions.`

	summary := Summarize("executor", input, 5000)

	want := map[string]string{
		"Goal":             "Ship SPEC-CONTEXT-COMPRESS-001 compaction.",
		"Constraints":      "Preserve user changes outside pkg/worker/compress.",
		"Progress":         "Added RED tests for acceptance criteria.",
		"Decisions":        "D1: Use pair-aware pruning before summarization.",
		"Relevant Files":   "pkg/worker/compress/compressor.go",
		"Next Steps":       "Implement schema extraction.",
		"Critical Context": "Do not silently drop constraints or decisions.",
	}
	for section, wantText := range want {
		assertSectionContains(t, summary, section, wantText)
	}
}

func TestContextCompress_ToolPairPruning_KeepsRecentPairsAndPrunesOlderPairs(t *testing.T) {
	trace := strings.Join([]string{
		`<tool_call>{"pair_id":"pair-1","ordinal":1,"command":"old call one"}</tool_call>`,
		`<tool_result>{"pair_id":"pair-1","ordinal":1,"body":"old result one"}</tool_result>`,
		`<tool_call>{"pair_id":"pair-2","ordinal":2,"command":"old call two"}</tool_call>`,
		`<tool_result>{"pair_id":"pair-2","ordinal":2,"body":"old result two"}</tool_result>`,
		`<tool_call>{"pair_id":"pair-3","ordinal":3,"command":"recent call"}</tool_call>`,
		`<tool_result>{"pair_id":"pair-3","ordinal":3,"body":"recent result"}</tool_result>`,
	}, "\n")

	pruned := PruneToolResults(trace, 1)

	assertContains(t, pruned, `"pair_id":"pair-3"`)
	assertContains(t, pruned, "provider_payload_omitted")
	assertNotContains(t, pruned, "recent call")
	assertNotContains(t, pruned, "recent result")
	assertNotContains(t, pruned, "old call one")
	assertNotContains(t, pruned, "old result one")
	assertNotContains(t, pruned, "old call two")
	assertNotContains(t, pruned, "old result two")

	wantPlaceholders := []string{
		"[tool_pair pruned: pair=pair-1 ordinal=1]",
		"[tool_pair pruned: pair=pair-2 ordinal=2]",
	}
	for _, placeholder := range wantPlaceholders {
		assertContains(t, pruned, placeholder)
	}

	results, calls := CountToolBlocks(pruned)
	if results != calls {
		t.Fatalf("pruned output has orphaned tool blocks: calls=%d results=%d\n%s", calls, results, pruned)
	}
	if results != 1 || calls != 1 {
		t.Fatalf("pruned output should keep exactly one complete pair, got calls=%d results=%d\n%s", calls, results, pruned)
	}
}

func TestContextCompress_RepeatedCompaction_RetainsContinuityTrail(t *testing.T) {
	previousSummary := `summary_id: summary-001

### Decisions
- D1: Keep pair placeholders instead of raw older tool output.

### Relevant Files
- pkg/a.go`

	laterTrace := previousSummary + `

## Decision
- D2: Add a context-budget blocker for impossible summaries.

## Relevant Files
- pkg/b.go`

	summary := Summarize("executor", laterTrace, 5000)

	assertContains(t, summary, "previous_summary_id: summary-001")
	assertSectionContains(t, summary, "Decisions", "D1: Keep pair placeholders instead of raw older tool output.")
	assertSectionContains(t, summary, "Decisions", "D2: Add a context-budget blocker for impossible summaries.")
	assertSectionContains(t, summary, "Relevant Files", "pkg/a.go")
	assertSectionContains(t, summary, "Relevant Files", "pkg/b.go")
}

func TestContextCompress_RequiredBudgetFailure_ReturnsContextBudgetBlocker(t *testing.T) {
	input := `## Goal
Compress without losing required context.

## Constraints
- C1: Constraints must survive compaction.

## Decision
- D1: Decisions must survive compaction.`

	summary := Summarize("planner", input, 12)

	assertContains(t, summary, "context-budget")
	assertContains(t, summary, "C1: Constraints must survive compaction.")
	assertContains(t, summary, "D1: Decisions must survive compaction.")
	assertNotContains(t, summary, "[...truncated")
}

func TestContextCompress_RedactionMetadataAndIndexEligibility_RedactsUnsafeBodies(t *testing.T) {
	// Fake credentials are joined from fragments: GitHub push protection and
	// secret scanners match the contiguous source text, so a provider-format
	// fake written as one literal would be reported. Each joined value is
	// byte-identical to the literal it replaced.
	const fakeSecret = "sk-" + "test-1234567890abcdef"
	const fakeAWSKey = "AKIA" + "ABCDEFGHIJKLMNOP"
	const fakeGitHubToken = "ghp_" + "abcdefghijklmnopqr" + "stuvwxyzABCDEFGHIJ"
	const fakeBearer = "Bearer abc.def-ghi_jkl"
	const fakePrivateKey = "-----BEGIN " + "OPENSSH PRIVATE" + " KEY-----"
	const rawLocalPath = "/Users/alice/private/provider-payload.json"
	const rawWindowsPath = `C:\Users\alice\private\payload.json`
	const rawMacPrivatePath = "/private/var/folders/zz/provider-payload.json"
	const providerPayload = "unstructured provider response body must not survive"

	input := fmt.Sprintf(`index_export=true

## Goal
Create a redacted derived summary.

## Progress
<tool_result>{"pair_id":"pair-secret","body":"token=%s aws=%s github=%s auth=%s key=%s failed at %s %s %s and %s"}</tool_result>

## Decision
- D1: Preserve safe metadata only.`, fakeSecret, fakeAWSKey, fakeGitHubToken, fakeBearer, fakePrivateKey, rawLocalPath, rawWindowsPath, rawMacPrivatePath, providerPayload)

	summary := Summarize("security-auditor", input, 5000)

	assertNotContains(t, summary, fakeSecret)
	assertNotContains(t, summary, fakeAWSKey)
	assertNotContains(t, summary, fakeGitHubToken)
	assertNotContains(t, summary, fakeBearer)
	assertNotContains(t, summary, fakePrivateKey)
	assertNotContains(t, summary, rawLocalPath)
	assertNotContains(t, summary, rawWindowsPath)
	assertNotContains(t, summary, rawMacPrivatePath)
	assertNotContains(t, summary, providerPayload)
	assertContains(t, summary, "redaction_reason=secret")
	assertContains(t, summary, "redaction_reason=local_path")
	assertContains(t, summary, "redaction_reason=provider_payload")
	assertContains(t, summary, "body=omitted")
	assertContains(t, summary, "index_eligibility=redacted_derived_context")
}

func assertSectionContains(t *testing.T, summary, section, want string) {
	t.Helper()

	body, ok := markdownSection(summary, section)
	if !ok {
		t.Fatalf("summary missing section %q\n%s", section, summary)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatalf("summary section %q is empty\n%s", section, summary)
	}
	assertContains(t, body, want)
}

func markdownSection(summary, section string) (string, bool) {
	marker := "### " + section
	start := strings.Index(summary, marker)
	if start < 0 {
		return "", false
	}

	body := summary[start+len(marker):]
	if next := strings.Index(body, "\n### "); next >= 0 {
		body = body[:next]
	}
	return strings.TrimSpace(body), true
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()

	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q\n%s", want, got)
	}
}

func assertNotContains(t *testing.T, got, forbidden string) {
	t.Helper()

	if strings.Contains(got, forbidden) {
		t.Fatalf("expected output not to contain %q\n%s", forbidden, got)
	}
}
