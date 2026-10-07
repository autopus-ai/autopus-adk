package brainstorm_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
)

// moveSection moves the H2 block named title so it starts right before the
// H2 line named before.
func moveSection(t *testing.T, doc, title, before string) string {
	t.Helper()
	start := strings.Index(doc, "\n## "+title+"\n")
	require.GreaterOrEqual(t, start, 0)
	end := start + 1 + strings.Index(doc[start+1:], "\n## ")
	require.Greater(t, end, start)
	block := doc[start:end]
	rest := doc[:start] + doc[end:]
	at := strings.Index(rest, "\n## "+before+"\n")
	require.GreaterOrEqual(t, at, 0)
	return rest[:at] + block + rest[at:]
}

func validate(doc string) []string { return brainstorm.Validate([]byte(doc)) }

// S10: the validator follows the idea.md rules and names exactly the rule a
// change breaks; code evidence may carry confidence 7, an inferred row not.
func TestValidate_S10NamesExactlyTheBrokenRule(t *testing.T) {
	t.Parallel()
	doc := render(t, "BS-BAND-013", o2Request())
	require.Empty(t, validate(doc))

	moved := moveSection(t, doc, "Outcome Lock", "Question Audit")
	require.Equal(t, []string{"원본 아이디어", "Clarification Ledger", "Outcome Lock", "Question Audit"}, headings(moved)[:4])
	assert.Equal(t, []string{"section_order"}, validate(moved))

	inferred := strings.Replace(doc, "| scope_boundary | assumed | inferred | 5 |", "| scope_boundary | assumed | inferred | 7 |", 1)
	require.NotEqual(t, doc, inferred)
	assert.Equal(t, []string{"ledger_confidence"}, validate(inferred))

	code := strings.Replace(doc, "| goal | assumed | code | 6 |", "| goal | assumed | code | 7 |", 1)
	require.NotEqual(t, doc, code)
	assert.Empty(t, validate(code))
}

func TestValidate_EachRuleHasItsOwnCode(t *testing.T) {
	t.Parallel()
	doc := render(t, "BS-BAND-013", o2Request())
	for _, tc := range []struct{ name, old, new, want string }{
		{"title without the ID colon", "# BS-BAND-013: ", "# BS-BAND-013 ", "title"},
		{"missing strategy line", "**Strategy**: band-diagnosis\n", "", "header"},
		{"empty status value", "**Status**: active", "**Status**:", "header"},
		{"missing section", "## Evolution Ideas\n", "", "section_order"},
		{"renamed column", "| If Wrong |", "| Risk |", "ledger_columns"},
		{"renamed row", "| constraints | assumed", "| constraint | assumed", "ledger_rows"},
		{"missing row", "| brownfield_impact | deferred | none | 2 |", "", "ledger_rows"},
		{"unknown status", "| goal | assumed |", "| goal | guessed |", "ledger_value"},
		{"unknown source", "| done_evidence | assumed | code |", "| done_evidence | assumed | gut |", "ledger_value"},
		{"confidence out of range", "| goal | assumed | code | 6 |", "| goal | assumed | code | 11 |", "ledger_confidence"},
		{"confidence not a number", "| goal | assumed | code | 6 |", "| goal | assumed | code | six |", "ledger_confidence"},
		{"high confidence without evidence", "| brownfield_impact | deferred | none | 2 |", "| brownfield_impact | deferred | none | 8 |", "ledger_confidence"},
		{"inferred row without If Wrong", "| The cause spans other series or modules, so the scope must widen. |", "|  |", "ledger_if_wrong"},
		{"unknown question transport", "- question_transport: none", "- question_transport: telepathy", "question_audit"},
		{"too many questions", "- question_count: 0", "- question_count: 4", "question_audit"},
		{"missing unresolved fields", "- unresolved_fields: [", "- unresolved: [", "question_audit"},
		{"missing completion evidence", "- Completion evidence: ", "- Proof: ", "outcome_lock"},
		{"next step names another BS", "--from-idea BS-BAND-013 \"", "--from-idea BS-BAND-014 \"", "next_step"},
		{"next step without description", " \"ci.failure_rate:CI tier 2 anomaly response\"`", "`", "next_step"},
	} {
		mutated := strings.Replace(doc, tc.old, tc.new, 1)
		require.NotEqual(t, doc, mutated, tc.name)
		assert.Equal(t, []string{tc.want}, validate(mutated), tc.name)
	}
}

// Headings, tables, and next-step lines inside a fence are content, and a
// fence closes only on a run at least as long as its opener.
func TestValidate_IgnoresEverythingInsideFences(t *testing.T) {
	t.Parallel()
	doc := render(t, "BS-BAND-013", o2Request())
	fake := "~~~text\n## 다음 단계\n`/auto plan --from-idea BS-BAND-999 \"x\"`\n~~~\n" +
		"`````\n```\n## Outcome Lock\n```\n`````\n"
	injected := strings.Replace(doc, "diagnosis_status: unavailable(provider_missing)\n",
		"diagnosis_status: unavailable(provider_missing)\n"+fake, 1)
	assert.Empty(t, validate(injected))

	unclosed := strings.Replace(doc, "## 추천 방향\n", "## 추천 방향\n````text\n", 1)
	assert.Contains(t, validate(unclosed), "section_order", "an unclosed fence swallows every later section")
}

// Escaped pipes stay inside their cell, a table without its separator row is
// malformed, and a fence closer may be indented three spaces but not four.
func TestValidate_FollowsMarkdownTableAndFenceRules(t *testing.T) {
	t.Parallel()
	doc := render(t, "BS-BAND-013", o2Request())
	status := "diagnosis_status: unavailable(provider_missing)\n"

	assert.Empty(t, validate(strings.Replace(doc, "Bring `ci.failure_rate:CI` back", `Bring a\|b back`, 1)))
	assert.Equal(t, []string{"ledger_columns", "ledger_rows"}, validate(strings.Replace(doc, "|---|---|---|---:|---|---|---|\n", "", 1)))
	assert.Empty(t, validate(strings.Replace(doc, status, status+"````\n## Fake\n   ````\n", 1)))
	assert.Empty(t, validate(strings.Replace(doc, status, status+"````\n    ````\n## Fake\n````\n", 1)))
	assert.Empty(t, validate(strings.Replace(doc, status, status+"````\n## Fake\n    ````\n````\n", 1)),
		"a four-space line is content, so the next run closes the fence instead of opening one")
}

// Agents often bold bullet labels; a bold label reads like a plain one.
func TestValidate_AcceptsBoldBulletLabels(t *testing.T) {
	t.Parallel()
	doc := render(t, "BS-BAND-013", o2Request())
	bold := strings.Replace(doc, "- Completion evidence:", "- **Completion evidence**:", 1)
	bold = strings.Replace(bold, "- question_count:", "- **question_count**:", 1)
	require.NotEqual(t, doc, bold)

	assert.Empty(t, validate(bold))
}

// Problems come in rule order, each once; CRLF files validate like LF.
func TestValidate_ReportsProblemsInRuleOrderOnce(t *testing.T) {
	t.Parallel()
	doc := render(t, "BS-BAND-013", o2Request())
	broken := strings.Replace(doc, "**Providers**: claude\n", "", 1)
	broken = strings.Replace(broken, "| goal | assumed | code | 6 |", "| goal | assumed | inferred | 9 |", 1)
	broken = strings.Replace(broken, "| constraints | assumed | inferred | 5 |", "| constraints | assumed | inferred | 8 |", 1)
	broken = strings.Replace(broken, "--from-idea BS-BAND-013 \"", "--from-idea BS-BAND-012 \"", 1)

	assert.Equal(t, []string{"header", "ledger_confidence", "next_step"}, validate(broken))
	assert.Empty(t, validate(strings.ReplaceAll(doc, "\n", "\r\n")))
	assert.Contains(t, validate(""), "title")
}
