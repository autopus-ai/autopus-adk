package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Security L4: a react report is read only through a real .autopus and a
// real .autopus/react directory, so a symlinked .autopus cannot pull a file
// from outside the project into a prompt or a BS.
func TestReactBandReport_RefusesASymlinkedAutopusDirectory(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "react"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "react", "4242.md"), []byte("outside secret notes\n"), 0o600))
	project := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(project, ".autopus")))

	_, ok := readBandReactReport(project, 4242)
	assert.False(t, ok)

	real := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(real, ".autopus", "react"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(real, ".autopus", "react", "4242.md"), []byte("step 3 failed\n"), 0o600))
	evidence, ok := readBandReactReport(real, 4242)
	require.True(t, ok)
	assert.Equal(t, "step 3 failed", evidence.Text)
}

// Security L6: a symlink named like a BS does not raise the BS ID, and band
// names its path on stderr, quoted, outside the report.
func TestReactBand_ReportsIgnoredBSEntriesOnStderr(t *testing.T) {
	t.Parallel()
	p := newBandCmdProject(t)
	p.store(healthband.CIRunsFile, bandO2CI())
	brainstorms := filepath.Join(p.dir, ".autopus", "brainstorms")
	require.NoError(t, os.MkdirAll(brainstorms, 0o700))
	link := filepath.Join(brainstorms, "BS-BAND-900.md")
	require.NoError(t, os.Symlink(filepath.Join(p.dir, "autopus.yaml"), link))

	run := p.run(emptyRunList(), nil, "--no-fetch", "--format", "json")

	require.NoError(t, run.err)
	assert.Equal(t, "BS-BAND-001", decodeBandEnvelope(t, run.stdout).check(t, "band.ci.failure_rate:CI").Fields["bs_id"])
	assert.Contains(t, run.stderr, `react band: BS-BAND ID scan ignored "`+link+`"`)
	assert.NotContains(t, run.stdout, link)
}

// Security L6, review round 2: a valid BS planted at the top of the ID range
// does not block the diagnosis BS, which takes the lowest free ID, and band
// names the planted path on its warning writer (stderr).
func TestReactBandDiagnose_NamesAPlantedRangeEndAndStillWritesTheBS(t *testing.T) {
	t.Parallel()
	fixture := newBandDiagnoseFixture(t)
	dir := filepath.Join(fixture.projectDir, ".autopus", "brainstorms")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	planted, err := brainstorm.Render("BS-BAND-999999999", brainstorm.Request{
		Evaluation: fixture.claim.Event.Evaluation, EpisodeID: fixture.claim.EpisodeID,
		Created: bandDiagnoseT0, DiagnosisStatus: bandDiagnosisSkippedNoAgent,
	})
	require.NoError(t, err)
	path := filepath.Join(dir, "BS-BAND-999999999.md")
	require.NoError(t, os.WriteFile(path, planted, 0o600))
	d := fixture.diagnoser(nil, nil)
	d.noAgent = true
	var warn bytes.Buffer
	d.warn = &warn

	outcome := d.Run(context.Background(), fixture.claim)

	assert.Equal(t, "BS-BAND-001", outcome.BSID)
	assert.Equal(t, bandBSWritten, outcome.BSStatus)
	assert.Contains(t, warn.String(), `react band: BS-BAND ID range end is held by "`+path+`"`)
}
