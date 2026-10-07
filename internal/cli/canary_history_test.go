package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// historyCmd is a bare command whose stderr is captured; withContext false
// leaves Context() nil, as for a command run without Execute.
func historyCmd(t *testing.T, stderr *bytes.Buffer, withContext bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetErr(stderr)
	if withContext {
		cmd.SetContext(t.Context())
	}
	return cmd
}

// canaryRan builds a finished result: the verdict, then build, e2e, doctor,
// endpoint, and browser statuses in summary order.
func canaryRan(verdict string, statuses ...string) canaryResult {
	result := canaryResult{Mode: "staging-canary", Verdict: verdict}
	fields := []*string{&result.Build, &result.E2E, &result.Doctor, &result.Endpoint, &result.Browser}
	for i, field := range fields {
		*field = "SKIPPED"
		if i < len(statuses) {
			*field = statuses[i]
		}
	}
	result.Summary = canarySummary(result)
	return result
}

// S18: which finished runs become one observation, under which series, and
// with which value. Only verdict FAIL counts as 1; WARN counts as 0 (research
// Q4); a dry run or a run whose every check is SKIPPED is no observation.
func TestCanaryHistorySample_S18Cases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		opts   canaryOptions
		result canaryResult
		series string
		value  float64
	}{
		{"endpoint FAIL", canaryOptions{apiURL: "https://API.Example.com:443/health"},
			canaryRan("FAIL", "SKIPPED", "PASS", "SKIPPED", "FAIL"), "canary.failure_rate:api.example.com", 1},
		{"verdict WARN", canaryOptions{}, canaryRan("WARN", "PASS", "PASS", "WARN"), "canary.failure_rate:local", 0},
		{"verdict PASS", canaryOptions{}, canaryRan("PASS", "PASS", "PASS", "PASS"), "canary.failure_rate:local", 0},
		{"build failure early return", canaryOptions{}, canaryRan("FAIL", "FAIL"), "canary.failure_rate:local", 1},
		{"no URL with local checks run", canaryOptions{}, canaryRan("PASS", "SKIPPED", "PASS"), "canary.failure_rate:local", 0},
		{"frontend and API hosts", canaryOptions{frontendURL: "https://app.example.com", apiURL: "http://api.example.com:8080"},
			canaryRan("PASS", "PASS", "PASS", "PASS", "PASS", "PASS"), "canary.failure_rate:api.example.com:8080+app.example.com", 0},
		{"hosts sort whatever flag holds them", canaryOptions{frontendURL: "https://app.example.com", apiURL: "https://zeta.example.com"},
			canaryRan("PASS", "PASS"), "canary.failure_rate:app.example.com+zeta.example.com", 0},
		{"shared URL is one host", canaryOptions{url: "https://Preview.Example.com:443/"},
			canaryRan("FAIL", "SKIPPED", "PASS", "SKIPPED", "FAIL", "PASS"), "canary.failure_rate:preview.example.com", 1},
		{"default port of each scheme only", canaryOptions{frontendURL: "https://api.example.com:80", apiURL: "http://API.example.com:80"},
			canaryRan("PASS", "PASS"), "canary.failure_rate:api.example.com+api.example.com:80", 0},
		{"user info never reaches the series", canaryOptions{apiURL: "https://ci:ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA@api.example.com/x"},
			canaryRan("PASS", "PASS"), "canary.failure_rate:api.example.com", 0},
		{"host without a scheme", canaryOptions{apiURL: "Localhost:3000"}, canaryRan("FAIL", "SKIPPED", "PASS", "SKIPPED", "FAIL"),
			"canary.failure_rate:localhost:3000", 1},
		{"unparsable URL", canaryOptions{apiURL: "http://[::1"}, canaryRan("FAIL", "SKIPPED", "PASS", "SKIPPED", "FAIL"),
			"canary.failure_rate:unknown-host", 1},
		{"untrusted host is filtered and hashed", canaryOptions{apiURL: "https://bücher.example/health"}, canaryRan("PASS", "PASS"),
			"canary.failure_rate:bcher.example#" + healthband.H8("bücher.example"), 0},
	} {
		series, value, ok := canaryHistorySample(tc.opts, tc.result)
		require.True(t, ok, tc.name)
		assert.Equal(t, tc.series, series, tc.name)
		assert.Equal(t, tc.value, value, tc.name)
		assert.True(t, healthband.ValidSeriesID(series), tc.name)
	}
	for _, tc := range []struct {
		name   string
		opts   canaryOptions
		result canaryResult
	}{
		{"--dry-run even with a check status", canaryOptions{dryRun: true}, canaryRan("PASS", "PASS")},
		{"every check SKIPPED", canaryOptions{apiURL: "https://api.example.com"}, canaryRan("PASS")},
		{"failed before any check", canaryOptions{}, canaryResult{Verdict: "FAIL"}},
	} {
		_, _, ok := canaryHistorySample(tc.opts, tc.result)
		assert.False(t, ok, tc.name)
	}
}

func canaryHistoryLines(t *testing.T, dir string) []healthband.Observation {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".autopus", "metrics", healthband.CanaryRunsFile))
	require.NoError(t, err)
	var observations []healthband.Observation
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		var observation healthband.Observation
		require.NoError(t, json.Unmarshal([]byte(line), &observation))
		observations = append(observations, observation)
	}
	return observations
}

// S18: two consecutive appends get sample keys c1 and c2, attempt 1, and
// write nothing to stderr.
func TestCanaryHistory_AppendsSequentialSampleKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opts := canaryOptions{projectDir: dir, apiURL: "https://API.Example.com:443/health"}
	var stderr bytes.Buffer
	cmd := historyCmd(t, &stderr, true)
	recordCanaryHistory(cmd, opts, canaryRan("FAIL", "SKIPPED", "PASS", "SKIPPED", "FAIL"))
	recordCanaryHistory(cmd, opts, canaryRan("WARN", "SKIPPED", "PASS", "SKIPPED", "PASS"))
	recordCanaryHistory(cmd, canaryOptions{projectDir: dir, dryRun: true}, canaryRan("PASS"))

	assert.Empty(t, stderr.String())
	observations := canaryHistoryLines(t, dir)
	require.Len(t, observations, 2)
	for i, want := range []struct {
		key   string
		value float64
	}{{"c1", 1}, {"c2", 0}} {
		got := observations[i]
		assert.Equal(t, "canary.failure_rate:api.example.com", got.Series)
		assert.Equal(t, want.key, got.SampleKey)
		assert.Equal(t, int64(i+1), got.Tiebreak)
		assert.Equal(t, want.value, got.Value)
		assert.Equal(t, 1, got.Attempt)
		assert.Equal(t, healthband.SourceCanary, got.Source)
		assert.False(t, got.ObservedAt.IsZero())
	}
	assert.False(t, observations[1].ObservedAt.Before(observations[0].ObservedAt))
}

// The project directory defaults to "." like runCanary, and a nil context
// (a command run without Execute) still appends.
func TestCanaryHistory_DefaultsLikeRunCanary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	recordCanaryHistory(historyCmd(t, &bytes.Buffer{}, false), canaryOptions{projectDir: dir}, canaryRan("PASS", "PASS"))
	require.Len(t, canaryHistoryLines(t, dir), 1)
}

// S18: an unwritable history prints a stderr warning and writes nothing.
func TestCanaryHistory_AppendFailureIsOnlyAStderrWarning(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not bind on this platform or user")
	}
	dir := t.TempDir()
	metrics := filepath.Join(dir, ".autopus", "metrics")
	require.NoError(t, os.MkdirAll(metrics, 0o700))
	require.NoError(t, os.Chmod(metrics, 0o500))
	t.Cleanup(func() { _ = os.Chmod(metrics, 0o700) })

	var stderr bytes.Buffer
	recordCanaryHistory(historyCmd(t, &stderr, true), canaryOptions{projectDir: dir}, canaryRan("FAIL", "FAIL"))
	assert.Contains(t, stderr.String(), "canary history append failed")
	assert.Equal(t, 1, strings.Count(stderr.String(), "\n"))
	assert.NoFileExists(t, filepath.Join(metrics, healthband.CanaryRunsFile))
}
