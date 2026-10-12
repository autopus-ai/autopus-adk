package cli

// SPEC-MODELUX-002 T2 (REQ-MUX2-12, REQ-MUX2-13, REQ-MUX2-16; acceptance S10,
// S11): the platforms an update detects join the composition table and the
// run's one Codex snapshot through a display view, every write still targets
// the loaded config, and the detected platforms generate from that snapshot.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const composeViewStatusD = "quality.default = balanced   ultra: 현재 기본값   balanced: 현재 기본값"

// newComposeDetectRun is the isolated `auto update` environment of a run
// that detects platforms: the TTY and CI seams, Korean text, the run's
// catalog probe answering CAT-A, and a PATH holding only stand-ins for clis.
// The codex stand-in logs every call and answers `debug models` with CAT-D,
// which refuses the standard balanced placement: an adapter that probed for
// itself would fail the generation the table showed. The omp stand-in passes
// the identity probe; every other stand-in only exists.
func newComposeDetectRun(t *testing.T, clis ...string) composeUpdateRun {
	t.Helper()
	stubDriverProbes(t, true, "")
	setComposeLanguage(t, "ko")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "codex-calls.log")
	for _, cli := range clis {
		script := "#!/bin/sh\nexit 0\n"
		switch cli {
		case "codex":
			script = "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + marker + "'\n" +
				"if [ \"$1\" = debug ] && [ \"$2\" = models ]; then printf '%s' '" +
				mustReadComposeCatalog(t, "CAT-D") + "'; fi\n"
		case "omp":
			script = "#!/bin/sh\nprintf 'omp/17.1.8\\n'\n"
		}
		require.NoError(t, os.WriteFile(filepath.Join(bin, cli), []byte(script), 0o755))
	}
	t.Setenv("PATH", bin)
	return composeUpdateRun{probes: composeSessionCatalog(t, "CAT-A"), codexLog: marker}
}

// S10 (DC): Codex is detected only, yet its column, the run's one catalog
// probe, and the same run's Codex generation all come from that probe.
func TestUpdateCompose_DetectedCodexJoinsTheTableAndGeneratesFromTheSnapshot(t *testing.T) {
	run := newComposeDetectRun(t, "codex")
	dir := writeComposeFixture(t, "claude-code")

	out, err := runComposeUpdate(t, strings.NewReader("\n"), "--dir", dir)

	require.NoError(t, err, out)
	assert.Contains(t, out, "  + 새 플랫폼 감지: codex\n")
	assert.Equal(t, []string{"그룹", "에이전트", "등급", "Claude Code", "Codex"}, composeTableHeader(t, out, composeViewStatusD))
	assert.Equal(t, "6-astra · max", composeRenderedTable(t, out, composeViewStatusD)["planner"]["Codex"])
	assert.Contains(t, out, "Codex catalog: 읽음 · 대체 0칸\n")
	assert.Len(t, *run.probes, 1)
	assert.False(t, run.adapterProbed(t), "the detected Codex generation read the run's snapshot")
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "max"}, readComposeCodexProfile(t, dir, "planner"))
	assert.Equal(t, []string{"claude-code", "codex"}, composeBindLoad(t, dir).Platforms)
}

// S10 (DP): every platform the update will generate has its column, in the
// table's fixed order; OMP without a profile and OpenCode keep the platform
// default.
func TestUpdateCompose_DetectedPlatformsAddTheirColumns(t *testing.T) {
	newComposeDetectRun(t, "codex", "opencode", "omp")
	dir := writeComposeFixture(t, "claude-code", "codex")

	out, err := runComposeUpdate(t, strings.NewReader("\n"), "--dir", dir)

	require.NoError(t, err, out)
	header := composeTableHeader(t, out, composeViewStatusD)
	assert.Equal(t, []string{"Claude Code", "Codex", "OMP", "OpenCode"}, header[3:])
	for agent, row := range composeRenderedTable(t, out, composeViewStatusD) {
		assert.Equal(t, "플랫폼 기본값", row["OMP"], agent)
		assert.Equal(t, "플랫폼 기본값", row["OpenCode"], agent)
	}
}

// S11, keep path with detection: a canonical file differs only by the
// detected platforms, appended in detection order; a hand-written file is
// rewritten whole by the detection save, so its decoded quality and
// role_model_policy are what stay; without detection it is byte-identical.
func TestUpdateCompose_KeepWithDetectionChangesOnlyThePlatforms(t *testing.T) {
	t.Run("canonical", func(t *testing.T) {
		newComposeDetectRun(t, "codex", "opencode", "omp")
		dir := writeComposeFixture(t, "claude-code", "codex")
		before := readBlockFixture(t, dir)

		out, err := runComposeUpdate(t, strings.NewReader("\n"), "--dir", dir)

		require.NoError(t, err, out)
		codexLine := regexp.MustCompile(`(?m)^( *)- codex\n`).FindStringSubmatch(before)
		require.NotNil(t, codexLine, before)
		indent := codexLine[1]
		want := strings.Replace(before, codexLine[0], codexLine[0]+indent+"- opencode\n"+indent+"- omp\n", 1)
		assert.Equal(t, want, readBlockFixture(t, dir))
	})
	t.Run("hand-written with detection", func(t *testing.T) {
		newComposeDetectRun(t, "codex", "opencode")
		dir := writeComposeW2(t)
		before := readBlockFixture(t, dir)

		out, err := runComposeUpdate(t, strings.NewReader("\n"), "--dir", dir)

		require.NoError(t, err, out)
		after := readBlockFixture(t, dir)
		assert.NotEqual(t, before, after, "the detection save rewrites the file")
		assert.Equal(t, rawQualityOf(t, before), rawQualityOf(t, after))
		assert.Equal(t, rawRoleModelPolicyOf(t, before), rawRoleModelPolicyOf(t, after))
	})
	t.Run("hand-written without detection", func(t *testing.T) {
		newComposeDetectRun(t, "codex")
		dir := writeComposeW2(t)
		before := composeSessionHash(t, dir)

		out, err := runComposeUpdate(t, strings.NewReader("\n"), "--dir", dir)

		require.NoError(t, err, out)
		assert.Equal(t, before, composeSessionHash(t, dir))
	})
}

// A confirmed composition while Codex is detected: the write targets the
// loaded config, so the detection save that follows keeps it, and the
// detected Codex generates exactly the approved preview (REQ-MUX2-16).
func TestUpdateCompose_ConfirmedEditWithDetectedCodexGeneratesThePreview(t *testing.T) {
	run := newComposeDetectRun(t, "codex")
	dir := writeComposeFixture(t, "claude-code")

	out, err := runComposeUpdate(t, strings.NewReader("g\n\n3\n\n4\ny\n"), "--dir", dir)

	require.NoError(t, err, out)
	saved := composeBindLoad(t, dir)
	assert.Equal(t, "custom", saved.Quality.Default)
	assert.Equal(t, []string{"claude-code", "codex"}, saved.Platforms)
	preview := composeRenderedTable(t, out, composeTextFor("ko").PreviewHeading)
	claudeMarks, codexMarks := assertGeneratedEqualsTable(t, dir, preview)
	assert.Positive(t, claudeMarks)
	assert.Positive(t, codexMarks)
	assert.Len(t, *run.probes, 1)
	assert.False(t, run.adapterProbed(t))
}
