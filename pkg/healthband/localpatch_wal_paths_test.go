package healthband

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Acceptance fixture of SPEC-SIGMABAND-002: O3 at sample key 1042, episode
// e1042, and the injected claim id whose <c8> is a1b2c3d4.
const (
	lpSeries   = "ci.failure_rate:CI"
	lpEpisode  = "e1042"
	lpClaimID  = "a1b2c3d4e5f60708a1b2c3d4e5f60708"
	lpFixtureK = "ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4"
)

func TestLocalPatchKey_AcceptanceFixture_MatchesTheSpecKey(t *testing.T) {
	t.Parallel()
	assert.Equal(t, lpFixtureK, LocalPatchKey(lpSeries, lpEpisode, lpClaimID))
}

func TestLocalPatchKey_SlugRule_MapsEverySeriesAndEpisodeToTheKeyAlphabet(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, series, episode, wantPrefix string }{
		{"punctuation runs fold to one dash", "ci.failure_rate:Build + Test #2", "e1042", "ci-failure-rate-build-test-2-"},
		{"leading and trailing runs are trimmed", "canary.failure_rate:-local-", "e.c7_", "canary-failure-rate-local-"},
		{"hashed name keeps its suffix", "ci.failure_rate:#0a1b2c3d", "e9", "ci-failure-rate-0a1b2c3d-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := LocalPatchKey(tc.series, tc.episode, lpClaimID)
			assert.True(t, strings.HasPrefix(key, tc.wantPrefix), key)
			assert.Regexp(t, `^[a-z0-9-]+$`, key)
			assert.True(t, strings.HasSuffix(key, "-"+H8(tc.series)+"-"+localPatchSlug(tc.episode)+"-a1b2c3d4"), key)
		})
	}
	assert.Equal(t, "e-c7", localPatchSlug("e.c7_"))
}

func TestLocalPatchKey_LongSeries_CutsEachSlugTo40Bytes(t *testing.T) {
	t.Parallel()
	series := "ci.failure_rate:" + strings.Repeat("Workflow", 9)
	episode := "e" + strings.Repeat("9", 64)
	key := LocalPatchKey(series, episode, lpClaimID)
	parts := strings.Split(key, "-"+H8(series)+"-")
	assert.Len(t, parts, 2)
	assert.Len(t, parts[0], 40)
	assert.Equal(t, strings.Repeat("9", 39), strings.TrimSuffix(strings.TrimPrefix(parts[1], "e"), "-a1b2c3d4"))
}

func TestLocalPatchRepoHash_IsTheFirst12HexOfTheCommonDirHash(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("/repo/.git"))
	assert.Equal(t, hex.EncodeToString(sum[:])[:12], LocalPatchRepoHash("/repo/.git"))
}

func TestLocalPatchLocation_Paths_DeriveEveryArtifactFromTheKey(t *testing.T) {
	t.Parallel()
	loc := LocalPatchLocation{CacheDir: "/cache", RepoHash: "0123456789ab", Path: "/cache/autopus/local-patches/0123456789ab"}
	paths := loc.Paths(lpFixtureK)
	lp := loc.Path
	assert.Equal(t, LocalPatchPaths{
		Key:       lpFixtureK,
		KeyDir:    filepath.Join(lp, lpFixtureK),
		Worktree:  filepath.Join(lp, lpFixtureK, "worktree"),
		Patch:     filepath.Join(lp, lpFixtureK+".patch"),
		PatchTemp: filepath.Join(lp, lpFixtureK+".patch.tmp-a1b2c3d4"),
		Diff:      filepath.Join(lp, lpFixtureK+".diff"),
		Lock:      filepath.Join(lp, lpFixtureK+".lock"),
		Ref:       "refs/heads/autopus/band/" + lpFixtureK,
		Branch:    "autopus/band/" + lpFixtureK,
	}, paths)
	assert.Equal(t, filepath.Join("/cache/autopus/local-patches", "0123456789ab"), localPatchDirOf("/cache", "0123456789ab"))
}
