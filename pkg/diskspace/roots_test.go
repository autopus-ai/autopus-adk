package diskspace

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// REQ-DISK-STATUS-02/TRIM-07: a set $GOCACHE wins, is canonicalized, and
// starts no go subprocess.
func TestLocatorCache_GOCACHEWinsWithoutAGoSubprocess(t *testing.T) {
	f := sharedS5(t)
	link := filepath.Join(canonicalTempDir(t), "cache-link")
	require.NoError(t, os.Symlink(f.cache, link))
	t.Setenv("GOCACHE", link)
	recorder := &execRecorder{}

	root := Locator{Run: recorder.run, Home: testHome, UID: os.Getuid()}.Cache(context.Background())

	assert.True(t, root.Available(), root.Message)
	assert.Equal(t, f.cache, root.Path)
	assert.NotZero(t, root.ID.Ino)
	assert.Empty(t, recorder.calls)
}

// S5: an unset $GOCACHE asks the go command exactly once, only `go env GOCACHE`.
func TestLocatorCache_UnsetAsksGoEnvOnce(t *testing.T) {
	f := sharedS5(t)
	t.Setenv("GOCACHE", "")
	recorder := &execRecorder{answers: map[string]string{"go env GOCACHE": f.cache + "\n"}}

	root := Locator{Run: recorder.run, Home: testHome, UID: os.Getuid()}.Cache(context.Background())

	assert.Equal(t, f.cache, root.Path)
	assert.True(t, root.Available(), root.Message)
	assert.Equal(t, []string{"go env GOCACHE"}, recorder.calls)
	assert.Equal(t, [][]string{{"GOTOOLCHAIN=local", "GOWORK=off"}}, recorder.envs,
		"a toolchain line in a go.mod or go.work above the temp dir must not switch toolchains")
}

// REQ-DISK-STATUS-02: without an answer from go, os.UserCacheDir()/go-build;
// a go answer of off is gocache_off.
func TestLocatorCache_FallbacksAndOff(t *testing.T) {
	t.Setenv("GOCACHE", "")
	t.Setenv("HOME", canonicalTempDir(t))
	t.Setenv("XDG_CACHE_HOME", "")
	userCache, err := os.UserCacheDir()
	require.NoError(t, err)
	want := filepath.Join(userCache, "go-build")

	root := Locator{Run: (&execRecorder{}).run, Home: testHome}.Cache(context.Background())
	assert.Equal(t, Root{Kind: KindCache, Path: want, Reason: ReasonRootMissing, Message: "cache root " + want + " does not exist"}, root)

	off := &execRecorder{answers: map[string]string{"go env GOCACHE": "off\n"}}
	assert.Equal(t, ReasonGOCACHEOff, Locator{Run: off.run, Home: testHome}.Cache(context.Background()).Reason)
	t.Setenv("GOCACHE", "off")
	assert.Equal(t, Root{Kind: KindCache, Reason: ReasonGOCACHEOff, Message: "GOCACHE=off"}, Locator{Run: off.run, Home: testHome}.Cache(context.Background()))
}

// REQ-DISK-TRIM-06/TRIM-10: every refusal names the root, its kind, and the
// reason, for both kinds of root.
func TestLocatorInspect_RefusesUnacceptableRoots(t *testing.T) {
	f := sharedS5(t)
	base := canonicalTempDir(t)
	noMarker := filepath.Join(base, "no-marker")
	linkedMarker := filepath.Join(base, "linked-marker")
	for _, dir := range []string{noMarker, linkedMarker} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	require.NoError(t, os.Symlink(filepath.Join(f.cache, "README"), filepath.Join(linkedMarker, "README")))
	file := filepath.Join(base, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	home, project := canonicalTempDir(t), canonicalTempDir(t)
	for _, dir := range []string{home, project} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o644))
	}
	uid := os.Getuid()
	l := Locator{Home: home, ProjectDir: project, UID: uid}
	foreign := Locator{Home: home, ProjectDir: project, UID: uid + 1}

	for _, tc := range []struct {
		l       Locator
		kind    string
		raw     string
		reason  string
		message string
	}{
		{l, KindCache, noMarker, ReasonNotAGoBuildCache, noMarker + " is not an acceptable cache root (no README marker)"},
		{l, KindCache, linkedMarker, ReasonNotAGoBuildCache, linkedMarker + " is not an acceptable cache root (no README marker)"},
		{l, KindCache, file, ReasonNotADirectory, file + " is not an acceptable cache root (not a directory)"},
		{l, KindCache, "/", ReasonFilesystemRoot, "/ is not an acceptable cache root (file system root)"},
		{l, KindWorkDir, home, ReasonHomeDirectory, home + " is not an acceptable work dir root (home directory)"},
		{l, KindCache, project, ReasonProjectDirectory, project + " is not an acceptable cache root (project directory)"},
		{l, KindWorkDir, "rel/tmp", ReasonRelativeRoot, "rel/tmp is not an acceptable work dir root (relative path)"},
		{foreign, KindCache, f.cache, ReasonNotOwned, f.cache + " is not an acceptable cache root (not owned by uid " + strconv.Itoa(uid+1) + ")"},
		{foreign, KindWorkDir, f.work, ReasonNotOwned, f.work + " is not an acceptable work dir root (not owned by uid " + strconv.Itoa(uid+1) + ")"},
	} {
		root := tc.l.inspect(tc.kind, tc.raw)
		assert.True(t, root.Refused, tc.raw)
		assert.Equal(t, tc.reason, root.Reason, tc.raw)
		assert.Equal(t, tc.message, root.Message, tc.raw)
	}
	accepted := l.inspect(KindWorkDir, noMarker)
	assert.True(t, accepted.Available(), "a work dir root needs no README marker")
	missing := filepath.Join(base, "missing")
	assert.Equal(t, Root{Kind: KindWorkDir, Path: missing, Reason: ReasonRootMissing, Message: "work dir root " + missing + " does not exist"},
		l.inspect(KindWorkDir, missing))
}

// REQ-DISK-STATUS-02 counterpart for work dirs: $GOTMPDIR first, then the
// temp directory, canonicalized.
func TestLocatorWorkDir_PrefersGOTMPDIR(t *testing.T) {
	dir := canonicalTempDir(t)
	link := filepath.Join(canonicalTempDir(t), "tmp-link")
	require.NoError(t, os.Symlink(dir, link))
	l := Locator{Home: testHome, UID: os.Getuid()}

	t.Setenv("GOTMPDIR", link)
	assert.Equal(t, dir, l.WorkDir().Path)
	assert.True(t, l.WorkDir().Available())

	t.Setenv("GOTMPDIR", "")
	t.Setenv("TMPDIR", link)
	assert.Equal(t, dir, l.WorkDir().Path)
}

// REQ-DISK-TRIM-10: only a sticky work dir root owned by uid 0 may belong
// to another uid. A sticky directory any other uid owns stays refused, since
// its owner may rename entries in it; the real /tmp, root-owned and sticky,
// is only read.
func TestLocatorInspect_StickyWorkDirRootMustBeRootOwned(t *testing.T) {
	t.Parallel()
	sticky := canonicalTempDir(t)
	require.NoError(t, os.Chmod(sticky, 0o777|os.ModeSticky))
	info, err := os.Lstat(sticky)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSticky, "the fixture root must carry the sticky bit")
	uid := os.Getuid()

	refused := Locator{Home: testHome, UID: uid + 1}.inspect(KindWorkDir, sticky)
	assert.Equal(t, []any{true, ReasonNotOwned}, []any{refused.Refused, refused.Reason})
	assert.True(t, Locator{Home: testHome, UID: uid}.inspect(KindWorkDir, sticky).Available(), "an own sticky root is plainly accepted")

	tmp, err := filepath.EvalSymlinks("/tmp")
	require.NoError(t, err)
	tmpInfo, err := os.Lstat(tmp)
	require.NoError(t, err)
	if statOf(tmpInfo).uid != 0 || tmpInfo.Mode()&os.ModeSticky == 0 || uid == 0 {
		t.Skip("/tmp is not a root-owned sticky directory, or the test runs as root")
	}
	assert.True(t, Locator{Home: testHome, UID: uid}.inspect(KindWorkDir, "/tmp").Available())
	assert.Equal(t, ReasonNotAGoBuildCache, Locator{Home: testHome, UID: uid}.inspect(KindCache, "/tmp").Reason)
}

// REQ-DISK-TRIM-06: without a known home directory every existing root is
// refused, and a cache root's README marker is judged before its owner.
func TestLocatorInspect_HomeUnknownAndREADMEBeforeOwner(t *testing.T) {
	t.Parallel()
	f := sharedS5(t)
	for _, kind := range []string{KindCache, KindWorkDir} {
		root := Locator{UID: os.Getuid()}.inspect(kind, f.work)
		assert.Equal(t, []any{true, ReasonHomeUnknown, f.work + " is not an acceptable " + kind + " root (home directory unknown)"},
			[]any{root.Refused, root.Reason, root.Message}, kind)
	}
	foreignNoREADME := Locator{Home: testHome, UID: os.Getuid() + 1}.inspect(KindCache, f.work)
	assert.Equal(t, ReasonNotAGoBuildCache, foreignNoREADME.Reason)
}
