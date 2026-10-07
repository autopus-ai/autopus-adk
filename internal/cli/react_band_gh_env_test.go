package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// envValue returns the value of key in env and whether key is set.
func envValue(env []string, key string) (string, bool) {
	for _, entry := range env {
		if name, value, _ := strings.Cut(entry, "="); name == key {
			return value, true
		}
	}
	return "", false
}

// Security M1: gh auth status runs without an injected GH_HOST, so gh answers
// only for a host in its own hosts config and never sends an environment
// token to whatever host origin names. An inherited GH_HOST survives only
// when it already names that host. Only after the check passes do the later
// calls get GH_HOST.
func TestReactBandGH_AuthStatusRunsWithoutAnInjectedHost(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		inherited string
		wantAuth  string // GH_HOST of the auth status call; empty means unset
	}{
		{"stale inherited host is dropped", "stale.example.com", ""},
		{"inherited host equal to origin is kept", "ghe.example.com", "ghe.example.com"},
		{"no inherited host", "", ""},
	} {
		runner := scriptedBandRunner("git@ghe.example.com:acme/app.git", "main", "[]")
		client := newBandGHClient(runner)
		client.environ = func() []string {
			env := []string{"PATH=/usr/bin", "GH_ENTERPRISE_TOKEN=synthetic"}
			if tc.inherited != "" {
				env = append(env, "GH_HOST="+tc.inherited)
			}
			return env
		}
		fetch, err := client.fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
		require.NoError(t, err, tc.name)
		require.Empty(t, fetch.Reason, tc.name)
		calls := runner.recorded("gh")
		require.Len(t, calls, 3, tc.name)
		require.Equal(t, "gh auth status --hostname ghe.example.com", strings.Join(calls[0].argv, " "), tc.name)
		host, set := envValue(calls[0].env, "GH_HOST")
		assert.Equal(t, tc.wantAuth, host, tc.name)
		assert.Equal(t, tc.wantAuth != "", set, tc.name)
		for _, call := range calls[1:] {
			host, _ := envValue(call.env, "GH_HOST")
			assert.Equal(t, "ghe.example.com", host, "%s: %v", tc.name, call.argv)
			assert.Equal(t, 1, strings.Count(strings.Join(call.env, "\n"), "GH_HOST="), tc.name)
		}
	}
}

// Security M1: an origin on localhost or an IP literal is never a GitHub
// host, so band runs no gh call that could carry a token to it.
func TestReactBandGH_LocalAndIPLiteralOriginsAreNotGitHub(t *testing.T) {
	t.Parallel()
	for _, origin := range []string{
		"https://127.0.0.1/acme/app.git", "git@10.0.0.7:acme/app.git", "ssh://git@169.254.169.254/acme/app.git",
		"https://localhost/acme/app.git", "git@localhost:acme/app.git", "https://api.localhost/acme/app.git",
		"http://2130706433/acme/app.git", "https://0x7f000001/acme/app.git", "https://[::1]/acme/app.git",
	} {
		_, ok := parseBandOrigin(origin)
		assert.False(t, ok, origin)
		runner := scriptedBandRunner(origin, "main", "[]")
		fetch, err := testBandClient(runner).fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
		require.NoError(t, err, origin)
		assert.Equal(t, healthband.ReasonRemoteNotGitHub, fetch.Reason, origin)
		assert.Empty(t, runner.recorded("gh"), origin)
	}
	target, ok := parseBandOrigin("git@ghe.example.com:acme/app.git")
	assert.True(t, ok)
	assert.Equal(t, "ghe.example.com", target.Host)
}

// Security L5: gh never prompts, pages, or colors, whatever the inherited
// environment says, so its output stays plain text for a pipe.
func TestReactBandGH_EnvironmentDisablesPromptsPagerAndColor(t *testing.T) {
	t.Parallel()
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
	client := newBandGHClient(runner)
	client.environ = func() []string {
		return []string{"PATH=/usr/bin", "GH_FORCE_TTY=100", "CLICOLOR_FORCE=1", "GH_PAGER=less", "GH_PROMPT_DISABLED=", "NO_COLOR="}
	}
	_, err := client.fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
	require.NoError(t, err)
	calls := runner.recorded("gh")
	require.Len(t, calls, 3)
	for _, call := range calls {
		for key, want := range map[string]string{"GH_PROMPT_DISABLED": "1", "GH_PAGER": "cat", "NO_COLOR": "1", "GH_REPO": "acme/app"} {
			got, _ := envValue(call.env, key)
			assert.Equal(t, want, got, "%s in %v", key, call.argv)
			assert.Equal(t, 1, strings.Count("\n"+strings.Join(call.env, "\n"), "\n"+key+"="), "%s once", key)
		}
		for _, key := range []string{"GH_FORCE_TTY", "CLICOLOR_FORCE"} {
			_, set := envValue(call.env, key)
			assert.False(t, set, "%s in %v", key, call.argv)
		}
	}
}
