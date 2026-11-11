package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

const reclaimUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

// reclaimFixture is a repository with one merged harness worktree, a linked
// worktree elsewhere, a plain directory, and a scratchpad root holding one
// idle session, all under a fresh temp dir, with every disk seam pointed at
// it and the clock 2000 minutes ahead.
type reclaimFixture struct {
	repo, harness, elsewhere, plain, scratch, session string
	now                                               time.Time
}

func newReclaimFixture(t *testing.T) reclaimFixture {
	t.Helper()
	f := newDiskS5(t, true)
	recorder := withDiskSeams(t, f)
	recorder.goCache = f.cache
	base := f.base
	rf := reclaimFixture{repo: filepath.Join(base, "rrepo"), plain: filepath.Join(base, "plain"),
		scratch: filepath.Join(base, "rscratch"), now: time.Now().Add(2000 * time.Minute)}
	rf.harness = filepath.Join(rf.repo, ".worktrees", "done")
	rf.elsewhere = filepath.Join(base, "elsewhere")
	rf.session = filepath.Join(rf.scratch, "-proj", reclaimUUID)
	require.NoError(t, os.MkdirAll(rf.repo, 0o755))
	require.NoError(t, os.MkdirAll(rf.plain, 0o755))
	git := func(dir string, args ...string) {
		_, err := recorder.run(t.Context(), dir, nil, "git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t.test"}, args...)...)
		require.NoError(t, err, "git %v", args)
	}
	git(rf.repo, "init", "--initial-branch=main")
	require.NoError(t, os.WriteFile(filepath.Join(rf.repo, "f.txt"), []byte("seed\n"), 0o644))
	git(rf.repo, "add", ".")
	git(rf.repo, "commit", "-m", "seed")
	git(rf.repo, "worktree", "add", "-b", "done", rf.harness, "main")
	git(rf.repo, "worktree", "add", "-b", "other", rf.elsewhere, "main")
	require.NoError(t, os.MkdirAll(filepath.Join(rf.session, "scratchpad"), 0o700))
	require.NoError(t, os.Chmod(rf.scratch, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(rf.session, "scratchpad", "out.bin"), make([]byte, 4096), 0o600))
	diskNow = func() time.Time { return rf.now }
	diskScratchpad = func(int) string { return rf.scratch }
	prevGetwd := diskGetwd
	diskGetwd = func() (string, error) { return base, nil }
	t.Cleanup(func() { diskGetwd = prevGetwd })
	withDiskFree(t, gib(50), "/fixture")
	return rf
}

// SPEC-DISK-002 S3 through the commands.
func TestDiskRegister_ReleaseRegistry(t *testing.T) {
	rf := newReclaimFixture(t)

	out := runDiskCLI(t, "disk", "register", rf.elsewhere, "--owner", "agent-1")
	require.Equal(t, 0, out.code, out.stderr)
	assert.Equal(t, "disk register: "+rf.elsewhere+"\n", out.stdout)

	bad := runDiskCLI(t, "disk", "register", rf.plain)
	assert.Equal(t, 1, bad.code)
	assert.Equal(t, "disk register: "+rf.plain+" is not the top of a linked git worktree\n", bad.stderr)

	list := runDiskCLI(t, "disk", "registry", "--json")
	require.Equal(t, 0, list.code, list.stderr)
	var env struct {
		Data struct {
			Entries []diskRegistryItem `json:"entries"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(list.stdout), &env))
	require.Len(t, env.Data.Entries, 1)
	assert.Equal(t, "live", env.Data.Entries[0].State)
	assert.Equal(t, "agent-1", env.Data.Entries[0].Owner)

	rel := runDiskCLI(t, "disk", "release", rf.elsewhere)
	require.Equal(t, 0, rel.code, rel.stderr)
	assert.Equal(t, "disk release: "+rf.elsewhere+"\n", rel.stdout)
	none := runDiskCLI(t, "disk", "release", rf.plain)
	assert.Equal(t, 0, none.code)
	assert.Equal(t, "disk release: "+rf.plain+" is not registered\n", none.stdout)
	text := runDiskCLI(t, "disk", "registry")
	assert.Contains(t, text.stdout, "released ")

	ledger := filepath.Join(diskStatePath(), "registry.json")
	require.NoError(t, os.WriteFile(ledger, []byte("{"), 0o600))
	corrupt := runDiskCLI(t, "disk", "register", rf.elsewhere)
	assert.Equal(t, 1, corrupt.code)
	assert.True(t, strings.HasPrefix(corrupt.stderr, "disk: ledger "+ledger+" is unreadable: "), corrupt.stderr)
	data, err := os.ReadFile(ledger)
	require.NoError(t, err)
	assert.Equal(t, "{", string(data))
}

// SPEC-DISK-002 S4/S5 through the command: the dry run names the harness
// worktree and the idle session and keeps the unregistered worktree; the
// apply removes both, prints the summary, and logs one event.
func TestDiskReclaim_DryRunThenApply(t *testing.T) {
	rf := newReclaimFixture(t)

	dry := runDiskCLI(t, "disk", "reclaim", "--dir", rf.repo)
	require.Equal(t, 0, dry.code, dry.stderr)
	assert.Contains(t, dry.stdout, "  worktree "+rf.harness+" reason=merged_idle")
	assert.Contains(t, dry.stdout, "  scratchpad "+rf.session+" reason=session_idle")
	assert.Contains(t, dry.stdout, "  kept worktree: not_owned=1\n")
	assert.Contains(t, dry.stdout, "disk reclaim: would reclaim ")
	assert.DirExists(t, rf.harness)

	applied := runDiskCLI(t, "disk", "reclaim", "--dir", rf.repo, "--apply")
	require.Equal(t, 0, applied.code, applied.stderr)
	assert.Contains(t, applied.stdout, "(worktrees=1 scratchpads=1 regenerable=0 large_files=0 skipped_changed=0 skipped_symlink=0 skipped_device=0 vanished=0 failed=0)\n")
	assert.NoDirExists(t, rf.harness)
	assert.NoDirExists(t, rf.session)
	assert.DirExists(t, rf.elsewhere)

	log := runDiskCLI(t, "disk", "log", "--json")
	require.Equal(t, 0, log.code, log.stderr)
	assert.Contains(t, log.stdout, `"kind": "reclaim_apply"`)
	assert.Contains(t, log.stdout, `"count": 2`)
}

// SPEC-DISK-002 S5: a group-writable scratchpad root deletes nothing and
// fails the apply, while the dry run still reports it.
func TestDiskReclaim_RefusedScratchRoot(t *testing.T) {
	rf := newReclaimFixture(t)
	require.NoError(t, os.Chmod(rf.scratch, 0o770))

	dry := runDiskCLI(t, "disk", "reclaim", "--dir", rf.repo)
	assert.Equal(t, 0, dry.code)
	assert.Contains(t, dry.stdout, "  scratchpad root: unavailable (not_owned)\n")

	applied := runDiskCLI(t, "disk", "reclaim", "--dir", rf.repo, "--apply")
	assert.Equal(t, 1, applied.code)
	assert.Contains(t, applied.stderr, "disk reclaim: refused: "+rf.scratch+" is not an acceptable scratchpad root (writable by group or others)")
	assert.DirExists(t, rf.session)
}

// SPEC-DISK-002 S7: status carries the reclaim preview, the leases, owned
// worktrees, and the trend once a sample is old enough.
func TestDiskStatus_SPEC002Sections(t *testing.T) {
	rf := newReclaimFixture(t)
	state, err := openDiskState()
	require.NoError(t, err)
	require.NoError(t, diskspace.RecordSample(state, diskspace.Sample{Time: rf.now.Add(-2 * time.Hour), FreeBytes: gib(70), Mount: "/fixture"}))
	lease, err := diskspace.Admit(t.Context(), state, diskspace.AdmitRequest{Free: func() (int64, error) { return gib(50), nil },
		FloorBytes: gib(10), NeedBytes: gib(5), Label: "build", PID: os.Getpid(), Now: rf.now})
	require.NoError(t, err)
	defer lease.Lease.Release()

	out := runDiskCLI(t, "disk", "status", "--dir", rf.repo, "--ci", "idle")
	require.Equal(t, 0, out.code, out.stderr)
	assert.Contains(t, out.stdout, "disk reclaim: worktrees=1 ")
	assert.Contains(t, out.stdout, "  owned worktree: "+rf.harness+"\n")
	assert.Contains(t, out.stdout, "disk lease: 5.0 GiB for build (pid ")
	assert.Contains(t, out.stdout, "free trend: -10.0 GiB/h over 2h0m0s\n")

	js := runDiskCLI(t, "disk", "status", "--dir", rf.repo, "--ci", "idle", "--json")
	require.Equal(t, 0, js.code, js.stderr)
	var env struct {
		Data diskStatusData `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(js.stdout), &env))
	require.NotNil(t, env.Data.Trend)
	assert.Equal(t, int64(120), env.Data.Trend.WindowMinutes)
	assert.Len(t, env.Data.Leases, 1)
	assert.Len(t, env.Data.ReclaimPreview.Worktrees.Candidates, 1)
}

// SPEC-DISK-002 review F-10/F-15: an unusable state dir never changes the
// outcome of reclaim, check, or trim; only ledger commands need it.
func TestDiskCommands_UnusableStateDirOnlyFailsLedgerCommands(t *testing.T) {
	rf := newReclaimFixture(t)
	elsewhere := t.TempDir()
	link := filepath.Join(t.TempDir(), "state-link")
	require.NoError(t, os.Symlink(elsewhere, link))
	diskStatePath = func() string { return filepath.Join(link, "disk") }

	applied := runDiskCLI(t, "disk", "reclaim", "--dir", rf.repo, "--apply")
	assert.Equal(t, 0, applied.code, applied.stderr)
	assert.NoDirExists(t, rf.harness)
	check := runDiskCLI(t, "disk", "check", "--dir", rf.repo)
	assert.Equal(t, 0, check.code, check.stderr)
	reg := runDiskCLI(t, "disk", "register", rf.elsewhere)
	assert.Equal(t, 1, reg.code)
	assert.Contains(t, reg.stderr, "is refused")
	entries, err := os.ReadDir(elsewhere)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing was written through the link")
}

// SPEC-DISK-002 review r2: an event that cannot be written leaves the
// reclaim's outcome and exit code unchanged.
func TestDiskReclaim_EventWriteFailureKeepsExitCode(t *testing.T) {
	rf := newReclaimFixture(t)
	state, err := openDiskState()
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(state.Join("events.jsonl"), 0o700))

	applied := runDiskCLI(t, "disk", "reclaim", "--dir", rf.repo, "--apply")
	assert.Equal(t, 0, applied.code, applied.stderr)
	assert.NoDirExists(t, rf.harness)
}
