# Disk management on a shared machine

`auto disk` gives the harness a disk budget it can read, a preflight it runs
before heavy steps, an observation command that names what fills the disk, and
a trim command for the Go build cache. It is written for one machine that hosts
several agent sessions, their worktrees, and CI runners at once, where a full
disk fails every running job with errors that look like regressions.

Defined by SPEC-DISK-001 (GitHub issue #198, first slice). The trim command
deletes only Go build cache entries and abandoned Go build work directories.
Nothing else is ever deleted: worktrees are listed as suggestions, and
scratchpads, VM images, model caches, and other projects are only reported.

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
```

`auto doctor` adds an advisory `Disk` row: `[OK]` at or above the floor,
`[WARN]` below it. The row never changes the overall doctor verdict.

## Exit codes

| Command | 0 | 1 | 3 |
| --- | --- | --- | --- |
| `auto disk check` | enough free space | measurement failed, or bad flags | insufficient free space |
| `auto disk trim` | dry run, or apply with no failure | a deletion failed, or a root was refused | — |
| `auto disk status` | report printed | measurement failed | — |

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
```

Every `*idle_minutes` value must be at least 60. A value below 120 is accepted
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
