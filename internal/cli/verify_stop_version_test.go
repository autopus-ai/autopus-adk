package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/freshness"
)

// No test in this package may reach a real registry: by default every
// lookup is offline with an empty cache, so it answers "unknown".
func init() {
	newFreshnessClient = func() *freshness.Client {
		c := freshness.NewClient()
		c.Offline, c.CacheDir, c.GoEnvFile = true, "", "off"
		c.Env = func(string) string { return "" }
		return c
	}
}

// registryResponses are trimmed recordings of the Docker Hub tag list and
// the npm dist-tags document.
var registryResponses = map[string]string{
	"/v2/repositories/library/postgres/tags": `{"results":[{"name":"18.6"},{"name":"18"},{"name":"19rc1"},{"name":"17"},{"name":"13"}]}`,
	"/-/package/react/dist-tags":             `{"latest":"19.3.1","next":"19.3.0-canary-d5736f09-20260507"}`,
}

// useFixtureRegistry points the gate at an httptest registry and returns
// the number of requests it served.
func useFixtureRegistry(t *testing.T, delay time.Duration) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if delay > 0 {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(delay):
			}
		}
		body, ok := registryResponses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	cache := t.TempDir()
	prev := newFreshnessClient
	newFreshnessClient = func() *freshness.Client {
		c := prev()
		c.Offline, c.CacheDir = false, cache
		c.Endpoints = freshness.Endpoints{GoProxy: srv.URL, NPM: srv.URL, PyPI: srv.URL, Crates: srv.URL, DockerHub: srv.URL,
			GoDL: srv.URL, NodeDist: srv.URL, PythonEOL: srv.URL}
		return c
	}
	t.Cleanup(func() { newFreshnessClient = prev })
	return &hits
}

// newManifestRepo commits base, then writes edits without committing them.
func newManifestRepo(t *testing.T, base, edits map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	for rel, body := range base {
		write(rel, body)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "base", "--allow-empty"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	for rel, body := range edits {
		write(rel, body)
	}
	return dir
}

func freshSession(t *testing.T) string {
	session := "version-" + strings.ReplaceAll(t.Name(), "/", "_")
	resetStopState(session) // a crashed earlier run may have left state behind
	t.Cleanup(func() { resetStopState(session) })
	return session
}

const oldCompose = "services:\n  db:\n    image: postgres:13\n"

// A newly added image a whole major line behind blocks once, naming item,
// chosen version, latest stable and the way out; the same set then ends
// the session with a warning instead of a loop.
func TestVerifyStop_VersionBlocksANewOldImageOnce(t *testing.T) {
	hits := useFixtureRegistry(t, 0)
	session := freshSession(t)
	dir := newManifestRepo(t, nil, map[string]string{"docker-compose.yml": oldCompose})

	got := runStopReply(t, dir, session, "done")
	assert.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "docker-compose.yml:3 image postgres:13: latest stable 18.6 (5 release line(s) behind")
	assert.Contains(t, got.Reason, "version-pin: <name> <reason>")
	assert.Positive(t, hits.Load(), "the verdict must come from the registry, not a default")

	again := runStopReply(t, dir, session, "done")
	assert.Empty(t, again.Decision, "the same pins block only once")
	assert.Contains(t, again.SystemMessage, "postgres:13")
}

// Pre-existing old pins never nag; only what this turn adds or bumps counts.
func TestVerifyStop_VersionIgnoresUntouchedOldPins(t *testing.T) {
	useFixtureRegistry(t, 0)
	session := freshSession(t)
	base := map[string]string{"docker-compose.yml": oldCompose, "package.json": `{"dependencies":{"react":"^16.0.0"}}`}
	edits := map[string]string{
		"docker-compose.yml": "services:\n  api:\n    image: postgres:18\n  db:\n    image: postgres:13\n  web:\n    restart: always\n",
		"package.json":       "{\n  \"name\": \"x\",\n  \"dependencies\": {\n    \"react\": \"^16.0.0\"\n  }\n}\n",
	}
	assert.Equal(t, stopHookOutput{}, runStopReply(t, newManifestRepo(t, base, edits), session, "done"))

	bumped := newManifestRepo(t, base, map[string]string{"package.json": `{"dependencies":{"react":"^17.0.0"}}`})
	got := runStopReply(t, bumped, freshSession(t)+"-bump", "done")
	assert.Equal(t, "block", got.Decision, "a bump that stays behind is a new choice")
	assert.Contains(t, got.Reason, "npm:react ^17.0.0: latest stable 19.3.1")
	resetStopState(freshSession(t) + "-bump")
}

func TestVerifyStop_VersionPinLineCommentAndOptOuts(t *testing.T) {
	useFixtureRegistry(t, 0)
	pinned := "services:\n  db:\n    image: postgres:13 # version-pin: matches the managed fleet\n"
	off := false
	for name, row := range map[string]struct {
		files map[string]string
		reply string
		setup func(t *testing.T, dir string)
	}{
		"reply line": {map[string]string{"docker-compose.yml": oldCompose}, "done\nversion-pin: postgres matches the managed fleet", nil},
		"comment":    {map[string]string{"docker-compose.yml": pinned}, "done", nil},
		"env":        {map[string]string{"docker-compose.yml": oldCompose}, "done", func(t *testing.T, _ string) { t.Setenv("AUTOPUS_VERSION_STOP", "off") }},
		"config": {map[string]string{"docker-compose.yml": oldCompose}, "done", func(t *testing.T, dir string) {
			cfg := config.DefaultFullConfig("demo")
			cfg.Hooks.VersionStop = &config.VersionStopConf{Enabled: &off}
			require.NoError(t, config.Save(dir, cfg))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			session := freshSession(t)
			dir := newManifestRepo(t, nil, row.files)
			if row.setup != nil {
				row.setup(t, dir)
			}
			assert.Equal(t, stopHookOutput{}, runStopReply(t, dir, session, row.reply))
		})
	}
}

// A threshold above the gap turns the block into a warning.
func TestVerifyStop_VersionThresholdWarnsInsteadOfBlocking(t *testing.T) {
	useFixtureRegistry(t, 0)
	dir := newManifestRepo(t, nil, map[string]string{"docker-compose.yml": oldCompose})
	five := 5
	cfg := config.DefaultFullConfig("demo")
	cfg.Hooks.VersionStop = &config.VersionStopConf{MaxMajorBehind: &five}
	require.NoError(t, config.Save(dir, cfg))
	got := runStopReply(t, dir, freshSession(t), "done")
	assert.Empty(t, got.Decision)
	assert.Contains(t, got.SystemMessage, "behind the latest stable")
}

// Offline, unreachable or slow registries never block, and the stop stays
// within the lookup deadline.
func TestVerifyStop_VersionLookupFailureOnlyWarns(t *testing.T) {
	session := freshSession(t)
	dir := newManifestRepo(t, nil, map[string]string{"docker-compose.yml": oldCompose})
	got := runStopReply(t, dir, session, "done")
	assert.Empty(t, got.Decision, "the default test client is offline")
	assert.Contains(t, got.SystemMessage, "could not check the latest version of image postgres:13")
	again := runStopReply(t, dir, session, "done")
	assert.NotContains(t, again.SystemMessage, "could not check", "the same warning is not repeated every stop")

	useFixtureRegistry(t, versionStopDeadline+5*time.Second)
	started := time.Now()
	slow := runStopReply(t, newManifestRepo(t, nil, map[string]string{"docker-compose.yml": oldCompose}), freshSession(t)+"-slow", "done")
	resetStopState(freshSession(t) + "-slow")
	assert.Empty(t, slow.Decision)
	assert.Less(t, time.Since(started), versionStopDeadline+4*time.Second)
}

// The block limit bounds the version gate like every other gate.
func TestVerifyStop_VersionRespectsTheBlockLimit(t *testing.T) {
	useFixtureRegistry(t, 0)
	session := freshSession(t)
	for i := 0; i < verifyStopMaxBlocks; i++ {
		countStopBlock(session)
	}
	dir := newManifestRepo(t, nil, map[string]string{"docker-compose.yml": oldCompose})
	got := runStopReply(t, dir, session, "done")
	assert.Empty(t, got.Decision)
	assert.Contains(t, got.SystemMessage, "still behind")
}
