package cireport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fixtureAPI serves the recorded responses under testdata/api and records
// every path it was asked for.
type fixtureAPI struct {
	t       *testing.T
	mu      sync.Mutex
	paths   []string
	private bool
}

var (
	jobsPath   = regexp.MustCompile(`^repos/o/r/actions/runs/(\d+)/jobs\?`)
	commitPath = regexp.MustCompile(`^repos/o/r/git/commits/(\w+)$`)
)

func (f *fixtureAPI) Get(_ context.Context, path string) ([]byte, error) {
	f.mu.Lock()
	f.paths = append(f.paths, path)
	f.mu.Unlock()
	switch {
	case path == "repos/o/r":
		return []byte(fmt.Sprintf(`{"private": %t, "default_branch": "main"}`, f.private)), nil
	case strings.HasPrefix(path, "repos/o/r/actions/runs?"):
		u, err := url.Parse(path)
		require.NoError(f.t, err)
		if u.Query().Get("page") != "1" {
			return []byte(`{"total_count": 5, "workflow_runs": []}`), nil
		}
		return os.ReadFile(filepath.Join("testdata", "api", "runs_page1.json"))
	case jobsPath.MatchString(path):
		u, _ := url.Parse(path)
		if u.Query().Get("page") != "1" {
			return []byte(`{"total_count": 0, "jobs": []}`), nil
		}
		id := jobsPath.FindStringSubmatch(path)[1]
		return os.ReadFile(filepath.Join("testdata", "api", "jobs_"+id+".json"))
	case commitPath.MatchString(path):
		data, err := os.ReadFile(filepath.Join("testdata", "api", "commits.json"))
		require.NoError(f.t, err)
		var all map[string]json.RawMessage
		require.NoError(f.t, json.Unmarshal(data, &all))
		if c, ok := all[commitPath.FindStringSubmatch(path)[1]]; ok {
			return c, nil
		}
		return nil, fmt.Errorf("gh api %s: HTTP 404", path)
	}
	return nil, fmt.Errorf("unexpected path %s", path)
}

var (
	fixtureSince = time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	fixtureUntil = time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
)

func fixtureOptions() CollectOptions {
	return CollectOptions{Repo: "o/r", Since: fixtureSince, Until: fixtureUntil, LimitRuns: 100}
}

func collectFixture(t *testing.T) (Dataset, *fixtureAPI) {
	t.Helper()
	api := &fixtureAPI{t: t}
	ds, err := Collect(context.Background(), api, fixtureOptions())
	require.NoError(t, err)
	return ds, api
}
