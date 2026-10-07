package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-001 REQ-21 / S20: the help text, docs/health-band.md, and
// CHANGELOG.md document the formula, the fixed constants, the tiers, the
// diagnosis-only tier 3, the upgrade-before-enable note for health_band, and
// scheduling through cron or /auto schedule.

// bandDocRequired are the statements every band document carries. The
// constants come from pkg/healthband, so a constant change fails here until
// the documents follow.
func bandDocRequired() []string {
	return []string{
		"z = (x - μ) / max(sd, 1/K)",
		fmt.Sprintf("K=%d", healthband.BlockSize),
		fmt.Sprintf("W=%d", healthband.BaselineWindow),
		fmt.Sprintf("N_min=%d", healthband.MinBaseline),
		fmt.Sprintf("floor=1/K=%v", healthband.VarianceFloor),
		"ε=1e-9",
		"Tier 3 is diagnosis-only",
		"cron",
		"/auto schedule",
		"health_band.diagnosis_provider",
		"Upgrade every auto binary that reads this autopus.yaml before you set health_band",
	}
}

// flowText joins every run of whitespace, so a statement wrapped across lines
// still matches.
func flowText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func TestReactBandHelp_LongDocumentsDetectorTiersAndScheduling(t *testing.T) {
	t.Parallel()

	require.Equal(t, 1e-9, healthband.TierEpsilon, "the documents state ε=1e-9")
	help := flowText(reactBandLong)
	for _, want := range bandDocRequired() {
		assert.Contains(t, help, want)
	}
	assert.Contains(t, help, "auto react band")
	assert.NotContains(t, reactBandLong, "allow_draft_pr", "the draft PR flag belongs to SPEC-SIGMABAND-002")
	for i, line := range strings.Split(reactBandLong, "\n") {
		assert.LessOrEqual(t, len([]rune(line)), 80, "help line %d is wider than 80 columns: %q", i+1, line)
	}
}

func TestReactBandHelp_ShortIsOneReadOnlyLine(t *testing.T) {
	t.Parallel()

	assert.NotContains(t, reactBandShort, "\n")
	assert.Contains(t, reactBandShort, "read-only")
}

func TestReactBandHelp_OperatorDocMatchesHelp(t *testing.T) {
	t.Parallel()

	doc := flowText(readRepoFile(t, "docs", "health-band.md"))
	for _, want := range bandDocRequired() {
		assert.Contains(t, doc, want, "docs/health-band.md")
	}
	assert.Contains(t, doc, "auto react band --dry-run")
	assert.Contains(t, doc, ".autopus/metrics/")
}

func TestReactBandHelp_ChangelogNamesCommandAndUpgradeNote(t *testing.T) {
	t.Parallel()

	changelog := flowText(readRepoFile(t, "CHANGELOG.md"))
	for _, want := range []string{"auto react band", "health_band", "docs/health-band.md", "켜기 전에"} {
		assert.Contains(t, changelog, want, "CHANGELOG.md")
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
	require.NoError(t, err)
	return string(data)
}
