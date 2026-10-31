# SPEC-DISK-001 Acceptance Criteria

Units: GiB is 1073741824 bytes, MiB is 1048576 bytes; human sizes print one decimal. Free space, CI state, the clock, the uid, and the scratchpad root enter the CLI through the REQ-DISK-STATUS-04 seams; tests use only paths they own and never create `/tmp/claude-<uid>`. Fixture files are written fully (non-sparse), so allocated bytes equal apparent size; the explicit tolerance on every fixture byte total is one 4096-byte block per file. `<hN>` is a 64-character lowercase hex name whose first two characters equal its subdirectory. Defaults apply unless a scenario states otherwise.

## Test Scenarios

### S1: AC-DISK-001 - Configuration defaults, validation, and the below-120 warning are exact
Priority: Must
Given an `autopus.yaml` without a `disk` key
When the configuration is loaded and `Disk.Effective()` is read
Then the expected values are floor 10737418240 bytes, no reserves, `start_below_gb` 0, `ci_process_names` `["Runner.Worker"]`, `file_mb` 1, `idle_minutes` 120, `ci_idle_minutes` 180, `emergency_idle_minutes` 120, `large_file_mb` 50, `large_file_idle_minutes` 120, `large_file_below_gb` 0, work dir `idle_minutes` 360, and saving writes no `disk` key
And each invalid file below fails to load with an error containing the listed text

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

And `trim: {go_build: {idle_minutes: 60, large_file_idle_minutes: 90}}` loads, and both `auto disk status` and `auto disk trim` print exactly `disk trim: warning: disk.trim.go_build.idle_minutes=60 is below 120 and may delete an entry a running build has already looked up` and `disk trim: warning: disk.trim.go_build.large_file_idle_minutes=90 is below 120 and may delete an entry a running build has already looked up`, while `--json` carries two envelope warnings with code `idle_threshold_below_120`.

### S2: AC-DISK-002 - Free space arithmetic and mount identity are exact
Priority: Must
Given a `Statfs_t` fixture with `Bavail` 37844 and `Bsize` 4096 (darwin) and a linux fixture with `Bavail` 37844, `Frsize` 1024, `Bsize` 4096
When the measurement converts them
Then the expected values are 155009024 bytes for darwin and 38752256 bytes for linux, and a linux fixture with `Frsize` 0 yields 155009024
And on the dev Mac `diskspace.Free` of the project path, `/tmp`, and `$TMPDIR` all report mount `/System/Volumes/Data`, and of three `Free` versus `df -k` sample pairs taken 1 s apart with their timestamps recorded, at least one pair agrees within an explicit tolerance of 1 MiB
And with the free-space seam returning `ErrUnsupported` for `plan9`, each of `auto disk status`, `auto disk check`, and `auto disk trim` exits 1 with the single stderr line `disk: free space measurement is not supported on plan9`.

### S3: AC-DISK-003 - Check exit codes, streams, and messages follow the table
Priority: Must
Given floor 45 GiB, reserves `{release: 30, nightly-full: 60}`, mount `/System/Volumes/Data`, and the seam values below
When `auto disk check` runs through `runRootCommand` with the listed flags
Then the exit code and the exact expected line are as listed, the other stream is empty, and no line starts with `Error: `

| flags | seam | exit | stream | expected line |
|-------|------|------|--------|---------------|
| none | free 9.0 GiB | 3 | stderr | `disk check: insufficient free space: 9.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 36.0 GiB or lower the requirement` |
| none | free 45.0 GiB | 0 | stdout | `disk check: ok: 45.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data` |
| `--need 30` | free 36.0 GiB | 3 | stderr | `disk check: insufficient free space: 36.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 9.0 GiB or lower the requirement` |
| `--need 50` | free 52.5 GiB | 0 | stdout | `disk check: ok: 52.5 GiB free, 50.0 GiB required (--need) on /System/Volumes/Data` |
| `--for release` | free 31.0 GiB | 3 | stderr | `disk check: insufficient free space: 31.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 14.0 GiB or lower the requirement` |
| `--for nightly-full` | free 61.0 GiB | 0 | stdout | `disk check: ok: 61.0 GiB free, 60.0 GiB required (disk.reserves.nightly-full) on /System/Volumes/Data` |
| `--for nightly-full` | free 59.9 GiB | 3 | stderr | `disk check: insufficient free space: 59.9 GiB free, 60.0 GiB required (disk.reserves.nightly-full) on /System/Volumes/Data; free 0.1 GiB or lower the requirement` |
| `--for weekly` | free 200.0 GiB | 1 | stderr | `disk check: reserve "weekly" is not configured under disk.reserves in autopus.yaml` |
| `--need 0` | free 200.0 GiB | 1 | stderr | `disk check: --need must be a positive number of GiB` |
| `--need 30 --for release` | free 200.0 GiB | 1 | stderr | `disk check: --need and --for are mutually exclusive` |
| none | error `statfs: permission denied` | 1 | stderr | `disk check: free space measurement failed: statfs: permission denied` |

And with `--json` the exit codes are identical, stderr is empty in every row, the envelope `status` is `ok` for exit 0 and `error` otherwise with `error.code` `disk_insufficient`, `disk_check_invalid`, or `disk_measurement_failed` by row, and `data` carries `free_bytes`, `required_bytes`, `source`, and `mount`.

### S4: AC-DISK-004 - Status reports every consumer with exact bytes from fixtures only
Priority: Must
Given the S5 cache root as `$GOCACHE`, the S5 work dir root as `$GOTMPDIR`, the S7 repository as the project, a scratchpad root fixture under the test's temp dir holding session directories of 1 MiB (mtime 4320 min before the injected clock) and 3 MiB (60 min), free 100 GiB, floor 10 GiB, and `--ci idle`
When `auto disk status --json` runs
Then the expected JSON `data.consumers` holds exactly four kinds: `go_build_cache` with `entries` 8 and `bytes` 324546560 (h1, h2, h3, h4, h5, the three executable inner files, the `-a` file, `stray.bin`, `README`, and `trim.txt` as allocated blocks; symlinks excluded), `go_build_work_dirs` with `dirs` 3, `idle_dirs` 1, and `bytes` 4296704, `worktrees` with the S7 items, and `scratchpads` with `dirs` 2, `bytes` 4194304, `oldest_age_minutes` 4320, and `newest_age_minutes` 60, each byte total within the stated tolerance
And `data.free_bytes` is 107374182400, `data.floor.ok` is true, `data.trim_preview.run` is `dry-run`, and `data.trim_preview.total_bytes` is 131072000
And the real `/tmp/claude-<uid>` directory is neither read nor created during the test
And with the project outside any git repository the `worktrees` consumer reports `available` false with `reason` `not_a_git_repository`; with `GOCACHE=off` the cache consumer reports `reason` `gocache_off`; with `GOCACHE` pointing at a directory without `README` it reports `not_a_go_build_cache`.

### S5: AC-DISK-005 - Trim selects, attributes, previews, and deletes exactly the policy set
Priority: Must
Given a cache root fixture `R` holding `README` and `trim.txt` of 1 KiB each plus the entries below, and a work dir root fixture `W` holding `go-build111/` (one 3 MiB file, every mtime 400 min), `go-build222/` (one 1 MiB file `b001/x.o` with mtime 10 min under a directory with mtime 400 min), `go-build333` (a symlink to a directory outside `W` holding a 2 MiB file), `go-build555/` (one 100 KiB file, 100 min), `msgmcp-railway-roles1/` (400 min), and `notes/` (400 min), all owned by the injected uid

| entry under R | kind | size | own mtime age |
|---------------|------|------|---------------|
| `aa/<h1>-d` | regular file | 2 MiB | 130 min |
| `aa/<h2>-d` | regular file | 2 MiB | 90 min |
| `bb/<h3>-d` | regular file | 60 MiB | 130 min |
| `bb/<h4>-d` | regular file | 60 MiB | 100 min |
| `cc/<h5>-d` | regular file | 512 KiB | 500 min |
| `cc/<h6>-a` | action file | 100 B | 500 min |
| `dd/<h7>-d` | symlink to `outside/big.bin` (2 MiB, 500 min) | - | 500 min |
| `ee/stray.bin` | non-entry name | 5 MiB | 500 min |
| `ff/<h8>-d/probe.test` | executable entry: directory holding a 60 MiB file with inner mtime 500 min | 60 MiB | directory 130 min |
| `ff/<h9>-d/probe.test` | executable entry as above | 60 MiB | directory 30 min |
| `ff/<h10>-d/` | directory holding `probe.test` 60 MiB and a symlink `link` to `outside/big.bin` | 60 MiB | directory 130 min |

When `auto disk trim` runs with floor 10 GiB and the inputs below
Then the expected stdout of the idle dry run (free 100 GiB, `--ci idle`) is exactly

```
disk trim: run=dry-run mode=idle free=100.0 GiB ci=forced-idle rules=go_build_large_files(>50 MiB, idle>120m) go_build_files(>1 MiB, idle>120m) go_build_work_dirs(idle>360m)
  roots: cache=<R> work=<W>
  note: idle>120m guarantees 60m without use; mtime trimming bounds the risk to a running build, it does not remove it
  go_build_large_files: candidates=2 size=120.0 MiB
  go_build_files: candidates=1 size=2.0 MiB
  go_build_work_dirs: candidates=1 size=3.0 MiB
  skipped_symlink=3 (go_build_cache=2, go_build_work_dirs=1)
disk trim: would reclaim 125.0 MiB (dry-run; pass --apply to delete)
```

And the attributed candidate sets are: idle (free 100 GiB, `--ci idle`) large h3 and h8, files h1, work dirs `go-build111`, total 125.0 MiB, with h3 counted under the large rule only although it also satisfies the files rule; ci (free 100 GiB, `--ci running`) large h3 and h8, files none, work dirs `go-build111`, total 123.0 MiB, header `mode=ci ci=forced-running` and `go_build_files(>1 MiB, idle>180m)`; emergency (free 5 GiB, `--ci running`) the idle sets with header `mode=emergency`, total 125.0 MiB; emergency with `emergency_idle_minutes: 60` configured adds h2 and h4 to the files rule (h4, 60 MiB idle 100m, is not taken by the large rule at idle>120m, so first-match attribution gives it to the files rule), total 187.0 MiB, and prints the S1 warning line for that key
And `--list` prints `bb/<h3>-d` and `ff/<h8>-d` under the large rule line, `aa/<h1>-d` under the files line, and `go-build111` under the work dir line, and `--json` carries the same values as `rules[].candidates`, `rules[].bytes`, `rules[].files`, `skipped.symlink.go_build_cache` 2, `skipped.symlink.go_build_work_dirs` 1, and `total_bytes` 131072000
And `--apply` in the idle run deletes exactly h1, h3, h8 (directory and inner file), and `go-build111`, leaves every other path of both roots and `outside/` byte-identical including h7, h9, h10 with its symlink, and `go-build333` with its target, prints `disk trim: reclaimed 125.0 MiB (deleted=4 skipped_changed=0 skipped_symlink=3 skipped_owner=0 vanished=0 failed=0)`, and exits 0
And with a hook that touches h1 between scan and removal the summary reads `deleted=3 skipped_changed=1 skipped_symlink=3 skipped_device=0 skipped_owner=0 vanished=0 failed=0` and h1 survives; with a hook that unlinks h3 it reads `deleted=3 skipped_changed=0 skipped_symlink=3 skipped_device=0 skipped_owner=0 vanished=1 failed=0` and exits 0; with a hook that replaces `go-build111/b001` by a directory on another device it reads `failed=1` with `deleted=3`, prints `disk trim: 1 candidate could not be removed`, and exits 1; with the injected uid differing from the fixture owner every candidate is `skipped_owner` and `R` is refused as `(not owned by uid <n>)` before any deletion
And with `README` removed from `R` both runs print `disk trim: refused: <R> is not an acceptable cache root (no README marker)`, delete nothing in `R` or `W`, and exit 1; with `$GOTMPDIR` set to the home directory both runs print `disk trim: refused: <home> is not an acceptable work dir root (home directory)`, delete nothing, and exit 1; with `R` missing the cache row reads `go_build_cache: unavailable (root_missing)` and the dry run exits 0
And the exec recorder sees zero `go` invocations when `$GOCACHE` is set, exactly one `go env GOCACHE` when it is unset, and no `clean` argument in any run.

### S6: AC-DISK-006 - CI detection matches executable basenames only and unknown stays conservative
Priority: Must
Given `ci_process_names` `["Runner.Worker"]`, floor 10 GiB, and an injected process lister
When the lister returns the rows below
Then the expected header fields are

| lister rows | free | `ci=` | `mode=` |
|-------------|------|-------|---------|
| `/opt/actions-runner/bin/Runner.Listener`, `/opt/actions-runner/bin/Runner.Worker`, `/bin/zsh` | 100 GiB | `running` | `ci` |
| `/opt/actions-runner/bin/Runner.Listener`, `/bin/zsh`, `/usr/bin/grep` | 100 GiB | `idle` | `idle` |
| `/opt/actions-runner/bin/Runner.Worker.old`, `/bin/zsh` | 100 GiB | `idle` | `idle` |
| lister returns an error | 100 GiB | `unknown` | `ci` |
| lister returns an error | 5 GiB | `unknown` | `emergency` |

And both error rows print `disk trim: ci detection unavailable (<reason>); treating CI as running`, and the 5 GiB row evaluates the files rule at `emergency_idle_minutes`
And `--ci idle` with the first row yields `ci=forced-idle mode=idle`, and `--ci running` with the second row yields `ci=forced-running mode=ci`
And on the dev Mac the darwin lister reports running while a `Runner.Worker` process exists and idle when only `Runner.Listener` processes exist.

### S7: AC-DISK-007 - Worktrees are classified, quoted, and never touched
Priority: Must
Given a temporary repository with branch `main`, a `.gitignore` containing `target/`, and linked worktrees: `wt-a` on `feat-a` merged into `main`, clean, holding `crate/Cargo.toml` and `crate/target/blob.bin` of 4 MiB; `wt-b` on `feat-b` with one unmerged commit, clean; `wt-c` at the `main` commit with a modified tracked file; `wt-d` whose directory was removed after creation; `wt-e` merged and clean but locked with `git worktree lock`; `wt-f` merged and clean at the path `<repo>/wt a$(b)'c`; `wt-g` merged and clean at a path containing a newline
When `auto disk status --json` runs from the main worktree with the other consumers unavailable
Then the expected JSON `worktrees.items` has eight entries: the main worktree with `main` true; `wt-a` with `clean` true, `merged` true, `base` `main`, `harness` false, `cargo_target_bytes` 4194304 within tolerance; `wt-b` with `merged` false and `clean` true; `wt-c` with `merged` true and `clean` false; `wt-d` with `prunable` true, `clean` `unknown`, `merged` `unknown`, `reason` `path_missing`; `wt-e` with `locked` true; `wt-f` and `wt-g` with `merged` true and `clean` true
And the text output contains exactly three suggestion lines: `candidate: git worktree remove -- '<repo>/wt-a' (merged into main, clean, 4.0 MiB)`, `candidate: git worktree remove -- '<repo>/wt a$(b)'\''c' (merged into main, clean, <size>)`, and for `wt-g` the line `suggestion withheld: path contains control characters` with the newline shown as `?` in its displayed path
And `git worktree list --porcelain -z` output and every file under every worktree are byte-identical before and after the run
And a worktree created under `.autopus/worktrees/` of the main worktree reports `harness` true.

### S8: AC-DISK-008 - The doctor row is advisory and mirrored in JSON
Priority: Must
Given floor 45 GiB and mount `/System/Volumes/Data`
When `auto doctor` runs with free 9.0 GiB, then with free 100.0 GiB, then with the seam returning `ErrUnsupported` for `plan9`
Then the expected text lines under a `Disk` section header are `[WARN] disk: 9.0 GiB free on /System/Volumes/Data is below the 45.0 GiB floor (disk.min_free_gb); run 'auto disk status'`, `[OK] disk: 100.0 GiB free on /System/Volumes/Data (floor 45.0 GiB)`, and `[WARN] disk: free space measurement is not supported on plan9`
And the result box verdict is unchanged by the disk row in all three runs
And `auto doctor --json` carries the check `doctor.disk.free_space` with `status` `warn`, `severity` `warning`, and fields `free_bytes` `9663676416`, `floor_bytes` `48318382080`, `mount` `/System/Volumes/Data` in the first run, `status` `pass` and `severity` `info` in the second, `status` `warn` with field `reason` `unsupported_os` in the third, and `data.overall_ok` identical to a run without the disk check in all three.

### S9: AC-DISK-009 - Every shipping guidance surface carries the preflight with exit semantics
Priority: Must
Given the surfaces `content/skills/references/agent-pipeline/delegation.md`, `content/skills/references/agent-pipeline/verification.md`, `content/skills/worktree-isolation.md`, `templates/shared/supervisor-worktree-isolation.md.tmpl`, `templates/gemini/skills/worktree-isolation/SKILL.md.tmpl`, `templates/shared/omp-worktree-isolation.md.tmpl`, and the string returned by `normalizeCodexExtendedSkill("worktree-isolation", body, cfg)`
When the parity tests run after `make generate-templates`
Then the expected content of every listed surface contains `auto disk check --need`, `exits 3`, and `exit 0`, and `delegation.md` does not contain `below 5 GiB`
And `RenderSkillResources("agent-pipeline", p, cfg)` for `p` in claude, codex, gemini, opencode, omp returns `references/delegation.md` containing `auto disk check --need`
And `git status --porcelain` shows no path under `.claude/`, `.codex/`, `.gemini/`, `.opencode/`, `.omp/`, or `.agents/`.

### S10: AC-DISK-010 - The disk commands never rewrite autopus.yaml
Priority: Must
Given a project whose `autopus.yaml` lists the platform alias `claude`, which `config.Load` normalizes to `claude-code` and writes back to disk
When `auto disk status`, `auto disk check`, and a dry-run `auto disk trim` run against that project
Then the SHA-256 of `autopus.yaml` is identical before and after all three runs
And a control call to `config.Load` on a copy of the same file changes its SHA-256.

### S11: AC-DISK-011 - Live evidence on the dev Mac under scratch roots
Priority: Must
Given one shell on the dev Mac with `export GOCACHE=<scratch>/gocache GOTMPDIR=<scratch>/gotmp TMPDIR=<scratch>/tmp` in force for every command below, the scratch cache populated by a `go build` of the A1 module, every scratch entry then backdated by 130 minutes, and the real `~/Library/Caches/go-build` and `$TMPDIR` of the login session left as they are
When `auto disk status --json`, `auto disk check --need 30`, `auto disk trim --list`, `auto disk trim --apply --ci idle`, and `go build` in the A1 module run in that order
Then the expected `roots:` line of both trim runs names only `<scratch>/gocache` and `<scratch>/gotmp`, the status `free_bytes` agrees with one of three `df -k` samples within 1 MiB, the dry-run candidate list is non-empty and every listed path still exists after the dry run, the apply deletes exactly the listed candidates and reports `failed=0`, every file of the real cache that was sampled before the sequence still exists afterwards, and the final `go build` exits 0 by rebuilding the missing entries from the scratch cache
And the trace with timestamps is recorded in `research.md`.

### S12: AC-DISK-012 - Gating thresholds and the large-file boundary attribute correctly
Priority: Must
Given the S5 fixtures with `--ci idle`
When `auto disk trim` runs with the configurations below
Then the expected results are

| configuration | free | result |
|---------------|------|--------|
| `start_below_gb: 80` | 100 GiB | only `disk trim: inactive (free above start_below_gb)`, exit 0, JSON `active` false |
| `start_below_gb: 80` | 79.9 GiB | the S5 idle result |
| `large_file_below_gb: 60` | 70 GiB | `rules=` omits the large rule; `go_build_files: candidates=3 size=122.0 MiB` (h1, h3, and h8, since with the large rule off both large entries fall to the files rule); total 125.0 MiB |
| `large_file_below_gb: 60` | 59 GiB | the S5 idle result with the large rule active |

## Oracle Acceptance Notes

Every Must scenario is oracle-first: it pairs concrete inputs with the concrete expected output, expected stdout, expected JSON values, or an explicit tolerance, never a bare file-exists or exit-success check.

- S1 fixes exact default values, exact error substrings, and the exact warning lines (INV-D09); S2 fixes the `statfs` arithmetic per field and the mount identity (INV-D01); S3 is an eleven-row exit-code, stream, and message table including the no-prefix contract and JSON codes (INV-D02).
- S4, S5, and S12 fix byte totals, attributed candidate sets, the full dry-run stdout, the post-apply file set, and the counter line per race hook, owner, and refusal case (INV-D03, INV-D04, INV-D05, INV-D12, INV-D13, INV-D14, INV-D15); S6 fixes detection and mode per lister row including the unknown-plus-emergency combination (INV-D06); S7 fixes classification, quoting, withheld suggestions, and unchanged repository state over eight heterogeneous worktrees (INV-D07); S8 fixes the row text and JSON fields including the unsupported-OS projection (INV-D10); S9 fixes token presence per shipping surface and the resource rendering (INV-D11); S10 fixes a SHA-256 equality with a control (INV-D08); S11 records live evidence under scratch roots with a candidate-existence oracle rather than file counts.

Tolerance statement: byte totals allow one 4096-byte block per fixture file; live free-space comparisons allow 1 MiB on at least one of three sample pairs; everything else (exit codes, message lines, candidate sets, counters, JSON field values) is exact.
