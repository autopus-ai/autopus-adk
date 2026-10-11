package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	driverQuestion        = "구성을 바꿀까요? [Enter] 유지 · g) 역할 그룹 · a) 고급\n"
	driverRefreshQuestion = "구성을 바꿀까요? [Enter] 유지 · u) 최신 기본값으로 갱신 · g) 역할 그룹 · a) 고급\n"
	driverToolsQuestion   = "구성을 바꿀까요? [Enter] 유지 · g) 역할 그룹 · a) 고급 · t) 도구별 설정\n"
	driverConfirm         = "적용할까요? [y/N] · e) 계속 편집\n"
	driverCancelled       = "취소했습니다. 설정을 바꾸지 않았습니다.\n"
	driverInvalidKeep     = "입력이 올바르지 않아 구성을 유지합니다.\n"
	driverApplied         = "구성을 적용했습니다.\n"
)

// TestComposeDriverSession_KeepPathsWriteNothing pins INV-MUX-01: Enter, input
// end, and fewer than three invalid answers before Enter all keep.
func TestComposeDriverSession_KeepPathsWriteNothing(t *testing.T) {
	inputs := map[string]string{
		"enter":                "\n",
		"eof":                  "",
		"blank then eof":       "   ",
		"two invalid then key": "x\nx\n\n",
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			session, table, planner := newDriverSession(driverConfig("claude-code"))

			result, out, err := runDriver(t, session, input)

			require.NoError(t, err)
			assert.Equal(t, composeDriverKept, result.Outcome)
			assert.Empty(t, planner.edits)
			assert.Zero(t, planner.refreshes)
			assert.Equal(t, 1, table.draws())
			assert.Contains(t, out, driverQuestion)
			assert.NotContains(t, out, driverInvalidKeep)
		})
	}
}

// TestComposeDriverSession_ThirdInvalidAnswerKeeps pins REQ-MUX-13: invalid
// answers re-show the choices, and the third keeps without reading further.
// `u` is invalid without a historical preset, as `t` is without tools.
func TestComposeDriverSession_ThirdInvalidAnswerKeeps(t *testing.T) {
	session, _, planner := newDriverSession(driverConfig("claude-code"))

	result, out, err := runDriver(t, session, "x\nu\nt\ng\n")

	require.NoError(t, err)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.Equal(t, 3, strings.Count(out, driverQuestion))
	assert.Equal(t, 1, strings.Count(out, driverInvalidKeep))
	assert.Empty(t, planner.edits, "the line after the third invalid answer must stay unread")
}

// TestComposeDriverSession_RefreshAppliesOnlyOnYes pins REQ-MUX-06 and
// REQ-MUX-11: `u` previews the refresh, and only y or yes writes it.
func TestComposeDriverSession_RefreshAppliesOnlyOnYes(t *testing.T) {
	answers := map[string]bool{
		"y\n": true, "YES\n": true, "Y": true,
		"n\n": false, "N\n": false, "\n": false, "maybe\n": false, "": false,
	}
	for answer, applies := range answers {
		t.Run(answer, func(t *testing.T) {
			session, table, planner := newDriverSession(driverHistoricalConfig("claude-code"))

			result, out, err := runDriver(t, session, "u\n"+answer)

			require.NoError(t, err)
			assert.Contains(t, out, driverRefreshQuestion)
			assert.Equal(t, 1, planner.refreshes)
			assert.Len(t, table.previews, 1)
			assert.Contains(t, out, "미리보기 (아직 바뀌지 않음)\n[preview]\n"+
				"바뀌는 설정:\n  quality.presets.ultra ← 최신 기본값\n"+driverConfirm)
			if applies {
				assert.Equal(t, composeDriverApplied, result.Outcome)
				assert.Equal(t, []map[string]string{planner.refreshed}, planner.applied)
				assert.Equal(t, planner.refreshed, result.Plan.Composition)
				assert.Contains(t, out, driverApplied)
				return
			}
			assert.Equal(t, composeDriverKept, result.Outcome)
			assert.Empty(t, planner.applied)
			assert.Contains(t, out, driverCancelled)
		})
	}
}

// TestComposeDriverSession_OneSnapshotFeedsEveryTable pins REQ-MUX-05: the run
// probes the catalog once and the table, the preview, and the caller's
// generation all read that one snapshot.
func TestComposeDriverSession_OneSnapshotFeedsEveryTable(t *testing.T) {
	session, table, _ := newDriverSession(driverHistoricalConfig("claude-code", "codex"))
	probes := stubComposeCatalogProbe(t, composeCatalogA, nil)
	cmd, _ := newDriverCommand(strings.NewReader("u\ny\n"))

	result, err := session.run(cmd)

	require.NoError(t, err)
	assert.Len(t, *probes, 1)
	assert.Equal(t, []byte(composeCatalogA), result.Snapshot.codexCatalog)
	assert.Equal(t, []driftGenerationSnapshot{result.Snapshot}, table.tables)
	require.Len(t, table.previews, 1)
	assert.Equal(t, result.Snapshot, table.previews[0].snapshot)
	assert.Same(t, session.Config, table.previews[0].start)
	assert.Same(t, result.Plan.Candidate, table.previews[0].candidate)
}

// TestComposeDriverSession_RefusedWriteKeepsAndReturnsTheError leaves the
// warning to the entry point and still hands back the snapshot.
func TestComposeDriverSession_RefusedWriteKeepsAndReturnsTheError(t *testing.T) {
	session, _, planner := newDriverSession(driverHistoricalConfig("claude-code", "codex"))
	planner.applyErr = errQualityWriteRefused

	result, out, err := runDriver(t, session, "u\ny\n")

	require.ErrorIs(t, err, errQualityWriteRefused)
	assert.Equal(t, composeDriverKept, result.Outcome)
	assert.True(t, result.Snapshot.codexCatalogFixed)
	assert.NotContains(t, out, driverApplied)
}

// TestComposeDriverSession_FailuresBeforeTheWriteKeep covers a planner error,
// a table error before the question, and a plan without a write.
func TestComposeDriverSession_FailuresBeforeTheWriteKeep(t *testing.T) {
	t.Run("planner", func(t *testing.T) {
		session, _, planner := newDriverSession(driverHistoricalConfig("claude-code"))
		planner.planErr = errors.New("plan failed")
		result, _, err := runDriver(t, session, "u\ny\n")
		require.EqualError(t, err, "plan failed")
		assert.Equal(t, composeDriverKept, result.Outcome)
	})
	t.Run("table", func(t *testing.T) {
		session, table, _ := newDriverSession(driverConfig("claude-code"))
		table.err = errors.New("draw failed")
		stubComposeCatalogProbe(t, composeCatalogA, nil)
		cmd, out := newDriverCommand(driverFailingReader{t})
		result, err := session.run(cmd)
		require.EqualError(t, err, "draw failed")
		assert.Equal(t, composeDriverKept, result.Outcome)
		assert.NotContains(t, out.String(), driverQuestion)
	})
	t.Run("preview", func(t *testing.T) {
		session, table, planner := newDriverSession(driverHistoricalConfig("claude-code"))
		table.previewErr = errors.New("preview failed")
		result, out, err := runDriver(t, session, "u\ny\n")
		require.EqualError(t, err, "preview failed")
		assert.Equal(t, composeDriverKept, result.Outcome)
		assert.Empty(t, planner.applied)
		assert.NotContains(t, out, driverConfirm)
	})
	t.Run("edit planner", func(t *testing.T) {
		session, table, planner := newDriverSession(driverConfig("claude-code"))
		planner.planErr = errors.New("plan failed")
		result, _, err := runDriver(t, session, "g\n1\n\n\n\ny\n")
		require.EqualError(t, err, "plan failed")
		assert.Equal(t, composeDriverKept, result.Outcome)
		assert.Empty(t, table.previews)
	})
	t.Run("plan without composition", func(t *testing.T) {
		session, _, planner := newDriverSession(driverHistoricalConfig("claude-code"))
		planner.refreshed = nil
		_, _, err := runDriver(t, session, "u\ne\na\ndebugger=opus\n\ny\n")
		require.NoError(t, err)
		assert.Equal(t, []map[string]string{{"debugger": "opus"}}, planner.applied)
	})
	t.Run("no write", func(t *testing.T) {
		session, _, planner := newDriverSession(driverHistoricalConfig("claude-code"))
		planner.noWrite = true
		result, _, err := runDriver(t, session, "u\ny\n")
		require.Error(t, err)
		assert.Equal(t, composeDriverKept, result.Outcome)
	})
}

// TestComposeDriverSession_ToolsRouteThroughTheSharedReader pins REQ-MUX-16
// and REQ-MUX-17: `t` and the existing target names reach the existing
// editors, which read the answers queued behind the menu from one reader.
func TestComposeDriverSession_ToolsRouteThroughTheSharedReader(t *testing.T) {
	cases := []struct {
		input  string
		routes []string
		read   []string
	}{
		{"t\nglobal\n1\n", []string{""}, []string{"global", "1"}},
		{"claude-code\nexecutor=fable\n\n", []string{"claude-code"}, []string{"executor=fable", ""}},
		{"CODEX\n\ny\n", []string{"codex"}, []string{"", "y"}},
		{"omp\nx\nGlobal\n1\n2\n", []string{"global"}, []string{"1", "2"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			stubComposeCatalogProbe(t, composeCatalogA, nil)
			cmd, out := newDriverCommand(strings.NewReader(tc.input))
			session, _, planner := newDriverSession(driverConfig("claude-code", "codex"))
			tools := &driverFakeTools{cmd: cmd, targets: []string{"global", "claude-code", "codex"}, reads: 2}
			session.Tools = tools

			result, err := session.run(cmd)

			require.NoError(t, err)
			assert.Equal(t, composeDriverRouted, result.Outcome)
			assert.Equal(t, tc.routes, tools.routes)
			assert.Equal(t, tc.read, tools.read)
			assert.Contains(t, out.String(), driverToolsQuestion)
			assert.Empty(t, planner.edits)
		})
	}
}

// TestComposeDriverSession_RouteErrorPropagates keeps the existing editors'
// errors, such as an invalid preset choice, visible to the command.
func TestComposeDriverSession_RouteErrorPropagates(t *testing.T) {
	stubComposeCatalogProbe(t, composeCatalogA, nil)
	cmd, _ := newDriverCommand(strings.NewReader("global\n"))
	session, _, _ := newDriverSession(driverConfig("claude-code"))
	session.Tools = &driverFakeTools{cmd: cmd, targets: []string{"global"}, err: errors.New("invalid quality choice")}

	result, err := session.run(cmd)

	require.EqualError(t, err, "invalid quality choice")
	assert.Equal(t, composeDriverRouted, result.Outcome)
}
