//go:build darwin || linux

package cireport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGH installs testdata/fake-gh.sh as gh in a PATH of its own and returns
// the response dir and the argv log.
func fakeGH(t *testing.T, authExit string) (string, string) {
	t.Helper()
	bin := t.TempDir()
	script, err := os.ReadFile(filepath.Join("testdata", "fake-gh.sh"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), script, 0o755))
	responses, log := t.TempDir(), filepath.Join(t.TempDir(), "argv.log")
	// The fake comes first; the system dirs supply the tools it calls.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
	t.Setenv("FAKE_GH_DIR", responses)
	t.Setenv("FAKE_GH_LOG", log)
	t.Setenv("FAKE_GH_AUTH_EXIT", authExit)
	return responses, log
}

func TestGH_GetCachesAndCountsRequests(t *testing.T) {
	responses, log := fakeGH(t, "0")
	require.NoError(t, os.WriteFile(filepath.Join(responses, "repos_o_r.json"), []byte(`{"private":false}`), 0o600))
	gh, err := NewGH(t.TempDir(), 5)
	require.NoError(t, err)
	require.NoError(t, gh.CheckAuth(context.Background()))

	for i := 0; i < 3; i++ {
		data, err := gh.Get(context.Background(), "repos/o/r")
		require.NoError(t, err)
		assert.JSONEq(t, `{"private":false}`, string(data))
	}
	assert.Equal(t, 1, gh.Requests(), "repeated paths come from the run cache")
	argv, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(argv), "api "), string(argv))

	_, err = gh.Get(context.Background(), "repos/o/missing")
	assert.ErrorContains(t, err, "HTTP 404")
}

func TestGH_BudgetIsEnforced(t *testing.T) {
	responses, _ := fakeGH(t, "0")
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.WriteFile(filepath.Join(responses, name+".json"), []byte(`{}`), 0o600))
	}
	gh, err := NewGH(t.TempDir(), 1)
	require.NoError(t, err)
	_, err = gh.Get(context.Background(), "a")
	require.NoError(t, err)
	_, err = gh.Get(context.Background(), "b")
	assert.True(t, errors.Is(err, ErrBudget), err)
}

func TestGH_MissingAndUnauthenticated(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := NewGH(t.TempDir(), 5)
	assert.ErrorContains(t, err, "gh CLI not found")

	fakeGH(t, "1")
	gh, err := NewGH(t.TempDir(), 5)
	require.NoError(t, err)
	err = gh.CheckAuth(context.Background())
	assert.ErrorContains(t, err, "gh auth login")
}
