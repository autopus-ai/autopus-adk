# Disk management on a shared machine

`auto disk` gives the harness a disk budget it can read, a preflight it runs
before heavy steps, an observation command that names what fills the disk, a
trim command for the Go build cache, a reclaim command for finished work, disk
leases for parallel heavy commands, an event log, and a low-space alert in
Claude Code sessions. It is written for one machine that hosts several agent
sessions, their worktrees, and CI runners at once, where a full disk fails
every running job with errors that look like regressions.

Defined by SPEC-DISK-001 (budget, check, status, trim) and SPEC-DISK-002
(ledger, reclaim, leases, log, alert), GitHub issue #198. Deletion is limited
to Go build cache entries, abandoned Go build work directories, worktrees the
harness owns (see [Reclaim](#reclaim)), and idle Claude Code session
scratchpads. VM images, model caches, container images, scratch databases, and
other people's worktrees are only reported, never deleted.

## Commands

```sh
auto disk status                 # free space, consumers, what trim would reclaim
auto disk status --json
auto disk check --need 30        # exit 3 when less than 30 GiB is free
auto disk check --for release    # use the named reserve from autopus.yaml
auto disk trim                   # dry run: what would be deleted, and why
auto disk trim --list            # ... with every candidate path
auto disk trim --apply           # delete the candidates
auto disk trim --ci idle         # override CI detection (running | idle)
auto disk reclaim                # dry run: finished worktrees and idle scratchpads
auto disk reclaim --apply        # remove them
auto disk register <worktree>    # record a worktree you created outside the harness dirs
auto disk release <worktree>     # mark it done; reclaim removes it once clean
auto disk registry               # list the ownership ledger
auto disk lease --need 20 -- cargo test --workspace
auto disk log                    # the latest trims, reclaims, refusals, and alerts
```

`auto doctor` adds an advisory `Disk` row: `[OK]` at or above the floor,
`[WARN]` below it. The row never changes the overall doctor verdict.

## Exit codes

| Command | 0 | 1 | 3 |
| --- | --- | --- | --- |
| `auto disk check` | enough free space | measurement failed, or bad flags | insufficient free space |
| `auto disk trim` | dry run, or apply with no failure | a deletion failed, or a root was refused | — |
| `auto disk status` | report printed | measurement failed | — |
| `auto disk reclaim` | dry run, or apply with no failure | a removal failed, or the scratchpad root was refused | — |
| `auto disk lease` | the command's own code (also 1 to 255) | usage, configuration, measurement, or state failure | the wait ran out |

A caller should go on only on exit 0 from `auto disk check`, and stop on 3 and
on any other non-zero code: a failed measurement is not a pass. In text mode the
check prints exactly one line; with `--json` it prints only the JSON envelope
(codes `disk_insufficient`, `disk_check_invalid`, `disk_measurement_failed`).

Free space is measured with `statfs` on the volume that holds the project, from
the same fields `df` uses. Linux and macOS are supported; other systems exit 1
with `disk: free space measurement is not supported on <GOOS>`.

## Configuration

Every key is optional. A project with no `disk:` section gets the defaults
below, and a configuration at defaults writes no `disk:` key. Sizes ending in
`_gb` are GiB and `_mb` are MiB.

```yaml
disk:
  min_free_gb: 10            # floor; below it trim runs in emergency mode
  reserves:                  # named needs for `auto disk check --for <name>`
    release: 30
  trim:
    start_below_gb: 0        # 0 = always evaluate; otherwise do nothing above this
    ci_process_names: [Runner.Worker]
    go_build:
      file_mb: 1
      idle_minutes: 120           # no CI running
      ci_idle_minutes: 180        # a CI job is running
      emergency_idle_minutes: 120 # free space below the floor
      large_file_mb: 50
      large_file_idle_minutes: 120
      large_file_below_gb: 0      # 0 = the large-entry rule applies at any free space
    go_build_work_dirs:
      idle_minutes: 360
  reclaim:
    worktree_idle_minutes: 360    # merged, clean, owned worktree idle this long
    scratchpad_idle_minutes: 1440 # session scratchpad idle this long
  lease:
    max_concurrent: 0             # 0 = no count limit; otherwise at most this many leases
    wait_minutes: 10              # how long a lease waits for space by default
    reclaim_on_shortage: true     # trim and reclaim once before waiting
    cpu_slots: 0                  # CPU slots --cpu shares; 0 = the logical CPU count (at most 4096)
    dir: ""                       # absolute shared lease dir for several users; "" = your state dir
    aging_minutes: 10             # a CPU waiter rises one priority class per interval; 0 = no aging
    max_load_ratio: 0.85          # admit a CPU waiter while 1-min load <= this x logical CPUs; 0 = off
    min_free_mem_gb: 8            # memory left free after every --mem reservation; 0 = no floor
    max_load_wait_minutes: 10     # with no lease running, load alone holds the head waiter this long; 0 = never force
```

Every `trim` and `reclaim` `*idle_minutes` value must be at least 60, and
`lease.max_concurrent` and `lease.wait_minutes` at least 0. Every idle and
wait value is at most 5256000 (ten years), every `*_gb` value at most 1048576
(1 PiB), and every `*_mb` value at most 1073741824, so no threshold can
overflow into "always idle" or "never short". A value below 120 is accepted
but `auto disk status` and `auto disk trim` warn about it, for the reason below.

## How trim decides

The mode is `emergency` when free space is below `min_free_gb`, otherwise `ci`
when a CI job is running, otherwise `idle`. Three rules are evaluated in a fixed
order, and a candidate belongs to the first rule it matches, so it is counted
and deleted once:

1. `go_build_large_files`: cache entries larger than `large_file_mb` idle
   beyond `large_file_idle_minutes`, in every mode.
2. `go_build_files`: cache entries larger than `file_mb` idle beyond the mode's
   threshold (`idle_minutes`, `ci_idle_minutes`, or `emergency_idle_minutes`).
3. `go_build_work_dirs`: `go-build<N>` directories under `$GOTMPDIR` (else the
   system temp directory) whose newest entry anywhere in the tree is idle
   beyond `go_build_work_dirs.idle_minutes`. A live build touches files inside
   its work directory, so the whole tree is checked, not only its top level.
   The name alone is not enough: every top-level child must be a `b001`-style
   directory, which is all Go creates there, and a directory that is the
   project, the home directory, or the current directory is never a candidate.
   A name match that fails these checks is counted as `skipped_shape`.

A cache entry is `<xx>/<64 hex>-d`: a regular file, or, for a cached
executable, a directory that holds the binary. Go marks an entry used by
updating that entry's own mtime, so idle time is read from the entry itself and
the files inside an executable directory are never judged one by one.

CI is detected by an exact executable-name match against `ci_process_names`. A
command-line substring match would also count the shells that mention the name.
If the process list cannot be read, trim treats CI as running (`ci=unknown`),
unless free space is below the floor, where emergency mode still applies.

### Why the idle thresholds default to 120 minutes or more

Go refreshes a cache entry's modification time when the entry is reused, but at
most once an hour (`mtimeInterval` in `cmd/go/internal/cache`). An entry whose
mtime is N minutes old is therefore guaranteed unused only for the last N − 60
minutes. A build looks an entry up and reads it later, so deleting it in between
breaks the build with `could not import ... no such file or directory`. Go's own
cache trimmer subtracts an extra hour from its cutoff for this reason.

A 60-minute threshold leaves no guaranteed window. The SPEC's probe reproduced
the failure: a held build lost all 21 of its cache files at 61 minutes and
failed, while a 120-minute threshold deleted nothing and the build passed.

mtime-based trimming narrows this risk; it does not remove it. A build that
looked an entry up just before the threshold passed can still lose it.
Re-checking each candidate right before deletion shrinks the window further but
cannot close it. Every dry run prints this as a `note:` line.

## What trim never does

- It never runs `go clean`; the only `go` command it runs is `go env GOCACHE`,
  only when `$GOCACHE` is unset, and with `GOTOOLCHAIN=local GOWORK=off` so a
  `go.mod` above the temp directory cannot make it fetch another toolchain.
- It refuses a cache root or work-dir root that resolves to `/`, the home
  directory, or the project directory, is relative, is not owned by the current
  user, or (for the cache) lacks Go's `README` marker, and then deletes nothing
  from either root. A root-owned sticky temp directory such as Linux `/tmp` (used
  when `$GOTMPDIR` and `$TMPDIR` are unset) is accepted as the work-dir root;
  only the current user's `go-build<N>` directories in it are candidates. A
  sticky directory owned by another user is refused, because its owner can
  rename any entry in it.
- It skips a candidate owned by another user (`skipped_owner`).
- It records each root's and candidate's identity at scan time. Before deleting
  a file it checks the device, inode, size, and mtime again and skips the file
  when any changed (`skipped_changed`). A directory candidate is walked first
  without following symlinks; any symlink, device change, or special file skips
  the whole candidate (`skipped_symlink`, `skipped_device`) before anything in it
  is deleted.
- It never deletes `README`, `trim.txt`, `log.txt`, or any `-a` file, and never
  removes worktrees, branches, `target/` directories, scratchpads, or anything
  outside the two roots.

An apply prints `disk trim: reclaimed <size> (deleted=… skipped_changed=…
skipped_symlink=… skipped_device=… skipped_owner=… vanished=… failed=…)`; an
entry that disappeared on its own counts as `vanished`, not as a failure.

## Reclaim

`auto disk reclaim` removes two kinds of finished work, as a dry run unless
`--apply` is given.

**Worktrees.** A linked worktree is *owned* when it lives under
`.autopus/worktrees/`, `.claude/worktrees/`, or `.worktrees/` of its main
worktree, directly under the system temp dir as `autopus-worktree-<digits>`
(the pipeline's), or was registered with `auto disk register`. Reclaim removes
an owned worktree only when the current user owns it, it is clean, unlocked,
not prunable, does not hold the current directory, has nothing mounted
inside it (git would delete across the mount), and either its owner
released it (and it has a branch or its HEAD is merged) or it is merged into
its base and idle beyond `reclaim.worktree_idle_minutes`. Idle is read from
the newest file anywhere in the worktree outside `.git` (so a build writing
deep inside `target/` keeps it active) and its git `HEAD`, `index`, and
`logs/HEAD`. Right before removal every one of those gates is
checked again, together with the worktree's identity and HEAD; anything that
changed is skipped as `skipped_changed`. Git removes the worktree without
`--force`, so it refuses a dirty or locked one on its own; the branch is
always kept.

A linked worktree that is neither in those directories nor registered is
removed only by the automatic rules (`merged_linked`, and agent worktrees
outside the harness directories): merged into `origin/HEAD` or
`origin/main`, clean, idle, and held by no process. Because
`git worktree remove` deletes git-ignored files even without `--force`, such
a worktree is also kept (`ignored_files`) while it holds any ignored file
outside a regenerable build directory (`node_modules`, `.next`, or `dist`
beside a `package.json`, `target` beside a `Cargo.toml`); this is checked at
scan time and again right before removal.

Register a worktree you create anywhere else, and release it when its work is
merged or abandoned:

```sh
git worktree add -b fix/x ../wt/fix-x origin/main
auto disk register ../wt/fix-x --owner "session 0646"
# ... work, push, merge ...
auto disk release ../wt/fix-x
```

Registration writes an identity token into the worktree's git administrative
directory (`$GIT_DIR/worktrees/<id>/autopus-disk-token`) and records it in the
ledger. Git deletes that directory with the worktree, so a new worktree later
created at the same path is never mistaken for the registered one, even on a
filesystem that reuses the inode number. Release records the worktree's
`HEAD`; a released worktree checked out on another branch or commit
afterwards is treated as registered, not released. Ledger entries written
before tokens existed are no longer trusted: register those worktrees again.

**Scratchpads.** A Claude Code session directory
`/tmp/claude-<uid>/<project slug>/<session uuid>` whose newest entry is idle
beyond `reclaim.scratchpad_idle_minutes` is removed. The scratchpad root must be
owned by you and writable by nobody else, or nothing in it is deleted. Inside a
session, symlinks and other non-directories are unlinked as names and never
followed; a tree on another device is skipped whole. Loose files at the top of
the scratchpad root are never touched.

## Leases

`auto disk lease` runs a heavy command only while free space minus the
reservations of the other running leases, minus its own `--need`, stays at or
above `disk.min_free_gb`:

```sh
auto disk lease --need 20 -- cargo build --workspace
auto disk lease --for release --wait 30m -- make release
```

When space is short it runs `auto disk trim --apply` and
`auto disk reclaim --auto --apply` once (`lease.reclaim_on_shortage`): nobody
watches it, so released worktrees, session scratchpads, and large files are
left for a person. Then it waits,
re-checking every 5 seconds, up to `--wait` (default `lease.wait_minutes`), and
exits 3 if space never came. A refusal because `lease.max_concurrent` leases
already run does not reclaim anything. The reservation is a locked file that
the command inherits, so it lasts exactly as long as the command, even when the
wrapper itself is killed; it is not reduced as the command writes, so admission
is conservative. A background process the command leaves running inherits the
reservation too and keeps it until it exits; `auto disk status` marks such a
lease `orphaned` once the wrapper is gone, so a daemon started under a lease
(a Gradle or Bazel server, for example) shows up there instead of silently
holding space. `SIGTERM` and `SIGHUP` are forwarded to the command; `SIGINT`
is not, because the terminal already sends it to the command. The label shown
by status and the log is `--label`, or the command's program name. Leases only
coordinate commands that use them: CI jobs and unwrapped builds still take
space without asking.

### CPU slots

`--cpu N` reserves N CPU slots as well; with `--cpu` alone no `--need` is
required and the disk floor is not consulted. The machine has
`AUTOPUS_LEASE_CPU_SLOTS`, else `lease.cpu_slots`, else as many slots as
logical CPUs, and a larger N is clamped to that. A refused CPU request takes
a queue ticket (a locked `.wait` file) that records its class and when it
was queued; no request is admitted while a live ticket ahead of it waits.
Ahead means a higher effective class (`--priority interactive`, `ci`, or
`background`; the default is `ci`, or inside GitHub Actions `interactive`
for `pull_request`, `pull_request_target`, and `merge_group` and
`background` for `schedule`), or the same class and queued earlier. A
waiter's effective class rises one step every `lease.aging_minutes`
(default 10; 0 turns aging off), so background work is reached in at most
two intervals. A running holder is never preempted.
A killed holder or waiter releases its lock and is discarded by the next
admission. When the wait runs out the lease exits 3 with a line such as
`disk lease: 4 CPUs requested, 2 of 8 slots free after 6 reserved by 2 leases, 1 waiters ahead; waited 30m0s`.
The command runs with `AUTOPUS_LEASE_CPUS=N` and `GOMAXPROCS=N`; a
`GOMAXPROCS` you set yourself is kept, and one set by an outer lease is
replaced. `lease.max_concurrent` counts disk leases only.

### Load and memory

Slots cannot tell an idle machine from a busy one, and work that takes no
lease (other sessions, CI runners) loads it too. So a CPU waiter at the head
of the queue, with its slots free, is also held while any of these holds:

- the 1-minute load average plus the in-flight grants is above
  `lease.max_load_ratio` × logical CPUs (default 0.85; 0 turns it off). The
  load average is a moving average with a 60-second time constant, so a
  lease admitted t seconds ago still counts `cpus × e^(-t/60)` on top of it
  for five minutes: a burst of admissions cannot overshoot before the
  average catches up;
- available memory minus the `--mem` reservations of the running leases is
  below the waiter's own `--mem` plus `lease.min_free_mem_gb` (default 8).
  `--mem SIZE` takes `8G`, `512M`, or a bare number of GiB, needs `--cpu`,
  and defaults to none, so only the floor applies;
- on macOS, `kern.memorystatus_vm_pressure_level` is critical.

macOS is read through `sysctl` with no process spawned: `vm.loadavg`,
`hw.memsize`, and `kern.memorystatus_level` (the free percentage that
`memory_pressure` prints, which counts inactive and purgeable pages); where
that key is missing, free, speculative, and purgeable pages are a lower
bound. Linux reads `/proc/loadavg` and `MemAvailable` from `/proc/meminfo`
and has no pressure check. A reading that fails is skipped, so admission
falls back to slots. `AUTOPUS_LEASE_MAX_LOAD_RATIO` and
`AUTOPUS_LEASE_MIN_FREE_MEM_GB` override the keys on one machine. A CPU wait
re-checks about every 2 seconds, with ±25% jitter.

The waiters behind the head wait for it, so priority and aging order the
admissions exactly as above. `auto disk status` appends what holds the head
waiter to its line, such as `; waiting: load 22.1 > 20.4` or
`; waiting: mem 6.2 GiB < 8+4 GiB`, and a timeout line ends the same way.
A `--mem` that with the floor exceeds the machine's memory fails at once
with exit 1. When no lease runs and only load has held the head waiter for
`lease.max_load_wait_minutes` since it was queued, it is admitted anyway,
prints `disk lease: starting <label> despite load …`, and logs a
`lease_forced` event; the next forced admission again needs no lease
running, so they go one at a time. Memory and pressure are never forced.
Such a lease needs a `--wait` longer than `max_load_wait_minutes`.

Migrating from a shell wrapper that serialises heavy jobs with a single
lock: replace it with `auto disk lease --cpu N --mem SIZE --priority … --
cmd`. The single lock runs one job however idle the machine is; the lease
runs as many as the slots, load, and memory allow.

Leases live in `leases/` of your state dir. To account for several users of
one machine (the runner accounts of a CI host), give all of them the same
`AUTOPUS_LEASE_DIR` or `lease.dir`. A missing directory is created with mode
1777; an existing one writable by group or others must have the sticky bit,
so no user can remove a lease another holds. Lease files there are 0644 and
locked through read-only descriptors. Where no lease can be held (Windows),
`--cpu` alone prints a warning and runs the command unleased with the same
environment.

## Events, trend, and alerts

`auto disk log` shows the latest `trim_apply`, `reclaim_apply`,
`lease_reclaimed`, `lease_refused`, `lease_forced`, and `low_space_alert` events. `auto disk
status`, `check`, `lease`, and the alert record a free-space sample at most
every 5 minutes per volume; `status` prints `free trend: <GiB>/h over
<duration>` once a sample between 10 minutes and 6 hours old exists.

In Claude Code, the per-prompt sticky hook adds this to the session's context
on every turn while free space is below the floor:

```text
Disk alert: 8.2 GiB free on /System/Volumes/Data, below the floor of 10.0 GiB (disk.min_free_gb). Do not start builds, worktrees, or parallel agents until space recovers; run `auto disk status`, then `auto disk reclaim --apply` and `auto disk trim --apply`. If you lead other agents, tell each of them to pause heavy work.
```

At or above the floor the hook's output is unchanged. Other platforms rely on
the `auto disk check` preflight in their guidance.

## State directory

The ledger, leases, and logs live in `$XDG_STATE_HOME/autopus/disk` (absolute
`XDG_STATE_HOME` only), else `~/.local/state/autopus/disk`. Each directory is
created with mode 0700 and refused when it is a symlink, owned by someone else,
or writable by group or others. When it is refused, `register`, `release`,
`registry`, `lease`, and `log` exit 1; `reclaim` still removes harness
worktrees and scratchpads; `status`, `check`, `trim`, and the alert work
without recording.

## Running trim and reclaim periodically

The harness installs no scheduler. To run both every 10 minutes, use your
system's own; replace `auto` with its absolute path when the scheduler's `PATH`
does not include it, and run from a project directory so its `autopus.yaml`
applies.

macOS launchd (`~/Library/LaunchAgents/co.autopus.disk.plist`, then
`launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/co.autopus.disk.plist`):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>co.autopus.disk</string>
  <key>ProgramArguments</key>
  <array><string>/bin/sh</string><string>-c</string>
    <string>auto disk trim --apply; auto disk reclaim --apply</string></array>
  <key>StartInterval</key><integer>600</integer>
</dict>
</plist>
```

Linux systemd user units (`~/.config/systemd/user/autopus-disk.service` and
`.timer`, then `systemctl --user enable --now autopus-disk.timer`):

```ini
# autopus-disk.service
[Service]
Type=oneshot
ExecStart=/bin/sh -c 'auto disk trim --apply; auto disk reclaim --apply'

# autopus-disk.timer
[Timer]
OnUnitActiveSec=10min
OnBootSec=5min
[Install]
WantedBy=timers.target
```

cron (`crontab -e`):

```cron
*/10 * * * * auto disk trim --apply; auto disk reclaim --apply
```

## Known residual risks

- A build that looked a cache entry up just before its idle threshold passed can
  still lose it (see the idle section above).
- A `go run` or `go test` binary that runs for longer than
  `go_build_work_dirs.idle_minutes` without writing to its work directory makes
  that directory look abandoned. On Unix the running binary survives the
  unlink, but anything it later writes into its work directory fails.
- On Linux with `hidepid` mounted, processes of other users are invisible, so a
  CI job running as another user is not detected; trim then uses the `idle`
  thresholds instead of the stricter `ci` ones.
- A process outside the harness that works in an owned, merged, clean worktree
  without writing to its top two levels or its git metadata for
  `reclaim.worktree_idle_minutes` loses the worktree. Releasing finished
  worktrees and keeping the default of six hours bound this.
- A Claude Code session resumed after `reclaim.scratchpad_idle_minutes` finds
  its scratchpad gone.
- A change made to a worktree between reclaim's last check and git's own checks
  is the remaining race; git still refuses it when the change leaves the
  worktree dirty.
- On Linux ext4, a worktree removed by hand and recreated at a registered path
  can get the old inode back and inherit a released ledger entry. Release or
  register again after recreating a worktree at the same path.
