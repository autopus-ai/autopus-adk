package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/cireport"
)

// ciFixtureAPI serves the recorded responses of pkg/cireport/testdata/api.
type ciFixtureAPI struct {
	mu    sync.Mutex
	count int
}

var ciJobsPath = regexp.MustCompile(`/runs/(\d+)/jobs\?.*?[?&]?\bpage=(\d+)`)

func (f *ciFixtureAPI) Get(_ context.Context, path string) ([]byte, error) {
	f.mu.Lock()
	f.count++
	f.mu.Unlock()
	dir := filepath.Join("..", "..", "pkg", "cireport", "testdata", "api")
	switch {
	case path == "repos/o/r":
		return []byte(`{"private": false}`), nil
	case strings.Contains(path, "/actions/runs?"):
		if !strings.Contains(path, "page=1&") {
			return []byte(`{"workflow_runs": []}`), nil
		}
		return os.ReadFile(filepath.Join(dir, "runs_page1.json"))
	case ciJobsPath.MatchString(path):
		m := ciJobsPath.FindStringSubmatch(path)
		if m[2] != "1" {
			return []byte(`{"jobs": []}`), nil
		}
		return os.ReadFile(filepath.Join(dir, "jobs_"+m[1]+".json"))
	case strings.Contains(path, "/git/commits/"):
		data, err := os.ReadFile(filepath.Join(dir, "commits.json"))
		if err != nil {
			return nil, err
		}
		var all map[string]json.RawMessage
		_ = json.Unmarshal(data, &all)
		if c, ok := all[path[strings.LastIndex(path, "/")+1:]]; ok {
			return c, nil
		}
	}
	return nil, errors.New("HTTP 404")
}

func (f *ciFixtureAPI) Requests() int { return f.count }

func withCIFixture(t *testing.T) {
	t.Helper()
	prevNow, prevAPI := ciNow, ciNewAPI
	ciNow = func() time.Time { return time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) }
	ciNewAPI = func(context.Context, string, int) (ciAPI, error) { return &ciFixtureAPI{}, nil }
	t.Cleanup(func() { ciNow, ciNewAPI = prevNow, prevAPI })
}

func TestCIReport_TextAndJSON(t *testing.T) {
	withCIFixture(t)
	wfDir := filepath.Join("..", "..", "pkg", "cireport", "testdata", "workflows")
	text := runDiskCLI(t, "ci", "report", "--repo", "o/r", "--workflows-dir", wfDir)
	require.Equal(t, 0, text.code, text.stderr)
	assert.Contains(t, text.stdout, "ci report: o/r, 2026-10-28..2026-11-11 (14.0 days)\n")
	assert.Contains(t, text.stdout, "sample: 4 of 4 runs (scale x1.00), 9 jobs, ")
	assert.Contains(t, text.stdout, "  1. [high] ")
	assert.Contains(t, text.stdout, "push runs re-test trees a PR run already passed")
	assert.Contains(t, text.stdout, "  CI / build: n=5 run median 15.0 p90 20.0, queue median 1.0 p90 10.0")
	assert.Contains(t, text.stdout, "  mac-1 (self-hosted): 4 jobs")

	out := runDiskCLI(t, "ci", "report", "--repo", "o/r", "--workflows-dir", wfDir, "--json")
	require.Equal(t, 0, out.code, out.stderr)
	var report cireport.Report
	require.NoError(t, json.Unmarshal(decodeDiskEnvelope(t, out.stdout).Data, &report))
	assert.Len(t, report.Duplicates, 1)
	assert.Len(t, report.Superseded, 1)
	assert.Len(t, report.Lint, 6)
	assert.Equal(t, "high", report.Findings[0].Severity)
}

func TestCIReport_UsageErrors(t *testing.T) {
	withCIFixture(t)
	for _, args := range [][]string{
		{"--repo", "o/r", "--since", "0d"},
		{"--repo", "not a repo"},
		{"--repo", "o/r", "--limit-runs", "-1"},
	} {
		out := runDiskCLI(t, append([]string{"ci", "report"}, args...)...)
		assert.Equal(t, 1, out.code, "%v: %s", args, out.stderr)
		assert.Contains(t, out.stderr, "ci report: ", args)
	}
}

// gh missing or logged out ends with exit 1 and a line that says what to do.
func TestCIReport_GHFailures(t *testing.T) {
	prevNow := ciNow
	ciNow = func() time.Time { return time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { ciNow = prevNow })
	t.Setenv("PATH", t.TempDir())
	out := runDiskCLI(t, "ci", "report", "--repo", "o/r")
	assert.Equal(t, 1, out.code)
	assert.Contains(t, out.stderr, "gh CLI not found on PATH")
}
