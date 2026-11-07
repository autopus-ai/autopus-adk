package cli

import (
	"maps"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	driverTierChoices = "  1) fable 2) opus 3) sonnet 4) haiku [Enter] 그대로\n"
	driverEditMenu    = "계속 편집: g) 역할 그룹 · a) 고급 · [Enter] 미리보기로 돌아가기\n"
	driverNoChange    = "변경 없음\n"
)

// driverS6Edit is the start after `g`, `4`, Enter: Implementation becomes
// haiku and Verification·Review keeps its mixed tiers.
func driverS6Edit() map[string]string {
	edited := driverStart()
	edited["frontend-specialist"] = "haiku"
	return edited
}

// TestComposeDriverSession_GroupsWithoutEditWriteNothing pins S2: Enter on
// every group prints 변경 없음 and ends without the confirmation question.
func TestComposeDriverSession_GroupsWithoutEditWriteNothing(t *testing.T) {
	session, table, planner := newDriverSession(driverConfig("claude-code"))

	result, out, err := runDriver(t, session, "g\n\n\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.Equal(t, []map[string]string{driverStart()}, planner.edits)
	assert.Contains(t, out, "역할 그룹마다 등급을 고르세요 (Enter = 그대로)\n"+
		"구현  frontend-specialist  현재: sonnet\n"+driverTierChoices)
	assert.Contains(t, out, "검증·리뷰  reviewer, qa-explorer, ux-validator  현재: fable+sonnet\n")
	assert.Equal(t, 2, strings.Count(out, driverTierChoices))
	assert.True(t, strings.HasSuffix(out, driverNoChange), out)
	assert.NotContains(t, out, driverConfirm)
	assert.Empty(t, table.previews)
}

// TestComposeDriverSession_GroupEditAppliesOnYes is the S6 and S10 input.
func TestComposeDriverSession_GroupEditAppliesOnYes(t *testing.T) {
	session, table, planner := newDriverSession(driverConfig("claude-code"))

	result, out, err := runDriver(t, session, "g\n4\n\ny\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverApplied, result.Outcome)
	assert.Equal(t, []map[string]string{driverS6Edit()}, planner.edits)
	assert.Equal(t, []map[string]string{driverS6Edit()}, planner.applied)
	assert.Len(t, table.previews, 1)
	assert.Contains(t, out, "바뀌는 설정:\n  quality.default balanced → custom\n"+driverConfirm+"> "+driverApplied)
}

// TestComposeDriverSession_GroupAnswersTakeNumbersOrTiers pins the Role Group
// Contract: a number or tier name selects, and the third invalid answer keeps
// that group and moves on.
func TestComposeDriverSession_GroupAnswersTakeNumbersOrTiers(t *testing.T) {
	session, _, planner := newDriverSession(driverConfig("claude-code"))

	_, out, err := runDriver(t, session, "g\nbogus\n9\nfable!\n OPUS \ny\n")

	require.NoError(t, err)
	want := driverStart()
	for _, agent := range []string{"reviewer", "qa-explorer", "ux-validator"} {
		want[agent] = "opus"
	}
	assert.Equal(t, []map[string]string{want}, planner.applied)
	assert.Equal(t, 4, strings.Count(out, driverTierChoices))
	assert.NotContains(t, out, driverInvalidKeep, "a kept group does not end the session")
}

// TestComposeDriverSession_InputEndInsideGroupsKeeps pins EOF anywhere: no
// plan, no preview.
func TestComposeDriverSession_InputEndInsideGroupsKeeps(t *testing.T) {
	session, table, planner := newDriverSession(driverConfig("claude-code"))

	result, out, err := runDriver(t, session, "g\n3\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.Empty(t, planner.edits)
	assert.Empty(t, table.previews)
	assert.True(t, strings.HasSuffix(out, driverCancelled), out)
}

// TestComposeDriverSession_AdvancedKeepsEditsAcrossRejectedLine pins S3 and
// REQ-MUX-04: the parser's error is printed, earlier edits stay, and each
// accepted edit shows its group recomputed.
func TestComposeDriverSession_AdvancedKeepsEditsAcrossRejectedLine(t *testing.T) {
	session, _, planner := newDriverSession(driverConfig("claude-code"))

	_, out, err := runDriver(t, session, "a\nqa-explorer=opus\nreviewer=gpt\nfrontend-specialist=haiku\n\ny\n")

	require.NoError(t, err)
	assert.Contains(t, out, "고급: 에이전트별 등급 (agent=tier, 빈 줄로 끝)\n")
	assert.Contains(t, out, "  qa-explorer = opus   검증·리뷰: fable+opus+sonnet\n")
	assert.Contains(t, out, "  unknown tier \"gpt\" (choose one of: fable, opus, sonnet, haiku)\n")
	assert.Contains(t, out, "  frontend-specialist = haiku   구현: haiku\n")
	want := driverStart()
	want["qa-explorer"], want["frontend-specialist"] = "opus", "haiku"
	assert.Equal(t, []map[string]string{want}, planner.applied)
}

// TestComposeDriverSession_AdvancedFinishesAtInputEnd: input end finishes the
// editor like an empty line, and the confirmation reads the same end and
// cancels, so nothing is written.
func TestComposeDriverSession_AdvancedFinishesAtInputEnd(t *testing.T) {
	session, table, planner := newDriverSession(driverConfig("claude-code"))

	result, out, err := runDriver(t, session, "a\nqa-explorer=opus")

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	want := driverStart()
	want["qa-explorer"] = "opus"
	assert.Equal(t, []map[string]string{want}, planner.edits)
	assert.Len(t, table.previews, 1)
	assert.Empty(t, planner.applied)
	assert.True(t, strings.HasSuffix(out, driverCancelled), out)
}

// TestComposeDriverSession_EditKeepsTheCandidate pins REQ-MUX-06: `e` returns
// to the edit menu and the next editor starts from the candidate.
func TestComposeDriverSession_EditKeepsTheCandidate(t *testing.T) {
	session, table, planner := newDriverSession(driverConfig("claude-code"))

	result, out, err := runDriver(t, session, "g\n4\n\ne\na\nqa-explorer=opus\n\ny\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverApplied, result.Outcome)
	assert.Contains(t, out, driverConfirm+"> "+driverEditMenu)
	second := driverS6Edit()
	second["qa-explorer"] = "opus"
	assert.Equal(t, []map[string]string{driverS6Edit(), second}, planner.edits)
	assert.Equal(t, []map[string]string{second}, planner.applied)
	assert.Len(t, table.previews, 2)
}

// TestComposeDriverSession_EditMenuEnterReturnsToThePreview keeps the pending
// plan when the operator leaves the edit menu without editing.
func TestComposeDriverSession_EditMenuEnterReturnsToThePreview(t *testing.T) {
	session, table, planner := newDriverSession(driverConfig("claude-code"))

	_, _, err := runDriver(t, session, "g\n\n3\ne\n\ny\n")

	require.NoError(t, err)
	assert.Len(t, planner.edits, 1)
	assert.Len(t, table.previews, 2)
	assert.Equal(t, planner.edits, planner.applied)
}

// TestComposeDriverSession_RefreshThenEditStartsFromTheCandidate: after `u`,
// `e` edits the refreshed composition rather than the starting one.
func TestComposeDriverSession_RefreshThenEditStartsFromTheCandidate(t *testing.T) {
	session, _, planner := newDriverSession(driverHistoricalConfig("claude-code"))

	_, _, err := runDriver(t, session, "u\ne\ng\n\n\ny\n")

	require.NoError(t, err)
	assert.Equal(t, []map[string]string{planner.refreshed}, planner.edits)
	assert.Equal(t, []map[string]string{planner.refreshed}, planner.applied)
}

// TestComposeDriverSession_EditMenuKeepPaths: three invalid answers, input
// end, or an edit back to the start all write nothing.
func TestComposeDriverSession_EditMenuKeepPaths(t *testing.T) {
	cases := map[string]struct {
		input string
		last  string
	}{
		"three invalid answers": {"g\n\n3\ne\nx\nu\nt\n", driverInvalidKeep},
		"input end":             {"g\n\n3\ne\n", driverCancelled},
		"edit back to start":    {"a\nqa-explorer=opus\n\ne\na\nqa-explorer=fable\n\n", driverNoChange},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			session, _, planner := newDriverSession(driverConfig("claude-code"))

			result, out, err := runDriver(t, session, tc.input)

			require.NoError(t, err)
			assert.Equal(t, composeDriverKept, result.Outcome)
			assert.Empty(t, planner.applied)
			assert.True(t, strings.HasSuffix(out, tc.last), out)
		})
	}
}

// TestComposeDriverSession_EditorNeverMutatesThePlannerStart guards the keep
// path: an edit works on a copy, so a cancelled session leaves nothing behind.
func TestComposeDriverSession_EditorNeverMutatesThePlannerStart(t *testing.T) {
	session, _, planner := newDriverSession(driverConfig("claude-code"))
	start := maps.Clone(planner.start)

	_, _, err := runDriver(t, session, "g\n1\n1\nn\n")

	require.NoError(t, err)
	assert.Equal(t, start, planner.start)
}

// composeDriverTierAnswer reads one group answer the driver has already
// trimmed: a menu number in config.QualityTiers() order or a tier name.
func TestComposeDriverTierAnswer_TakesAMenuNumberOrATierName(t *testing.T) {
	t.Parallel()
	tiers := config.QualityTiers()
	for answer, want := range map[string]string{
		"1": "fable", "2": "opus", "3": "sonnet", "4": "haiku", "Opus": "opus", "haiku": "haiku",
	} {
		tier, ok := composeDriverTierAnswer(answer, tiers)
		assert.True(t, ok, answer)
		assert.Equal(t, want, tier, answer)
	}
	for _, answer := range []string{"0", "5", "gpt", "1)"} {
		_, ok := composeDriverTierAnswer(answer, tiers)
		assert.False(t, ok, answer)
	}
}

// The advanced step and the per-tool editor share one `agent=tier` parser, so
// both refuse the same malformed lines.
func TestParseQualityToolTierAssignment_RejectsMalformedLines(t *testing.T) {
	t.Parallel()
	// security-auditor is a legacy role: no agent file is generated for it,
	// so a tier for it is refused like an unknown name.
	for _, line := range []string{"bogus=fable", "reviewer", "reviewer=", "=opus", "reviewer=gpt", "security-auditor=opus"} {
		_, _, err := parseQualityToolTierAssignment(line)
		assert.Error(t, err, line)
	}
	agent, tier, err := parseQualityToolTierAssignment("UX_Validator=Opus")
	require.NoError(t, err)
	assert.Equal(t, [2]string{"ux-validator", "opus"}, [2]string{agent, tier})
}
