package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// withDiskForeignUID makes every disk command run as a uid that owns none
// of the fixture, for the rest of the test.
func withDiskForeignUID(t *testing.T) int {
	t.Helper()
	uid := os.Getuid() + 1
	previous := diskUID
	diskUID = func() int { return uid }
	t.Cleanup(func() { diskUID = previous })
	return uid
}

// diskRefusedEverywhere runs a dry run and an apply, text and JSON, and
// asserts each is refused with message while both roots stay as they were.
func diskRefusedEverywhere(t *testing.T, f diskS5, project, message string) diskTrimData {
	t.Helper()
	before := diskTree(t, f.cache, f.work)
	for _, args := range [][]string{{"--ci", "idle"}, {"--ci", "idle", "--apply"}} {
		run := runDiskCLI(t, append([]string{"disk", "trim", "--dir", project}, args...)...)
		assert.Equal(t, 1, run.code, args)
		assert.Equal(t, "disk trim: refused: "+message+"\n", run.stderr, args)
		assert.Empty(t, run.stdout, args)
	}
	jsonRun := runDiskCLI(t, "disk", "trim", "--dir", project, "--ci", "idle", "--apply", "--json")
	envelope, data := decodeDiskTrim(t, jsonRun.stdout)
	assert.Equal(t, 1, jsonRun.code)
	assert.Equal(t, "disk_trim_refused", envelope.Error.Code)
	require.NotNil(t, data.Refused)
	// The shared envelope masks the home directory as ~ in every string.
	assert.Equal(t, maskHomePath(message), data.Refused.Message)
	assert.Equal(t, before, diskTree(t, f.cache, f.work))
	return data
}

// REQ-DISK-TRIM-06/10: an unacceptable root refuses both runs with one line
// and deletes nothing from either root, and a refused root is never walked;
// a missing root is only unavailable.
func TestDiskTrim_RefusesAndReportsUnavailableRoots(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, f diskS5) string
	}{
		{"no README", func(t *testing.T, f diskS5) string {
			require.NoError(t, os.Remove(filepath.Join(f.cache, "README")))
			return f.cache + " is not an acceptable cache root (no README marker)"
		}},
		{"work dir is home", func(t *testing.T, f diskS5) string {
			t.Setenv("HOME", f.work)
			return f.work + " is not an acceptable work dir root (home directory)"
		}},
		{"foreign uid", func(t *testing.T, f diskS5) string {
			return f.cache + " is not an acceptable cache root (not owned by uid " + strconv.Itoa(withDiskForeignUID(t)) + ")"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDiskS5(t, true)
			withDiskSeams(t, f)
			withDiskFree(t, gib(100), diskDataMount)
			data := diskRefusedEverywhere(t, f, writeDiskProject(t, ""), tc.setup(t, f))
			assert.Zero(t, data.Skipped.Owner, "a refused root is never walked, so nothing in it is counted")
		})
	}

	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	t.Setenv("GOCACHE", filepath.Join(f.base, "missing"))
	run := runDiskCLI(t, "disk", "trim", "--dir", writeDiskProject(t, ""), "--ci", "idle")
	assert.Equal(t, 0, run.code, run.stderr)
	assert.Contains(t, run.stdout, "  roots: cache=root_missing work="+f.work+"\n  go_build_cache: unavailable (root_missing)\n")
}

// REQ-DISK-TRIM-10: a sticky work dir root another uid owns is accepted
// only when that uid is 0. A sticky directory of any other owner, whose
// owner may rename entries in it, refuses the run and nothing is deleted;
// the uid-0 case is pinned read-only against the real /tmp in pkg/diskspace.
func TestDiskTrim_StickyWorkDirRootMustBeRootOwned(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	t.Setenv("GOCACHE", "off")
	require.NoError(t, os.Chmod(f.work, 0o777|os.ModeSticky))
	uid := withDiskForeignUID(t)

	diskRefusedEverywhere(t, f, writeDiskProject(t, ""),
		f.work+" is not an acceptable work dir root (not owned by uid "+strconv.Itoa(uid)+")")
}

// REQ-DISK-TRIM-06/10: a work dir root another uid owns without the sticky
// bit refuses the whole run, so the cache root, though acceptable and full
// of candidates, loses nothing either. /usr, root-owned and not sticky on
// darwin and linux, serves as that root and is never walked.
func TestDiskTrim_NonStickyForeignWorkDirRootRefusesBothRoots(t *testing.T) {
	info, err := os.Lstat("/usr")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSticky != 0 || os.Getuid() <= 0 {
		t.Skip("needs a root-owned, non-sticky /usr and a non-root uid")
	}
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	t.Setenv("GOTMPDIR", "/usr")
	project := writeDiskProject(t, "")
	before := diskTree(t, f.cache)
	want := "disk trim: refused: /usr is not an acceptable work dir root (not owned by uid " + strconv.Itoa(os.Getuid()) + ")\n"

	for _, args := range [][]string{{"--ci", "idle"}, {"--ci", "idle", "--apply"}} {
		run := runDiskCLI(t, append([]string{"disk", "trim", "--dir", project}, args...)...)
		assert.Equal(t, 1, run.code, args)
		assert.Equal(t, want, run.stderr, args)
		assert.Empty(t, run.stdout, args)
	}
	assert.Equal(t, before, diskTree(t, f.cache))
}

// REQ-DISK-TRIM-06: with $HOME empty the home directory comes from the user
// database and is still refused as a root; a run that knows neither refuses
// every root as home_unknown and deletes nothing.
func TestDiskTrim_HomeFallsBackToTheUserDatabase(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	t.Setenv("HOME", "")
	home := ""
	previous := diskUserHome
	diskUserHome = func() string { return home }
	t.Cleanup(func() { diskUserHome = previous })
	project := writeDiskProject(t, "")

	home = f.work
	diskRefusedEverywhere(t, f, project, f.work+" is not an acceptable work dir root (home directory)")

	home = ""
	diskRefusedEverywhere(t, f, project, f.cache+" is not an acceptable cache root (home directory unknown)")
}

// REQ-DISK-TRIM-04: the reviewer's repro. With $GOTMPDIR mis-set to the
// parent of a project named go-build2, the project, idle and holding
// main.go, is not a candidate and survives --apply as skipped_shape, while a
// sibling shaped as Go makes work dirs is still removed.
func TestDiskTrim_ProjectNamedLikeAWorkDirIsNeverACandidate(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	t.Setenv("GOCACHE", "off")
	project := filepath.Join(f.work, "go-build2")
	a := diskAged{t: t, now: f.now}
	a.file(filepath.Join(project, "main.go"), diskspace.KiB, 400*time.Minute)
	a.stamp(project, 400*time.Minute)

	run := runDiskCLI(t, "disk", "trim", "--dir", project, "--ci", "idle", "--apply", "--list")

	require.Equal(t, 0, run.code, run.stderr)
	assert.Contains(t, run.stdout, "  go_build_work_dirs: candidates=1 size=3.0 MiB\n    go-build111\n")
	assert.Contains(t, run.stdout, "  skipped_shape=1\n")
	assert.FileExists(t, filepath.Join(project, "main.go"))
	assert.NoDirExists(t, filepath.Join(f.work, "go-build111"))
}
