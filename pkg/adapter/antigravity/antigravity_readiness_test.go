package antigravity

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAntigravityRunner struct {
	output []byte
	err    error
	args   [][]string
}

func (s *stubAntigravityRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	s.args = append(s.args, args)
	return s.output, s.err
}

// The probe must never reach beyond `--version`. Anything else would start a
// provider turn or mutate the user's global plugin registry.
func TestProbeAntigravityReadiness_OnlyReadsVersion(t *testing.T) {
	t.Parallel()
	runner := &stubAntigravityRunner{output: []byte("1.1.26\n")}

	report := ProbeAntigravityReadiness(context.Background(), AntigravityReadinessOptions{Runner: runner})

	assert.Equal(t, [][]string{{"--version"}}, runner.args)
	assert.Equal(t, "1.1.26", report.Version)
	assert.Equal(t, cliBinary, report.Executable)
}

func TestProbeAntigravityReadiness_VerifiedFloorSupportsEveryEmittedComponent(t *testing.T) {
	t.Parallel()
	runner := &stubAntigravityRunner{output: []byte("1.1.26\n")}

	report := ProbeAntigravityReadiness(context.Background(), AntigravityReadinessOptions{Runner: runner})

	require.Len(t, report.Capabilities, len(antigravityCapabilityFloors))
	for _, capability := range report.Capabilities {
		assert.True(t, capability.Supported, "%s: %s", capability.ID, capability.Reason)
		assert.Equal(t, "verified_min_version", capability.Reason)
	}
	assert.Empty(t, report.Unsupported())
}

func TestProbeAntigravityReadiness_NewerVersionStillSupported(t *testing.T) {
	t.Parallel()
	runner := &stubAntigravityRunner{output: []byte("1.2.2\n")}

	report := ProbeAntigravityReadiness(context.Background(), AntigravityReadinessOptions{Runner: runner})

	assert.Equal(t, "1.2.2", report.Version)
	assert.Empty(t, report.Unsupported())
}

// An older CLI must be reported as unsupported instead of being silently
// treated as capable: the generated plugin components were only ever observed
// on the verified floor.
func TestProbeAntigravityReadiness_OlderVersionReportsBelowFloor(t *testing.T) {
	t.Parallel()
	runner := &stubAntigravityRunner{output: []byte("1.0.9\n")}

	report := ProbeAntigravityReadiness(context.Background(), AntigravityReadinessOptions{Runner: runner})

	unsupported := report.Unsupported()
	require.Len(t, unsupported, len(antigravityCapabilityFloors))
	for _, capability := range unsupported {
		assert.Equal(t, "below_min_version", capability.Reason)
		assert.Equal(t, antigravityVerifiedFloor, capability.MinVersion)
	}
}

func TestProbeAntigravityReadiness_UnparseableVersionNeverClaimsSupport(t *testing.T) {
	t.Parallel()
	runner := &stubAntigravityRunner{output: []byte("some unexpected banner\n")}

	report := ProbeAntigravityReadiness(context.Background(), AntigravityReadinessOptions{Runner: runner})

	assert.Empty(t, report.Version)
	require.Len(t, report.Unsupported(), len(antigravityCapabilityFloors))
	assert.Equal(t, "version_unparsed", report.Capabilities[0].Reason)
}

func TestProbeAntigravityReadiness_ProbeFailureNeverClaimsSupport(t *testing.T) {
	t.Parallel()
	runner := &stubAntigravityRunner{err: errors.New("exec: \"agy\": executable file not found in $PATH")}

	report := ProbeAntigravityReadiness(context.Background(), AntigravityReadinessOptions{Runner: runner})

	require.Len(t, report.Unsupported(), len(antigravityCapabilityFloors))
	assert.Equal(t, "probe_failed", report.Capabilities[0].Reason)
}

func TestProbeAntigravityReadiness_OversizedOutputIsRejected(t *testing.T) {
	t.Parallel()
	runner := &stubAntigravityRunner{output: make([]byte, 64)}

	report := ProbeAntigravityReadiness(context.Background(), AntigravityReadinessOptions{
		Runner:    runner,
		MaxOutput: 8,
	})

	assert.Empty(t, report.Version)
	assert.Equal(t, "output_oversized", report.Capabilities[0].Reason)
}

func TestCompareAntigravityVersions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		left, right string
		want        int
	}{
		{"1.1.26", "1.1.26", 0},
		{"1.1.26", "1.1.9", 1},
		{"1.1.9", "1.1.26", -1},
		{"1.2.0", "1.1.26", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.2", "1.2.0", 0},
	}
	for _, tc := range cases {
		left, ok := parseAntigravityVersion(tc.left)
		require.True(t, ok, tc.left)
		right, ok := parseAntigravityVersion(tc.right)
		require.True(t, ok, tc.right)
		assert.Equal(t, tc.want, compareAntigravityVersions(left, right), "%s vs %s", tc.left, tc.right)
	}
}

func TestParseAntigravityVersion_RejectsNonVersionBanner(t *testing.T) {
	t.Parallel()
	for _, banner := range []string{"", "agy", "No imported plugins.", "vNext"} {
		_, ok := parseAntigravityVersion(banner)
		assert.False(t, ok, banner)
	}
	parsed, ok := parseAntigravityVersion("v1.1.26\n")
	require.True(t, ok)
	assert.Equal(t, [3]int{1, 1, 26}, parsed)
}
