package cli

// SPEC-MODELUX-002 T7 (REQ-MUX2-01, REQ-MUX2-11, REQ-MUX2-13, REQ-MUX2-14;
// acceptance S2, S8, S10, S11, S12): the real `auto update` takes the form
// flow in a terminal, keeps the file on every keep path, ends the whole
// command on Ctrl+C or on an OMP write it cannot read back, and generates
// a detected Codex from the run's snapshot.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// S2: Enter at the update menu keeps autopus.yaml and the update generates.
func TestUpdateComposeForms_EnterKeepsAndGenerates(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	dir := run.settled(t, "R")
	useAccessibleComposeForms(t)
	before := composeSessionHash(t, dir)

	out, err := runComposeUpdate(t, formInput("\n"), "--dir", dir)

	require.NoError(t, err, out)
	assert.Contains(t, out, composeFormMenuR)
	assert.Contains(t, out, "Update complete: 5 platform(s) updated")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// S10: a Codex tab edit while Codex is only detected: the tab offers Codex
// from the view, the write binds codex-custom in the file the detection
// save then extends, and the detected Codex generates the edit from the
// run's one probe.
func TestUpdateComposeForms_DetectedCodexTabEdit(t *testing.T) {
	run := newComposeDetectRun(t, "codex")
	useAccessibleComposeForms(t)
	dir := writeComposeFixture(t, "claude-code")

	out, err := runComposeUpdate(t, formInput(composeFormLines(composeFormS5, "2\n", "\n")...), "--dir", dir)

	require.NoError(t, err, out)
	assert.Contains(t, out, composeFormTabD)
	saved := composeBindLoad(t, dir)
	assert.Contains(t, saved.Platforms, "codex")
	assert.Equal(t, "codex-custom", saved.Quality.Providers["codex"])
	assert.Equal(t, config.CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"}, readComposeCodexProfile(t, dir, "planner"))
	assert.Len(t, *run.probes, 1)
	assert.False(t, run.adapterProbed(t))
}

// S11: every keep path of the form flow writes nothing.
func TestUpdateComposeForms_KeepPathsWriteNothing(t *testing.T) {
	base := newComposeUpdateRun(t, true, "")
	settled := base.settled(t, "R")
	cases := []struct {
		name  string
		dir   func(*testing.T) string
		lines []string
	}{
		{"R keep", func(t *testing.T) string { return cloneComposeProject(t, settled) }, []string{"\n"}},
		{"O3 keep", composeO3, []string{"\n"}},
		{"W2 keep", writeComposeW2, []string{"\n"}},
		{"Codex groups unchanged", composeO3, []string{"2\n", "3\n", "\n", "\n", "\n", "\n", "\n", "\n"}},
		{"Codex edit cancelled", composeO3, composeFormLines(composeFormS5, "\n", "\n")},
		{"OMP without a pin", composeO3, []string{"2\n", "4\n", "4\n", "1\n", "\n", "\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base.rerun(t, true, "")
			useAccessibleComposeForms(t)
			deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())
			useComposeUpdateOMPDeps(t, deps)
			usePlatformOMPModelRunner(t, &ompCLIFakeRunner{fail: true})
			dir := tc.dir(t)
			before := composeSessionHash(t, dir)

			out, err := runComposeUpdate(t, formInput(tc.lines...), "--dir", dir)

			require.NoError(t, err, out)
			assert.Equal(t, before, composeSessionHash(t, dir))
			assert.Zero(t, activations.calls)
		})
	}
}

// S12: Ctrl+C at the menu ends the update with an error before any
// generation; the generated files keep their bytes and times.
func TestUpdateComposeForms_InterruptEndsTheUpdate(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	dir := run.settled(t, "R")
	abortComposeFormAt(t, 1)
	before := composeSessionHash(t, dir)
	generated := filepath.Join(dir, ".codex", "agents", "planner.toml")
	data, err := os.ReadFile(generated)
	require.NoError(t, err)
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(generated, stamp, stamp))

	out, err := runComposeUpdate(t, formInput(), "--dir", dir)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errComposeInterrupted), "%v", err)
	assert.Contains(t, out, "중단했습니다. 구성을 더 바꾸지 않습니다.\n")
	assert.NotContains(t, out, "→ Updating")
	assert.Equal(t, before, composeSessionHash(t, dir))
	after, err := os.ReadFile(generated)
	require.NoError(t, err)
	assert.Equal(t, data, after)
	info, err := os.Stat(generated)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(stamp), info.ModTime().String())
}

// S12: a write the run applied before Ctrl+C stays in the file.
func TestUpdateComposeForms_InterruptAfterAWriteKeepsTheWrite(t *testing.T) {
	newComposeUpdateRun(t, true, "")
	abortComposeFormAt(t, 9)
	dir := writeComposeFixture(t, "claude-code", "codex")

	out, err := runComposeUpdate(t, formInput(composeFormLines(composeFormS5, "2\n")...), "--dir", dir)

	require.Error(t, err)
	assert.Contains(t, out, "구성을 적용했습니다.\n")
	assert.NotContains(t, out, "→ Updating")
	assert.Equal(t, "codex-custom", composeBindLoad(t, dir).Quality.Providers["codex"])
}

// S8: an OMP write the update cannot read back ends the update with an
// error before every generation; the file keeps the applied policy.
func TestUpdateComposeForms_FailedOMPReReadEndsTheUpdate(t *testing.T) {
	newComposeDetectRun(t, "codex", "omp")
	useAccessibleComposeForms(t)
	deps, _ := composeOMPDeps(ompCLIBalancedCatalogJSON())
	useComposeUpdateOMPDeps(t, deps)
	original := composeReloadConfig
	t.Cleanup(func() { composeReloadConfig = original })
	composeReloadConfig = func(string) (*config.HarnessConfig, error) { return nil, errors.New("disk gone") }
	dir := writeNativeComposeFixture(t, "claude-code", "codex", "omp")

	out, err := runComposeUpdate(t, formInput(composeUpdateS8...), "--dir", dir)

	var reload composeReloadError
	require.ErrorAs(t, err, &reload)
	assert.Contains(t, out, "OMP 설정은 적용됐지만 다시 읽지 못했습니다: disk gone\n")
	assert.NotContains(t, out, "→ Updating")
	assert.False(t, strings.Contains(out, "Update complete"))
	assert.Equal(t, "balanced", rawRoleModelPolicyOf(t, readBlockFixture(t, dir)).Profile)
}
