package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-ADK-GENERATED-HYGIENE-001 R13: review evidence must be classified apart
// from canonical SPEC documents before any untrack action.
func TestClassifyTrackedIgnoredSeparatesSpecEvidenceFromDocuments(t *testing.T) {
	t.Parallel()

	families := classifyTrackedIgnored([]string{
		".autopus/specs/SPEC-X/review.md",
		".autopus/specs/SPEC-X/review-findings.json",
		".autopus/specs/SPEC-X/.self-verify.log",
		".autopus/specs/SPEC-X/review-session.json",
		".autopus/specs/SPEC-X/.review-session.lock",
		".autopus/specs/SPEC-X/spec.md",
		"backend/.autopus/specs/SPEC-Y/review.md",
	})

	byLabel := map[string][]string{}
	for _, family := range families {
		byLabel[family.Label] = family.Paths
	}

	assert.ElementsMatch(t, []string{
		".autopus/specs/SPEC-X/review.md",
		".autopus/specs/SPEC-X/review-findings.json",
		".autopus/specs/SPEC-X/.self-verify.log",
		".autopus/specs/SPEC-X/review-session.json",
		".autopus/specs/SPEC-X/.review-session.lock",
		"backend/.autopus/specs/SPEC-Y/review.md",
	}, byLabel["SPEC review evidence"])
	assert.Equal(t, []string{".autopus/specs/SPEC-X/spec.md"}, byLabel["canonical SPEC document"])
}

// A canonical document in the inventory means the rule is too broad. Proposing
// `git rm --cached` over the whole set would drop it from the next commit.
func TestTrackedIgnoredRemediationPrefersRuleFixOverUntrack(t *testing.T) {
	t.Parallel()

	evidenceOnly := classifyTrackedIgnored([]string{".autopus/specs/SPEC-X/review.md"})
	assert.False(t, trackedIgnoredNeedsRuleFix(evidenceOnly))
	assert.Contains(t, trackedIgnoredRemediation(evidenceOnly), "git rm --cached")

	withDocument := classifyTrackedIgnored([]string{
		".autopus/specs/SPEC-X/review.md",
		".autopus/specs/SPEC-X/plan.md",
	})
	require.True(t, trackedIgnoredNeedsRuleFix(withDocument))
	advice := trackedIgnoredRemediation(withDocument)
	assert.Contains(t, advice, "narrow the .gitignore rule")
	assert.NotContains(t, advice, "git rm --cached")
}

// An unknown basename inside a SPEC directory is reported as canonical rather
// than proposed for untracking.
func TestClassifyTrackedIgnoredIsFailClosedInsideSpecDirectories(t *testing.T) {
	t.Parallel()

	families := classifyTrackedIgnored([]string{".autopus/specs/SPEC-X/decision-log.md"})

	require.Len(t, families, 1)
	assert.Equal(t, "canonical SPEC document", families[0].Label)
	assert.Equal(t, trackedIgnoredNarrow, families[0].Disposition)
}

func TestClassifyTrackedIgnoredGroupsLocalOnlyAndGeneratedSurfaces(t *testing.T) {
	t.Parallel()

	families := classifyTrackedIgnored([]string{
		".autopus/brainstorms/BS-001.md",
		".autopus/orchestra/review-debate.md",
		".autopus/audit.jsonl",
		".mcp.json",
		"docs/hand-written.md",
	})

	byLabel := map[string][]string{}
	for _, family := range families {
		byLabel[family.Label] = family.Paths
	}

	assert.ElementsMatch(t,
		[]string{".autopus/brainstorms/BS-001.md", ".autopus/orchestra/review-debate.md"},
		byLabel["local-only brainstorm/orchestra/runtime output"])
	assert.ElementsMatch(t,
		[]string{".autopus/audit.jsonl", ".mcp.json"},
		byLabel["generated harness surface"])
	assert.Equal(t, []string{"docs/hand-written.md"}, byLabel["unclassified"])
}

// Every path lands in exactly one family, so the printed per-family counts sum
// to the reported total.
func TestClassifyTrackedIgnoredPartitionsEveryPathExactlyOnce(t *testing.T) {
	t.Parallel()

	paths := []string{
		".autopus/specs/SPEC-X/review.md",
		".autopus/specs/SPEC-X/spec.md",
		".autopus/brainstorms/BS-001.md",
		".autopus/audit.jsonl",
		"docs/hand-written.md",
	}

	total := 0
	seen := map[string]bool{}
	for _, family := range classifyTrackedIgnored(paths) {
		total += len(family.Paths)
		for _, rel := range family.Paths {
			assert.False(t, seen[rel], "path %s appeared in two families", rel)
			seen[rel] = true
		}
	}
	assert.Equal(t, len(paths), total)
}

func TestRenderTrackedIgnoredShowsFamiliesAndRemediation(t *testing.T) {
	t.Parallel()

	paths := []string{
		".autopus/audit.jsonl",
		".autopus/brainstorms/BS-001.md",
		".autopus/brainstorms/BS-002.md",
		".autopus/brainstorms/BS-012.md",
		".autopus/brainstorms/BS-013.md",
		".autopus/specs/SPEC-X/review.md",
	}

	var buf bytes.Buffer
	renderTrackedIgnored(&buf, paths)
	text := buf.String()

	assert.Contains(t, text, "6 candidate(s) in 3 family(s)")
	assert.Contains(t, text, "SPEC review evidence: 1 file(s)")
	assert.Contains(t, text, "local-only brainstorm/orchestra/runtime output: 4 file(s)")
	assert.Contains(t, text, "generated harness surface: 1 file(s)")
	assert.Contains(t, text, "git rm --cached")
	// The old renderer truncated a sorted path list; it never named a family.
	assert.NotContains(t, text, "... and 1 more")
}

func TestRenderTrackedIgnoredReportsNoneObserved(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderTrackedIgnored(&buf, nil)

	assert.Contains(t, buf.String(), "tracked-but-ignored files: none observed")
}

func TestTrackedIgnoredCheckDetailNamesFamiliesNotRawPathPrefix(t *testing.T) {
	t.Parallel()

	report := statusHygieneReport{
		Available: true,
		Status:    "warn",
		TrackedButIgnored: []string{
			".autopus/specs/SPEC-X/review.md",
			".autopus/brainstorms/BS-001.md",
		},
	}

	check := trackedIgnoredCheck("doctor", report)
	assert.Equal(t, "doctor.hygiene.tracked_but_ignored", check.ID)
	assert.Equal(t, "warn", check.Status)
	assert.Contains(t, check.Detail, "2 candidate(s) in 2 family(s)")
	assert.Contains(t, check.Detail, "SPEC review evidence")
	assert.Contains(t, check.Detail, "untrack after review")

	payload := report.payload()
	require.Len(t, payload.TrackedIgnoredFamilies, 2)
	assert.Equal(t, "SPEC review evidence", payload.TrackedIgnoredFamilies[0].Label)
	assert.Equal(t, 1, payload.TrackedIgnoredFamilies[0].Count)
}
