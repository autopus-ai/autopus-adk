# SPEC-DISK-001 Acceptance Criteria

Units: GiB is 1073741824 bytes, MiB is 1048576 bytes; human sizes print one decimal. Free space, CI state, and the clock enter the CLI through test seams. Fixture files are written fully (non-sparse), so allocated bytes equal apparent size; the explicit tolerance on every fixture byte total is one 4096-byte block per file. `<hN>` is a 64-character lowercase hex name whose first two characters equal its subdirectory.

## Test Scenarios

### S1: AC-DISK-001 - Configuration defaults and validation are exact
Priority: Must
Given an `autopus.yaml` without a `disk` key
When the configuration is loaded and `Disk.Effective()` is read
Then the expected values are floor 10737418240 bytes, no reserves, `start_below_gb` 0, `ci_process_names` `["Runner.Worker"]`, `file_mb` 1, `idle_minutes` 120, `ci_idle_minutes` 180, `emergency_idle_minutes` 60, `large_file_mb` 50, `large_file_idle_minutes` 60, `large_file_below_gb` 0, work dir `idle_minutes` 360
And saving that configuration writes no `disk` key
And given instead each invalid file below, loading fails with an error containing the listed text

| disk section | expected error contains |
|--------------|-------------------------|
| `min_free_gb: -1` | `disk.min_free_gb must be >= 0` |
| `reserves: {release: 0}` | `disk.reserves.release must be a positive integer` |
| `reserves: {"Release Build": 30}` | `disk.reserves name "Release Build" must match` |
| `trim: {go_build: {idle_minutes: 30}}` | `disk.trim.go_build.idle_minutes must be >= 60` |
| `trim: {go_build_work_dirs: {idle_minutes: 59}}` | `disk.trim.go_build_work_dirs.idle_minutes must be >= 60` |
| `trim: {go_build: {file_mb: 0}}` | `disk.trim.go_build.file_mb must be >= 1` |
| `trim: {ci_process_names: ["bin/Runner.Worker"]}` | `disk.trim.ci_process_names entries must be bare executable names` |
| `trim: {go_build: {idle_min: 120}}` | the existing strict-decoder unknown-key error |

And `min_free_gb: 45` with `reserves: {release: 30, visual-linux: 37}` loads with floor 48318382080 bytes and both reserves.

### S2: AC-DISK-002 - Free space equals df and names the mount
Priority: Must
Given the project directory on the macOS Data volume
When `diskspace.Free(projectDir)` and `df -k <projectDir>` run within the same second
Then the expected value of `Free` is `Available * 1024` from `df -k` within an explicit tolerance of 1 MiB, and the mount is `/System/Volumes/Data` for the project path, `/tmp`, and `$TMPDIR`
And on a linux host the same comparison holds against `df -k` with the mount taken from `/proc/self/mounts`
And on any other `GOOS` each of `auto disk status`, `auto disk check`, and `auto disk trim` exits 1 with the single stderr line `disk: free space measurement is not supported on <GOOS>`.

### S3: AC-DISK-003 - Check exit codes and messages follow the requirement table
Priority: Must
Given floor 45 GiB, reserves `{release: 30, nightly-full: 60}`, mount `/System/Volumes/Data`, and the free values below injected through the seam
When `auto disk check` runs with the listed flags
Then the exit code and the exact expected stdout or stderr line are

| flags | free GiB | exit | stream | expected line |
|-------|----------|------|--------|---------------|
| none | 9.0 | 3 | stderr | `disk check: insufficient free space: 9.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 36.0 GiB or lower the requirement` |
| none | 45.0 | 0 | stdout | `disk check: ok: 45.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data` |
| `--need 30` | 36.0 | 3 | stderr | `disk check: insufficient free space: 36.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 9.0 GiB or lower the requirement` |
| `--need 50` | 52.5 | 0 | stdout | `disk check: ok: 52.5 GiB free, 50.0 GiB required (--need) on /System/Volumes/Data` |
| `--for release` | 31.0 | 3 | stderr | `disk check: insufficient free space: 31.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 14.0 GiB or lower the requirement` |
| `--for nightly-full` | 61.0 | 0 | stdout | `disk check: ok: 61.0 GiB free, 60.0 GiB required (disk.reserves.nightly-full) on /System/Volumes/Data` |
| `--for nightly-full` | 59.9 | 3 | stderr | `disk check: insufficient free space: 59.9 GiB free, 60.0 GiB required (disk.reserves.nightly-full) on /System/Volumes/Data; free 0.1 GiB or lower the requirement` |
| `--for weekly` | 200.0 | 1 | stderr | `disk check: reserve "weekly" is not configured under disk.reserves in autopus.yaml` |
| `--need 0` | 200.0 | 1 | stderr | `disk check: --need must be a positive number of GiB` |
| `--need 30 --for release` | 200.0 | 1 | stderr | `disk check: --need and --for are mutually exclusive` |

And with `--json` the exit codes are identical, the envelope `status` is `ok` for exit 0 and `error` with `error.code` `disk_insufficient` for exit 3, and `data` carries `free_bytes`, `required_bytes`, `source`, and `mount` as integers and strings.

### S4: AC-DISK-004 - Status reports every consumer with exact bytes
Priority: Must
Given the S5 cache fixture as `$GOCACHE`, the S5 work dir fixture as `$GOTMPDIR`, the S7 repository as the project, a scratchpad fixture `<tmp>/claude-<uid>/` holding two session directories of 1 MiB and 3 MiB, free 100 GiB, floor 10 GiB, and `--ci idle`
When `auto disk status --json` runs
Then the expected JSON `data.consumers` holds exactly four kinds: `go_build_cache` with `bytes` 135802880 (h1, h2, h3, h4, h5, the `-a` file, `stray.bin`, `README`, and `trim.txt` summed as allocated blocks, symlink excluded) and `entries` 5, `go_build_work_dirs` with `dirs` 2, `idle_dirs` 1, and `bytes` 3248128, `worktrees` with the S7 items, and `scratchpads` with `dirs` 2 and `bytes` 4194304, each within the stated tolerance
And `data.free_bytes` is 107374182400, `data.floor.ok` is true, and `data.trim_preview.total_bytes` equals the S5 idle-mode total 68157440
And the text output prints one row per kind and ends with the S5 dry-run block
And with the project outside any git repository the `worktrees` consumer reports `available` false and `reason` `not_a_git_repository`, and with `GOCACHE=off` the cache consumer reports `reason` `gocache_off`.

### S5: AC-DISK-005 - Trim selects, previews, and deletes exactly the policy set
Priority: Must
Given a cache root fixture `R` holding a regular `README` and a `trim.txt` of 1 KiB each plus the entries below, and a work dir root fixture `W` holding `go-build111/` (one 3 MiB file, directory mtime 400 min old), `go-build222/` (one 100 KiB file, 100 min old), `go-build333` (a symlink to a directory outside `W` holding a 2 MiB file), `msgmcp-railway-roles1/` (400 min old), and `notes/` (400 min old)

| entry under R | size | mtime age | kind |
|---------------|------|-----------|------|
| `aa/<h1>-d` | 2 MiB | 130 min | regular file |
| `aa/<h2>-d` | 2 MiB | 90 min | regular file |
| `bb/<h3>-d` | 60 MiB | 70 min | regular file |
| `bb/<h4>-d` | 60 MiB | 30 min | regular file |
| `cc/<h5>-d` | 512 KiB | 500 min | regular file |
| `cc/<h6>-a` | 100 B | 500 min | action file |
| `dd/<h7>-d` | symlink to `outside/big.bin` (2 MiB, 500 min) | 500 min | symlink |
| `ee/stray.bin` | 5 MiB | 500 min | non-entry name |

When `auto disk trim` runs with floor 10 GiB and the mode inputs below
Then the expected stdout for the idle run (free 100 GiB, `--ci idle`) is exactly

```
disk trim: mode=dry-run free=100.0 GiB ci=forced-idle emergency=no rules=go_build_files(>1 MiB, idle>120m) go_build_large_files(>50 MiB, idle>60m) go_build_work_dirs(idle>360m)
  note: idle>120m guarantees 60m without use (Go refreshes a reused entry's mtime at most hourly)
  go_build_files: candidates=1 size=2.0 MiB
  go_build_large_files: candidates=1 size=60.0 MiB
  go_build_work_dirs: candidates=1 size=3.0 MiB
  skipped_symlink=2 (go_build_cache=1, go_build_work_dirs=1)
disk trim: would reclaim 65.0 MiB (dry-run; pass --apply to delete)
```

And the candidate sets per mode are: idle (free 100 GiB, `--ci idle`) h1, h3, `go-build111`, total 65.0 MiB; ci (free 100 GiB, `--ci running`) h3 and `go-build111`, total 63.0 MiB, header `ci=forced-running` and rule `go_build_files(>1 MiB, idle>180m)`; emergency (free 5 GiB, `--ci running`) h1, h2, h3, `go-build111`, total 67.0 MiB, header `emergency=yes` and rule `go_build_files(>1 MiB, idle>60m)`; and with `start_below_gb: 80` and free 100 GiB the only output is `disk trim: inactive (free above start_below_gb)` with exit 0
And `--list` prints `aa/<h1>-d`, `bb/<h3>-d`, and `go-build111` on their own lines in the idle run
And `--apply` in the idle run deletes exactly h1, h3, and `go-build111`, leaves h2, h4, h5, the `-a` file, `dd/<h7>-d` as a symlink, `outside/big.bin`, `stray.bin`, `README`, `trim.txt`, `go-build222`, `go-build333`, its target directory, `msgmcp-railway-roles1`, and `notes` byte-identical, prints `disk trim: reclaimed 65.0 MiB (deleted=3 skipped_refreshed=0 skipped_symlink=2 vanished=0 failed=0)`, and exits 0
And with a test hook that touches h1 between scan and removal the apply run keeps h1 and prints `deleted=2 skipped_refreshed=1 skipped_symlink=2 vanished=0 failed=0`
And with a hook that unlinks h3 between scan and removal the run prints `deleted=2 skipped_refreshed=0 skipped_symlink=2 vanished=1 failed=0` and exits 0
And with `README` removed from `R` the dry run and the apply run both print `disk trim: refused: <R> is not a Go build cache (no README marker)`, delete nothing, and exit 1, while a missing `R` prints `go_build_cache: unavailable (cache root <R> does not exist)` and exits 0
And an exec recorder sees exactly one `go` invocation, `go env GOCACHE`, and no `clean` argument in any run.

### S6: AC-DISK-006 - CI detection matches executable basenames only
Priority: Must
Given `ci_process_names` `["Runner.Worker"]` and an injected process lister
When the lister returns the rows below
Then the expected detection results are

| lister rows | running | header `ci=` |
|-------------|---------|--------------|
| `/opt/actions-runner/bin/Runner.Listener`, `/opt/actions-runner/bin/Runner.Worker`, `/bin/zsh` | true | `running` |
| `/opt/actions-runner/bin/Runner.Listener`, `/bin/zsh`, `/usr/bin/grep` | false | `idle` |
| `/opt/actions-runner/bin/Runner.Worker.old`, `/bin/zsh` | false | `idle` |
| lister returns an error | true | `unknown` |

And the error row also prints `disk trim: ci detection unavailable (<reason>); applying the CI rule set`
And `--ci idle` with the first row yields `forced-idle` and the idle rule set, and `--ci running` with the second row yields `forced-running` and the ci rule set
And on the dev Mac the darwin lister over `ps -axo comm=` reports running while a `Runner.Worker` process exists and idle when only `Runner.Listener` processes exist.

### S7: AC-DISK-007 - Worktrees are classified, sized, and never touched
Priority: Must
Given a temporary repository with branch `main`, a `.gitignore` containing `target/`, and three linked worktrees: `wt-a` on `feat-a` whose commit is merged into `main` and which holds `crate/Cargo.toml` and `crate/target/blob.bin` of 4 MiB; `wt-b` on `feat-b` with one commit not merged into `main`; `wt-c` on `feat-c` at the `main` commit with a modified tracked file
When `auto disk status --json` runs from the main worktree with the other consumers unavailable
Then the expected JSON `worktrees.items` has four entries: the main worktree with `main` true and no suggestion; `wt-a` with `clean` true, `merged` true, `base` `main`, `harness` false, `cargo_target_bytes` 4194304 within tolerance; `wt-b` with `merged` false and `clean` true; `wt-c` with `merged` true and `clean` false
And the text output contains exactly one suggestion line, `candidate: git worktree remove <path of wt-a> (merged into main, clean, 4.0 MiB)`
And `git worktree list --porcelain` output and every file under all four worktrees are byte-identical before and after the run
And a worktree created under `.autopus/worktrees/` of the main worktree reports `harness` true.

### S8: AC-DISK-008 - The doctor row is advisory and mirrored in JSON
Priority: Must
Given floor 45 GiB and mount `/System/Volumes/Data`
When `auto doctor` runs with free 9.0 GiB and then with free 100.0 GiB
Then the expected text lines are `[WARN] disk: 9.0 GiB free on /System/Volumes/Data is below the 45.0 GiB floor (disk.min_free_gb); run 'auto disk status'` under a `Disk` section header, and `[OK] disk: 100.0 GiB free on /System/Volumes/Data (floor 45.0 GiB)`
And the result box verdict is unchanged by the disk row in both runs
And `auto doctor --json` carries the check `doctor.disk.free_space` with `status` `warn`, `severity` `warning`, and fields `free_bytes` `9663676416`, `floor_bytes` `48318382080`, `mount` `/System/Volumes/Data` in the first run, `status` `pass` and `severity` `info` in the second, and `data.overall_ok` identical to a run without the disk check in both.

### S9: AC-DISK-009 - Guidance surfaces carry the preflight and generated roots stay untouched
Priority: Must
Given the surfaces `content/skills/references/agent-pipeline/delegation.md`, `content/skills/references/agent-pipeline/verification.md`, `content/skills/worktree-isolation.md`, `templates/shared/supervisor-worktree-isolation.md.tmpl`, `templates/shared/omp-worktree-isolation.md.tmpl`, `templates/gemini/skills/worktree-isolation/SKILL.md.tmpl`, and `templates/gemini/skills/agent-pipeline/SKILL.md.tmpl`
When `TestDiskCheckSurfaceParity` runs after `make generate-templates`
Then the expected content of every listed file contains the token `auto disk check` and `delegation.md` no longer contains `Check free disk with` followed by a backquoted `df`
And `git status --porcelain` shows no path under `.claude/`, `.codex/`, `.gemini/`, `.opencode/`, `.omp/`, or `.agents/`.

### S10: AC-DISK-010 - The disk commands never rewrite autopus.yaml
Priority: Must
Given a project whose `autopus.yaml` lists the platform alias `claude`, which `config.Load` normalizes to `claude-code` and writes back to disk
When `auto disk status`, `auto disk check`, and a dry-run `auto disk trim` run against that project
Then the SHA-256 of `autopus.yaml` is identical before and after all three runs
And a control call to `config.Load` on a copy of the same file changes its SHA-256, proving the seam is real.

### S11: AC-DISK-011 - Live evidence on the dev Mac
Priority: Must
Given the dev Mac with its real `~/Library/Caches/go-build`, `$TMPDIR` work dirs, and the A1 scratch cache
When `auto disk status --json` and `df -k .` run within the same second, then `auto disk check --need 30`, then `auto disk trim`, then `GOCACHE=<scratch>/gocache auto disk trim --apply --ci idle` after backdating every scratch entry by 130 minutes, then `go build` in the scratch module
Then the expected `free_bytes` is `Available * 1024` within 1 MiB, the cache root is the real `go-build` directory, `go_build_work_dirs.dirs` equals the count of `$TMPDIR/go-build*` directories at that moment, the dry run changes the file count under the real cache by zero, the scratch apply deletes exactly the entries over 1 MiB and reports `failed=0`, and the subsequent `go build` exits 0 by rebuilding the missing entries
And the trace is recorded in `research.md`.

## Oracle Acceptance Notes

Every Must scenario is oracle-first: it pairs concrete inputs with the concrete expected output or an explicit tolerance rather than asserting that a file exists or that a command exits successfully.

- S1 fixes exact default values and exact error substrings per invalid key (INV-D09).
- S2 and S11 fix the measurement against `df` with a 1 MiB explicit tolerance and the mount identity (INV-D01).
- S3 is a ten-row exit-code and message table over heterogeneous flags, floors, and reserves (INV-D02).
- S4 and S5 fix byte totals, candidate sets, the full expected stdout of a dry run, the post-apply file set, and the counter line per race hook (INV-D03, INV-D04, INV-D05, INV-D12).
- S6 fixes detection per lister row including the false-positive shape found by probe A3 (INV-D06).
- S7 fixes classification and sizes for three heterogeneous worktrees plus the main worktree and asserts byte-identical repository state (INV-D07).
- S8 fixes the exact row text and JSON fields (INV-D10); S9 fixes token presence per surface and the generated-root exclusion (INV-D11); S10 fixes a SHA-256 equality with a control (INV-D08).

Tolerance statement: byte totals allow one 4096-byte block per fixture file; free-space comparisons allow 1 MiB; everything else (exit codes, message lines, candidate sets, counters, JSON field values) is exact.
