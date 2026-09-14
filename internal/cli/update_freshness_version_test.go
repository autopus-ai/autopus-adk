package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/selfupdate"
)

// A candidate label is not the release it sits next to. The upgrade canary
// builds `0.50.109-canary` from source; when that counted as release 0.50.109
// the freshness gate installed the newer official release over it and re-execed
// that binary, so the canary asserted the released surface and the candidate's
// own cutover was never exercised.
func TestStableCurrentVersion_RejectsPrereleaseLabels(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"0.50.109-canary", "v0.50.109-canary", "0.50.109-rc1",
		"0.50.109-20", "0.50.109-gabcdef1", "0.50.109-3-gabc",
	} {
		value, ok := stableCurrentVersion(raw)
		assert.False(t, ok, raw)
		assert.Empty(t, value, raw)
	}
}

// git-describe output still names its base release: such a build carries that
// release's surface plus local commits.
func TestStableCurrentVersion_AcceptsGitDescribeShapes(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]string{
		"0.50.117":                     "0.50.117",
		"v0.50.117":                    "0.50.117",
		"v0.50.90-dirty":               "0.50.90",
		"v0.50.117-20-gffda591a":       "0.50.117",
		"v0.50.117-20-gffda591a-dirty": "0.50.117",
	} {
		value, ok := stableCurrentVersion(raw)
		assert.True(t, ok, raw)
		assert.Equal(t, want, value, raw)
	}
}

// The gate must not reach GitHub for a non-release binary: there is no "you are
// behind the latest release" claim to make about a build that never shipped.
func TestUpdateFreshness_CandidateLabelProceedsWithoutReleaseCheck(t *testing.T) {
	t.Parallel()

	checks := 0
	deps := freshnessTestDeps("0.50.109-canary", func(string) (*selfupdate.ReleaseInfo, error) {
		checks++
		return &selfupdate.ReleaseInfo{TagName: "v0.50.117"}, nil
	})
	mutated := false
	deps.resolveBinary = func() (binaryPathInfo, error) {
		mutated = true
		return binaryPathInfo{}, nil
	}
	cmd, _ := newFreshnessTestCommand()

	outcome, err := (updateFreshnessGate{deps: deps}).enforce(cmd, false, "")

	require.NoError(t, err)
	assert.Equal(t, updateFreshnessProceed, outcome)
	assert.Equal(t, 0, checks)
	assert.False(t, mutated)
}
