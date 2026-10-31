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
| `auto disk check` | enough free space | measurement or usage error | insufficient free space |
| `auto disk trim` | dry run, or apply with no failure | a deletion failed, or the cache root was refused | — |
| `auto disk status` | report printed | measurement error | — |

Free space is measured with `statfs` on the volume that holds the project, the
same number `df` prints. Linux and macOS are supported; other systems exit 1
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
      idle_minutes: 120          # no CI running
      ci_idle_minutes: 180       # a CI job is running
      emergency_idle_minutes: 60 # free space below the floor
      large_file_mb: 50
      large_file_idle_minutes: 60
      large_file_below_gb: 0     # 0 = the large-file rule applies at any free space
    go_build_work_dirs:
      idle_minutes: 360
```

Every `*idle_minutes` value must be at least 60, for the reason below.

## How trim decides

The mode is `emergency` when free space is below `min_free_gb`, otherwise `ci`
when a CI job is running, otherwise `idle`. The mode picks the idle threshold
for cache entries larger than `file_mb`. Two rules apply in every mode: linked
executables larger than `large_file_mb` idle beyond `large_file_idle_minutes`
(a build re-links them when they are missing), and `go-build<N>` work
directories idle beyond `go_build_work_dirs.idle_minutes` (a running build
touches its work directory continuously, so a long-idle one was abandoned by a
killed build).

CI is detected by an exact executable-name match against
`ci_process_names`. A command-line substring match would also count the shells
that mention the name. If the process list cannot be read, trim assumes a CI
job is running and uses the stricter rule set.

### Why the idle threshold is at least 120 minutes by default

Go refreshes a cache entry's modification time when the entry is reused, but at
most once an hour (`mtimeInterval` in `cmd/go/internal/cache`). An entry whose
mtime is N minutes old is therefore guaranteed unused only for the last N − 60
minutes. A build looks an entry up and reads the file later, so deleting it in
between breaks the build with `could not import ... no such file or directory`.
Go's own cache trimmer subtracts an extra hour from its cutoff for this reason.

A 60-minute threshold leaves no guaranteed window. The SPEC's probe reproduced
the failure: a held build lost all 21 of its cache files at 61 minutes and
failed, while a 120-minute threshold deleted nothing and the build passed. The
`emergency_idle_minutes` default stays at 60 because below the floor a full
disk fails every running build anyway.

## What trim never does

- It never runs `go clean`; the only `go` command it runs is `go env GOCACHE`.
- It deletes only regular files named `<xx>/<64 hex>-d` under the resolved
  cache root, and only `go-build<N>` directories directly under the work-dir
  root (`$GOTMPDIR`, else the system temp directory).
- It refuses a cache root that lacks Go's `README` marker or resolves to `/`,
  the home directory, or the project directory, and deletes nothing.
- It deletes through a handle rooted at the cache directory, never follows a
  symlink, and checks each file again right before removal, skipping any file
  Go touched since the scan.
- It never removes worktrees, branches, `target/` directories, scratchpads, or
  anything outside the two roots above.
