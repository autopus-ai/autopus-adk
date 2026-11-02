package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildReviewPromptIncludesAuxiliaryDocs verifies that plan.md, research.md,
// and acceptance.md in opts.SpecDir are injected into the prompt. Under the
// generous default total budget (SPEC-ADK-REVIEW-INTEGRITY-001 REQ-RINT-FULL-02)
// a typical oversize document injects in full with no truncation notice —
// replacing the prior head-only 200-line trim.
func TestBuildReviewPromptIncludesAuxiliaryDocs(t *testing.T) {
	t.Parallel()

	specDir := t.TempDir()

	// plan.md: 50 lines
	planLines := make([]string, 50)
	for i := range planLines {
		planLines[i] = "plan line"
	}
	require.NoError(t, os.WriteFile(filepath.Join(specDir, "plan.md"), []byte(strings.Join(planLines, "\n")), 0o644))

	// research.md: 250 lines — exceeds the legacy 200-line cap but fits the
	// generous default budget, so it must now inject in full.
	researchLines := make([]string, 250)
	for i := range researchLines {
		researchLines[i] = "research line"
	}
	require.NoError(t, os.WriteFile(filepath.Join(specDir, "research.md"), []byte(strings.Join(researchLines, "\n")), 0o644))

	// acceptance.md: 80 lines
	acceptanceLines := make([]string, 80)
	for i := range acceptanceLines {
		acceptanceLines[i] = "acceptance line"
	}
	require.NoError(t, os.WriteFile(filepath.Join(specDir, "acceptance.md"), []byte(strings.Join(acceptanceLines, "\n")), 0o644))

	doc := &SpecDocument{ID: "SPEC-TEST-001", Title: "Test"}
	opts := ReviewPromptOptions{SpecDir: specDir}
	prompt := BuildReviewPrompt(doc, "", opts)

	assert.Contains(t, prompt, "### Plan Document", "plan.md section must be present")
	assert.Contains(t, prompt, "### Research Document", "research.md section must be present")
	assert.Contains(t, prompt, "### Acceptance Criteria Document", "acceptance.md section must be present")
	assert.NotContains(t, prompt, "additional lines were omitted",
		"a 250-line research.md fits the generous default budget and must inject in full")
}

// TestBuildReviewPromptOmitsMissingDocs verifies that absent auxiliary files are
// silently omitted — no section header, no error.
func TestBuildReviewPromptOmitsMissingDocs(t *testing.T) {
	t.Parallel()

	specDir := t.TempDir()
	// No plan.md, research.md, or acceptance.md present

	doc := &SpecDocument{ID: "SPEC-EMPTY-001", Title: "Empty"}
	opts := ReviewPromptOptions{SpecDir: specDir}
	prompt := BuildReviewPrompt(doc, "", opts)

	assert.NotContains(t, prompt, "### Plan Document")
	assert.NotContains(t, prompt, "### Research Document")
	assert.NotContains(t, prompt, "### Acceptance Criteria Document")
}

// TestBuildReviewPromptIncludesVerdictRules verifies that the default discover-mode
// prompt includes the Verdict Decision Rules section with required keywords.
func TestBuildReviewPromptIncludesVerdictRules(t *testing.T) {
	t.Parallel()

	doc := &SpecDocument{ID: "SPEC-VR-001", Title: "Verdict Rules Test"}
	prompt := BuildReviewPrompt(doc, "", ReviewPromptOptions{})

	assert.Contains(t, prompt, "### Verdict Decision Rules")
	// SPEC-REVIEWPOLICY-001: the rules are the canonical lines the judge and
	// the runtime share; the old "major <= 2 is PASS" rule is gone.
	for _, line := range ReviewVerdictRuleLines() {
		assert.Contains(t, prompt, line)
	}
	assert.NotContains(t, prompt, "major <= 2")
	assert.Contains(t, prompt, "PASS")
	assert.Contains(t, prompt, "REJECT")
	assert.Contains(t, prompt, "REVISE")
}

// TestBuildReviewPromptRespectsPassCriteriaOverride verifies that a custom
// PassCriteria value in ReviewPromptOptions overrides the default verdict rules.
func TestBuildReviewPromptRespectsPassCriteriaOverride(t *testing.T) {
	t.Parallel()

	doc := &SpecDocument{ID: "SPEC-PC-001", Title: "Pass Criteria Override"}
	opts := ReviewPromptOptions{
		PassCriteria: "CUSTOM_OVERRIDE",
	}
	prompt := BuildReviewPrompt(doc, "", opts)

	assert.Contains(t, prompt, "CUSTOM_OVERRIDE")
}

// TestBuildReviewPromptIncludesFewShotExamples verifies that the prompt contains
// a Finding Format Examples section with structured positive examples and one AVOID example.
func TestBuildReviewPromptIncludesFewShotExamples(t *testing.T) {
	t.Parallel()

	doc := &SpecDocument{ID: "SPEC-FS-001", Title: "Few Shot Examples"}
	prompt := BuildReviewPrompt(doc, "", ReviewPromptOptions{})

	assert.Contains(t, prompt, "### Finding Format Examples")

	// Positive structured examples use FINDING: [severity] [category] scope description
	assert.Contains(t, prompt, "FINDING: [major] [correctness]", "must include structured positive example")
	assert.Contains(t, prompt, "FINDING: [critical] [security]", "must include second structured positive example")

	// AVOID label for legacy format
	assert.Contains(t, prompt, "do NOT use")
}

func TestBuildReviewPromptWarnsAgainstTrimNoticeFindings(t *testing.T) {
	t.Parallel()

	doc := &SpecDocument{ID: "SPEC-TRIM-001", Title: "Trim Notice"}
	prompt := BuildReviewPrompt(doc, "", ReviewPromptOptions{})

	assert.Contains(t, prompt, "review-context notice")
	assert.Contains(t, prompt, "not as a source-document defect or finding evidence")
}
