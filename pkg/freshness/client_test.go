package freshness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureRoutes maps request paths onto responses recorded from the real
// sources (trimmed to the fields this package reads).
var fixtureRoutes = map[string]string{
	"/-/package/react/dist-tags":                 "npm-react.json",
	"/github.com/spf13/cobra/@latest":            "go-cobra.json",
	"/pypi/django/json":                          "pypi-django.json",
	"/api/v1/crates/serde":                       "crates-serde.json",
	"/dl/":                                       "go-dl.json",
	"/dist/index.json":                           "node-index.json",
	"/api/python.json":                           "python-eol.json",
	"/v2/repositories/library/postgres/tags":     "docker-postgres.json",
	"/github.com/!burnt!sushi/toml/@latest":      "go-cobra.json",
	"/-/package/@scope%2fpkg/dist-tags":          "npm-react.json",
	"/github.com/example/private-thing/@latest":  "go-cobra.json",
	"/github.com/example/also-private/x/@latest": "go-cobra.json",
}

type fixtureServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func (f *fixtureServer) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func newFixtureServer(t *testing.T) *fixtureServer {
	t.Helper()
	f := &fixtureServer{hits: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits[r.URL.EscapedPath()]++
		f.mu.Unlock()
		name, ok := fixtureRoutes[r.URL.EscapedPath()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile(filepath.Join("testdata", "registry", name))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(f.Close)
	return f
}

// newTestClient points every source at srv and keeps the user's real go env
// file and environment out of the test.
func newTestClient(t *testing.T, base string, env map[string]string) *Client {
	t.Helper()
	return &Client{
		HTTP:      &http.Client{Timeout: 2 * time.Second},
		Endpoints: Endpoints{GoProxy: base, NPM: base, PyPI: base, Crates: base, DockerHub: base, GoDL: base, NodeDist: base, PythonEOL: base},
		CacheDir:  t.TempDir(), TTL: time.Hour, Concurrency: 2,
		Env: func(k string) string { return env[k] }, GoEnvFile: "off",
	}
}

func TestLookup_EverySourceReturnsLatestStable(t *testing.T) {
	srv := newFixtureServer(t)
	c := newTestClient(t, srv.URL, nil)
	for _, tc := range []struct {
		item      Item
		want, lts string
	}{
		{Item{Ecosystem: EcoNPM, Name: "react"}, "19.3.1", ""},
		{Item{Ecosystem: EcoGo, Name: "github.com/spf13/cobra"}, "v1.10.2", ""},
		{Item{Ecosystem: EcoPyPI, Name: "django"}, "6.1.3", ""},
		{Item{Ecosystem: EcoCrates, Name: "serde"}, "1.0.229", ""},
		{Item{Ecosystem: EcoToolchain, Name: "go"}, "1.27.4", ""},
		{Item{Ecosystem: EcoToolchain, Name: "node"}, "26.13.0", "26.13.0"},
		{Item{Ecosystem: EcoToolchain, Name: "python"}, "3.15.2", ""},
		{Item{Ecosystem: EcoImage, Name: "postgres"}, "18.6", ""},
		{Item{Ecosystem: EcoImage, Name: "node"}, "26.13.0", "26.13.0"},
	} {
		got := c.Lookup(context.Background(), tc.item)
		assert.Empty(t, got.Err, tc.item.Key())
		assert.Equal(t, tc.want, got.Version, tc.item.Key())
		assert.Equal(t, tc.lts, got.LTS, tc.item.Key())
		assert.Contains(t, got.Source, srv.URL, tc.item.Key())
		assert.False(t, got.FetchedAt.IsZero(), tc.item.Key())
	}
}

// Private module paths never reach the proxy, whichever variable names them.
func TestLookup_GoPrivateModulesAreNeverSent(t *testing.T) {
	srv := newFixtureServer(t)
	c := newTestClient(t, srv.URL, map[string]string{"GOPRIVATE": "github.com/example/private-*", "GONOSUMDB": "github.com/example/also-private"})
	for _, path := range []string{"github.com/example/private-thing", "github.com/example/also-private/x"} {
		got := c.Lookup(context.Background(), Item{Ecosystem: EcoGo, Name: path})
		assert.Contains(t, got.Err, "private module")
		assert.Zero(t, srv.count("/"+path+"/@latest"), "a private path must not be sent")
	}
	got := c.Lookup(context.Background(), Item{Ecosystem: EcoGo, Name: "github.com/BurntSushi/toml"})
	assert.Empty(t, got.Err, "a public module is still looked up, case-escaped")
}

func TestLookup_GoproxyOffDirectAndCustom(t *testing.T) {
	srv := newFixtureServer(t)
	for setting, want := range map[string]string{"off": "GOPROXY=off", "direct": "nothing to ask"} {
		c := newTestClient(t, srv.URL, map[string]string{"GOPROXY": setting})
		assert.Contains(t, c.Lookup(context.Background(), Item{Ecosystem: EcoGo, Name: "github.com/spf13/cobra"}).Err, want)
	}
	c := newTestClient(t, "http://127.0.0.1:1", map[string]string{"GOPROXY": srv.URL + ",direct"})
	assert.Equal(t, "v1.10.2", c.Lookup(context.Background(), Item{Ecosystem: EcoGo, Name: "github.com/spf13/cobra"}).Version)
}

func TestLookup_GoEnvFileIsHonoured(t *testing.T) {
	srv := newFixtureServer(t)
	file := filepath.Join(t.TempDir(), "env")
	require.NoError(t, os.WriteFile(file, []byte("GOPRIVATE=github.com/example\n"), 0o600))
	c := newTestClient(t, srv.URL, nil)
	c.GoEnvFile = file
	assert.Contains(t, c.Lookup(context.Background(), Item{Ecosystem: EcoGo, Name: "github.com/example/private-thing"}).Err, "GOPRIVATE")
}

func TestLookup_CacheOfflineAndUnreachable(t *testing.T) {
	srv := newFixtureServer(t)
	c := newTestClient(t, srv.URL, nil)
	react := Item{Ecosystem: EcoNPM, Name: "react"}
	c.Lookup(context.Background(), react)
	c.Lookup(context.Background(), react)
	assert.Equal(t, 1, srv.count("/-/package/react/dist-tags"), "the second answer comes from the cache")

	c.Offline = true
	assert.Equal(t, "19.3.1", c.Lookup(context.Background(), react).Version, "offline still reads the cache")
	assert.Contains(t, c.Lookup(context.Background(), Item{Ecosystem: EcoNPM, Name: "vue"}).Err, "offline")

	dead := newTestClient(t, "http://127.0.0.1:1", nil)
	got := dead.Lookup(context.Background(), react)
	assert.NotEmpty(t, got.Err)
	assert.Equal(t, StatusUnknown, Classify(Item{Ecosystem: EcoNPM, Name: "react", Spec: "^16.0.0"}, got).Status)
	assert.Contains(t, c.Lookup(context.Background(), Item{Ecosystem: EcoPyPI, Name: "missing"}).Err, "offline")
}

func TestLookup_PrivatePythonIndexIsNotBypassed(t *testing.T) {
	srv := newFixtureServer(t)
	c := newTestClient(t, srv.URL, map[string]string{"PIP_INDEX_URL": "https://pkgs.internal.example/simple"})
	assert.Contains(t, c.Lookup(context.Background(), Item{Ecosystem: EcoPyPI, Name: "django"}).Err, "private index")
	assert.Zero(t, srv.count("/pypi/django/json"))
}

// A deadline turns the answers into unknown instead of hanging.
func TestLookupAll_DeadlineAndDedupe(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	c := newTestClient(t, slow.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	got := c.LookupAll(ctx, []Item{{Ecosystem: EcoNPM, Name: "a"}, {Ecosystem: EcoNPM, Name: "b"}, {Ecosystem: EcoNPM, Name: "c"}})
	assert.Less(t, time.Since(started), 2*time.Second)
	for _, l := range got {
		assert.NotEmpty(t, l.Err)
	}

	srv := newFixtureServer(t)
	c = newTestClient(t, srv.URL, nil)
	both := c.LookupAll(context.Background(), []Item{{Ecosystem: EcoNPM, Name: "react", File: "a"}, {Ecosystem: EcoNPM, Name: "react", File: "b"}})
	assert.Equal(t, both[0], both[1])
	assert.Equal(t, 1, srv.count("/-/package/react/dist-tags"))
}

func TestLookup_ScopedNpmUsesItsRegistry(t *testing.T) {
	srv := newFixtureServer(t)
	c := newTestClient(t, "http://127.0.0.1:1", nil)
	got := c.Lookup(context.Background(), Item{Ecosystem: EcoNPM, Name: "@scope/pkg", Registry: srv.URL})
	assert.Equal(t, "19.3.1", got.Version)
}
