package cireport

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSince(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"14d":        now.Add(-14 * 24 * time.Hour),
		"36h":        now.Add(-36 * time.Hour),
		"2w":         now.Add(-14 * 24 * time.Hour),
		"2026-10-01": time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	} {
		got, err := ParseSince(in, now)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "0d", "-3d", "abc", "2026-13-01", "3000d"} {
		_, err := ParseSince(bad, now)
		assert.Error(t, err, bad)
	}
}

func TestParseGitHubRemote(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"git@github.com:owner/name.git":           "owner/name",
		"https://github.com/owner/name":           "owner/name",
		"https://github.com/owner/name.git":       "owner/name",
		"ssh://git@github.com/owner/name.git":     "owner/name",
		"https://token@github.com/owner/name.git": "owner/name",
	} {
		got, err := ParseGitHubRemote(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}
	for _, bad := range []string{"", "https://gitlab.com/o/n", "git@github.com:onlyowner", "/local/path"} {
		_, err := ParseGitHubRemote(bad)
		assert.Error(t, err, bad)
	}
	assert.True(t, ValidRepo("o-1/r.name_2"))
	assert.False(t, ValidRepo("o/r/x"))
	assert.False(t, ValidRepo("o/../r"))
}
