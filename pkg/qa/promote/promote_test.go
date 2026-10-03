package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

const acceptanceX = `# SPEC-X — Acceptance

### S1: AC-X-001 — Sign in reaches the dashboard

GIVEN a registered user
WHEN they sign in
THEN the dashboard says "Welcome back".
`

const acceptanceScenario = `schema_version: qamesh.scenario.v2
id: sign-in-dashboard
title: Sign in reaches the dashboard
journey: browser-gui-explore
intent_source: acceptance
spec: SPEC-X
acceptance_refs: [AC-X-001]
screens:
  - id: sign-in
    path: /login
    steps:
      - click: {role: button, name: Sign in}
      - expect_text: Welcome back
        ac: AC-X-001
`

// recordedScenario carries one agent assertion nobody has confirmed yet.
const recordedScenario = `schema_version: qamesh.scenario.v2
id: recorded-checkout
title: Recorded checkout
journey: browser-gui-explore
intent_source: recording
recording_ref: .autopus/qa/recordings/checkout.jsonl
screens:
  - id: cart
    path: /cart
    steps:
      - click: {role: button, name: Checkout}
        by: agent
      # The agent saw this text after checkout.
      - expect_text: Order placed
        by: agent
        confirm: required
`

func homeScenario(text string) string {
	return "schema_version: qamesh.scenario.v1\nid: home\ntitle: Home\njourney: browser-gui-explore\n" +
		"screens:\n  - id: home\n    path: /\n    steps:\n      - expect_text: " + text + "\n"
}

func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	put(t, dir, ".autopus/specs/SPEC-X/acceptance.md", acceptanceX)
	return dir
}

func put(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func get(t *testing.T, dir, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(body)
}

func codes(skips []Skip) map[string]string {
	out := map[string]string{}
	for _, s := range skips {
		out[s.ID] = s.Code
	}
	return out
}

const (
	candDir   = ".autopus/qa/scenarios/candidates/"
	activeDir = ".autopus/qa/scenarios/"
)

// AC-QALOOP-007: only the valid acceptance scenario moves; the unconfirmed
// recording moves once a person accepts its assertions; the clash never does.
func TestPromote_All_AppliesEveryGate(t *testing.T) {
	t.Parallel()
	dir := project(t)
	put(t, dir, candDir+"sign-in-dashboard.yaml", acceptanceScenario)
	put(t, dir, candDir+"recorded-checkout.yaml", recordedScenario)
	put(t, dir, candDir+"home.yaml", homeScenario("Hi"))
	put(t, dir, activeDir+"home.yaml", homeScenario("Hello"))

	report, err := Promote(dir, Options{All: true})
	require.NoError(t, err)
	require.Len(t, report.Promoted, 1)
	assert.Equal(t, Item{ID: "sign-in-dashboard", Kind: KindScenario,
		From: candDir + "sign-in-dashboard.yaml", To: activeDir + "sign-in-dashboard.yaml"}, report.Promoted[0])
	assert.Equal(t, acceptanceScenario, get(t, dir, activeDir+"sign-in-dashboard.yaml"))
	assert.NoFileExists(t, filepath.Join(dir, candDir, "sign-in-dashboard.yaml"))
	assert.Equal(t, map[string]string{
		"recorded-checkout": CodeUnconfirmedAgentAssertion,
		"home":              CodeConflict,
	}, codes(report.Skipped))
	assert.NoFileExists(t, filepath.Join(dir, activeDir, "recorded-checkout.yaml"))

	report, err = Promote(dir, Options{All: true, AcceptAgentAssertions: true})
	require.NoError(t, err)
	require.Len(t, report.Promoted, 1)
	assert.Equal(t, "recorded-checkout", report.Promoted[0].ID)
	assert.Equal(t, 1, report.Promoted[0].AcceptedAssertions)
	promoted := get(t, dir, activeDir+"recorded-checkout.yaml")
	assert.NotContains(t, promoted, "confirm")
	assert.Equal(t, strings.Replace(recordedScenario, "        confirm: required\n", "", 1), promoted,
		"only the confirm line goes: key order, flow style, and comments stay")
	_, err = scenario.ParseBytes("promoted", []byte(promoted))
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, candDir, "recorded-checkout.yaml"))

	assert.Equal(t, map[string]string{"home": CodeConflict}, codes(report.Skipped))
	assert.Equal(t, homeScenario("Hello"), get(t, dir, activeDir+"home.yaml"))
	assert.Equal(t, homeScenario("Hi"), get(t, dir, candDir+"home.yaml"))
}

func TestPromote_AcceptanceRefs_MustStillResolve(t *testing.T) {
	t.Parallel()
	dir := project(t)
	put(t, dir, candDir+"retired.yaml", strings.NewReplacer("id: sign-in-dashboard", "id: retired", "AC-X-001", "AC-X-002").Replace(acceptanceScenario))
	put(t, dir, candDir+"no-spec-file.yaml", strings.NewReplacer("id: sign-in-dashboard", "id: no-spec-file", "SPEC-X", "SPEC-GONE").Replace(acceptanceScenario))
	put(t, dir, ".autopus/qa/test-scenarios/candidates/SPEC-X.yaml", "schema_version: qamesh.test-scenarios.v1\nspec: SPEC-X\ncases:\n"+
		"  - {id: stale, ac: AC-X-009, kind: happy, title: t, given: g, when: w, then: th, automation: {type: manual, reason: r}}\n")

	report, err := Promote(dir, Options{All: true})
	require.NoError(t, err)
	assert.Empty(t, report.Promoted)
	assert.Equal(t, map[string]string{
		"retired":      CodeAcceptanceRefMissing,
		"no-spec-file": CodeAcceptanceRefMissing,
		"SPEC-X":       CodeAcceptanceRefMissing,
	}, codes(report.Skipped))
}

func TestPromote_StepThatGainedAnAc_NeedsNoAcceptFlag(t *testing.T) {
	t.Parallel()
	dir := project(t)
	withAc := strings.Replace(recordedScenario, "        confirm: required\n", "        confirm: required\n        ac: AC-X-001\n", 1)
	put(t, dir, candDir+"recorded-checkout.yaml", strings.Replace(withAc, "intent_source: recording\n", "intent_source: recording\nspec: SPEC-X\n", 1))
	put(t, dir, candDir+"orphan-ac.yaml", strings.Replace(withAc, "id: recorded-checkout", "id: orphan-ac", 1))

	report, err := Promote(dir, Options{All: true})
	require.NoError(t, err)
	require.Len(t, report.Promoted, 1)
	assert.Equal(t, "recorded-checkout", report.Promoted[0].ID)
	assert.Equal(t, map[string]string{"orphan-ac": CodeAcceptanceRefMissing}, codes(report.Skipped),
		"an ac that names no spec cannot be shown to exist")
}

func TestPromote_TestScenarios_MoveToActiveDirectory(t *testing.T) {
	t.Parallel()
	dir := project(t)
	doc := "schema_version: qamesh.test-scenarios.v1\nspec: SPEC-X\ncases:\n" +
		"  - {id: happy, ac: AC-X-001, kind: happy, title: t, given: g, when: w, then: th, automation: {type: manual, reason: r}}\n"
	put(t, dir, ".autopus/qa/test-scenarios/candidates/SPEC-X.yaml", doc)

	report, err := Promote(dir, Options{IDs: []string{"SPEC-X"}})
	require.NoError(t, err)
	require.Len(t, report.Promoted, 1)
	assert.Equal(t, Item{ID: "SPEC-X", Kind: KindTestScenarios,
		From: ".autopus/qa/test-scenarios/candidates/SPEC-X.yaml", To: ".autopus/qa/test-scenarios/SPEC-X.yaml"}, report.Promoted[0])
	assert.Equal(t, doc, get(t, dir, ".autopus/qa/test-scenarios/SPEC-X.yaml"))
}

func TestPromote_IdenticalActiveFile_CountsAsPromoted(t *testing.T) {
	t.Parallel()
	dir := project(t)
	put(t, dir, candDir+"home.yaml", homeScenario("Hi"))
	put(t, dir, activeDir+"home.yaml", homeScenario("Hi"))

	report, err := Promote(dir, Options{IDs: []string{"home"}})
	require.NoError(t, err)
	require.Len(t, report.Promoted, 1)
	assert.True(t, report.Promoted[0].AlreadyActive)
	assert.NoFileExists(t, filepath.Join(dir, candDir, "home.yaml"))
}

func TestPromote_ActiveFileDeclaringSameID_IsAConflict(t *testing.T) {
	t.Parallel()
	dir := project(t)
	put(t, dir, candDir+"home-v2.yaml", homeScenario("Hi"))
	put(t, dir, activeDir+"home.yaml", homeScenario("Hello"))

	report, err := Promote(dir, Options{All: true})
	require.NoError(t, err)
	assert.Empty(t, report.Promoted)
	assert.Equal(t, map[string]string{"home": CodeConflict}, codes(report.Skipped))
	assert.NoFileExists(t, filepath.Join(dir, activeDir, "home-v2.yaml"))
}

func TestPromote_DryRun_TouchesNothing(t *testing.T) {
	t.Parallel()
	dir := project(t)
	put(t, dir, candDir+"sign-in-dashboard.yaml", acceptanceScenario)
	put(t, dir, candDir+"home.yaml", homeScenario("Hi"))
	put(t, dir, activeDir+"home.yaml", homeScenario("Hello"))

	report, err := Promote(dir, Options{All: true, DryRun: true})
	require.NoError(t, err)
	assert.True(t, report.DryRun)
	require.Len(t, report.Promoted, 1)
	assert.Equal(t, "sign-in-dashboard", report.Promoted[0].ID)
	assert.Equal(t, map[string]string{"home": CodeConflict}, codes(report.Skipped),
		"a dry run predicts the conflict instead of promising a move")
	assert.FileExists(t, filepath.Join(dir, candDir, "sign-in-dashboard.yaml"))
	assert.NoFileExists(t, filepath.Join(dir, activeDir, "sign-in-dashboard.yaml"))
}

func TestPromote_Selection(t *testing.T) {
	t.Parallel()
	dir := project(t)
	put(t, dir, candDir+"sign-in-dashboard.yaml", acceptanceScenario)
	put(t, dir, candDir+"broken.yaml", "schema_version: qamesh.scenario.v2\nid: broken\n")

	_, err := Promote(dir, Options{})
	assert.Equal(t, CodeSelectionInvalid, ErrorCode(err))
	_, err = Promote(dir, Options{All: true, IDs: []string{"broken"}})
	assert.Equal(t, CodeSelectionInvalid, ErrorCode(err))

	report, err := Promote(dir, Options{IDs: []string{"broken", "nope"}})
	require.NoError(t, err)
	assert.Empty(t, report.Promoted)
	assert.Equal(t, map[string]string{"broken": CodeInvalid, "nope": CodeCandidateMissing}, codes(report.Skipped))
	assert.FileExists(t, filepath.Join(dir, candDir, "sign-in-dashboard.yaml"), "an unnamed candidate is not touched")
}
