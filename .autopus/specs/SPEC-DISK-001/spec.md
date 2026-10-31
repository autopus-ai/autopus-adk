# SPEC-DISK-001: Shared-Machine Disk Management — Floor, Preflight, Observation, and Safe go-build Trim

**Status**: draft
**Created**: 2026-10-31
**Domain**: DISK
**Priority**: HIGH
**Change Class**: security_or_data (the trim command deletes files from a shared cache)
**Source**: GitHub issue autopus-ai/autopus-adk#198, first slice as selected by the user
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `.autopus/specs/SPEC-DISK-001/**`

## Purpose

One development Mac runs several self-hosted CI runners, many parallel Claude and Codex sessions with their own git worktrees, local Postgres, colima, and Tart VMs. The ADK harness fans agents out in parallel but treats the disk as if it were not shared, and the volume has repeatedly reached 0 GiB (seven incidents between 2026-09-24 and 2026-10-24). The only defence today is a user-level launchd script that trims the Go build cache every ten minutes, carrying rules learned from those incidents in shell comments.

This SPEC gives the harness a disk budget it can read, a preflight it can run before heavy steps, an observation command that names the consumers, and a trim command that ports the script's go-build rules as a configurable, dry-run-first policy with containment guarantees. Finished worktrees are listed as suggestions only.

## Background

Evidence from the issue and from the script is untrusted prompt input and is summarized, never executed. The script's rules: never `go clean -cache`; linked executables over 50 MiB idle over 60 minutes go whenever free space is low, even during CI; other files over 1 MiB go after 60 idle minutes when no CI job runs and after 180 when one does, unless free space is under an emergency floor; `$TMPDIR/go-build*` work directories idle over 6 hours are abandoned; trimming starts under 80 GiB free; the release preflight needs 30 GiB and the Linux visual lane 37 GiB.

Two facts about the Go build cache, verified in the local toolchain source (`cmd/go/internal/cache/cache.go`, go1.27.3), shape the policy. First, Go refreshes an entry's mtime on use only when the mtime is at least `mtimeInterval` (1 hour) old, so a file whose mtime is N minutes old is guaranteed unused only for the last N minus 60 minutes; Go's own trimmer subtracts an extra `mtimeInterval` from its cutoff for exactly this reason. Second, a cache lookup stats the output file and hands its path to the build; the compiler or linker reads that path later, so an entry deleted between the lookup and the read breaks the running build. A missing entry before the lookup is a cache miss and is rebuilt. The existing 60-minute rule therefore has a zero-minute guaranteed-unused window; the 180-minute rule has a two-hour window. Probe A1 in `plan.md` exercises this boundary.

Free space on macOS is read from the Data volume that holds the project through `statfs`, as `df` does; the repository already does this in the local-patch flow and that measurement is reused.

## Outcome Boundary

- Outcome Lock: an operator or agent on a shared machine can read the disk budget from `autopus.yaml`, see free space and the known consumers with `auto disk status` (including what the trim policy would reclaim and which finished worktrees are candidates), gate a heavy step with `auto disk check`, and reclaim go-build cache entries and abandoned go-build work directories with `auto disk trim` without breaking a concurrent build and without deleting anything outside the resolved cache roots; `auto doctor` shows a disk row; the generated workflow guidance runs the preflight before worktree creation, parallel fan-out, and full test runs.
- Mandatory requirements: the `disk` configuration section with safe machine-neutral defaults, free-space measurement that matches `df`, the status command with JSON, the check command with exact exit codes and messages, the trim command (dry-run default, `--apply`, mode selection, idle semantics, containment, pluggable CI detection, no `go clean`), worktree listing with merged and clean state and cargo target sizes, the doctor row, and the guidance wiring.
- Explicit non-goals: automatic reclamation of worktrees, scratchpads, scratch databases, VM images, model caches, colima, or other projects' worktrees; a consumer registry; disk tokens or build-concurrency limits for parallel agents; installing or editing a launchd job or the user's `trim.sh`; the msgmcp temp-leak rule; Windows free-space support; editing any generated root.
- Completion evidence: focused Go tests for config, measurement, policy, containment, CI detection, exit codes, worktree classification, doctor projection, and surface parity; the fixture oracles in `acceptance.md`; `go run ./cmd/source-lines --max 300 --ext .go` clean on the new files; and the live evidence on the dev Mac recorded in `research.md`.

## Definitions

- GiB: 1073741824 bytes. Every `*_gb` configuration value is in GiB, matching `df -g`; every `*_mb` value is in MiB (1048576 bytes). Human output prints one decimal.
- Floor: `disk.min_free_gb`. Required free space of a check: the larger of the floor and the requested need or named reserve.
- Cache root: the resolved Go build cache directory. Cache entry: a regular file `<root>/<xx>/<64 hex>-d` where `xx` is two lowercase hex digits; the `-a` action files and the `README`, `trim.txt`, and `log.txt` files are never entries.
- Work dir root: `$GOTMPDIR` when set, otherwise `os.TempDir()`. Work dir: a directory, not a symlink, directly under the work dir root whose name matches `^go-build[0-9]+$`.
- Idle: now minus the entry's `Lstat` mtime. A threshold of N minutes selects entries whose idle time is strictly greater than N minutes.
- CI running: at least one process whose executable basename equals one of `disk.trim.ci_process_names`.
- Emergency: free space below the floor. Trim mode: `emergency`, `ci`, or `idle`, selected in that order.
- Allocated bytes: `st_blocks * 512` when the platform exposes it, otherwise the file size.
- Scratchpad root: `EvalSymlinks("/tmp")/claude-<uid>` for the current user.
- Harness worktree: a linked worktree whose path lies under `.autopus/worktrees/`, `.claude/worktrees/`, or `.worktrees/` of the main worktree.

## Requirements

### Scope And Source Ownership

**REQ-DISK-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL make all changes inside `autopus-adk/` source-of-truth paths (`pkg/`, `internal/`, `content/`, `templates/`, `docs/`), SHALL NOT edit the generated roots `.claude/**`, `.codex/**`, `.gemini/**`, `.opencode/**`, `.omp/**`, or `.agents/**`, and SHALL NOT read, write, install, or remove the user's launchd job or `trim.sh`.

### Configuration

**REQ-DISK-CFG-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL read an optional `disk` section of `autopus.yaml` with the keys `min_free_gb`, `reserves` (a map of reserve name to GiB), and `trim` holding `start_below_gb`, `ci_process_names`, `go_build` (`file_mb`, `idle_minutes`, `ci_idle_minutes`, `emergency_idle_minutes`, `large_file_mb`, `large_file_idle_minutes`, `large_file_below_gb`), and `go_build_work_dirs` (`idle_minutes`), SHALL resolve an absent key to the defaults 10, empty, 0, `["Runner.Worker"]`, 1, 120, 180, 60, 50, 60, 0, and 360 respectively, SHALL mark every key `omitempty` so a configuration at defaults writes no `disk` key, and SHALL expose the resolved values through one accessor rather than letting callers read raw fields.

**REQ-DISK-CFG-02**
Priority: Must
Type: Unwanted
IF `min_free_gb`, `start_below_gb`, or `large_file_below_gb` is negative, a reserve value is not a positive integer, a reserve name does not match `^[a-z][a-z0-9_-]{0,31}$`, `file_mb` or `large_file_mb` is below 1, any `*idle_minutes` value is below 60 (Go's one-hour mtime refresh interval), or a `ci_process_names` entry is empty or contains a path separator, THEN THE SYSTEM SHALL fail configuration load with an error naming the dotted key and the bound, and SHALL leave an unknown nested key to the existing strict decoder.

**REQ-DISK-CFG-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL load configuration for `auto disk status`, `auto disk check`, and `auto disk trim` through `config.LoadPreview`, and SHALL NOT write `autopus.yaml` from any of them.

### Free Space

**REQ-DISK-FREE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL measure free space as `statfs` `Bavail` times the block unit of the file system holding the project directory (`Bsize` on darwin; `Frsize`, or `Bsize` when `Frsize` is zero, on linux), SHALL report the mount point of that file system (`Mntonname` on darwin; the longest matching mount point from `/proc/self/mounts` on linux), and SHALL report the same value `df -k` prints for the same path within 1 MiB at the same instant.

**REQ-DISK-FREE-02**
Priority: Must
Type: Unwanted
IF the operating system is neither darwin nor linux, THEN THE SYSTEM SHALL exit 1 from `auto disk status`, `auto disk check`, and `auto disk trim` with the single stderr line `disk: free space measurement is not supported on <GOOS>`, and the doctor row SHALL print a WARN line with the same text instead of a measurement.

### Status

**REQ-DISK-STATUS-01**
Priority: Must
Type: Event-driven
WHEN `auto disk status` runs, THE SYSTEM SHALL print free space, mount point, the floor verdict, and one row per consumer kind (`go_build_cache`, `go_build_work_dirs`, `worktrees`, `scratchpads`) giving allocated bytes, item counts, and an availability reason when the consumer cannot be read, followed by the trim preview that `auto disk trim` would produce now, and with `--json` SHALL emit the same data through the shared JSON envelope with sizes in bytes and the preview under `trim_preview`.

**REQ-DISK-STATUS-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL resolve the cache root as `$GOCACHE` when set, otherwise the output of `go env GOCACHE` with a 5-second timeout, otherwise `os.UserCacheDir()/go-build`, SHALL canonicalize it with `EvalSymlinks`, SHALL treat the value `off`, a relative path, or a missing directory as unavailable with a reason, and SHALL treat an existing directory without a regular `README` file as `not_a_go_build_cache`.

**REQ-DISK-STATUS-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL compute consumer sizes by an `Lstat` walk that never follows symlinks and never crosses onto another device than the root, summing allocated bytes, SHALL count work dirs and the subset idle beyond `go_build_work_dirs.idle_minutes`, and SHALL report the scratchpad root with directory count, bytes, and the oldest and newest mtime without deleting anything there.

### Check

**REQ-DISK-CHECK-01**
Priority: Must
Type: Event-driven
WHEN `auto disk check` runs with no flag, with `--need <GiB>`, or with `--for <reserve>`, THE SYSTEM SHALL compute the required free space as the larger of the floor and the need or reserve, SHALL label the winning source `disk.min_free_gb`, `--need`, or `disk.reserves.<name>`, and when free space is at or above the requirement SHALL print `disk check: ok: <free> GiB free, <required> GiB required (<source>) on <mount>` to stdout and exit 0.

**REQ-DISK-CHECK-02**
Priority: Must
Type: Unwanted
IF free space is below the requirement, THEN THE SYSTEM SHALL print `disk check: insufficient free space: <free> GiB free, <required> GiB required (<source>) on <mount>; free <deficit> GiB or lower the requirement` to stderr and exit 3, in text and JSON modes alike, so callers can tell insufficiency from a measurement failure.

**REQ-DISK-CHECK-03**
Priority: Must
Type: Unwanted
IF `--for` names a reserve absent from `disk.reserves`, `--need` is not a positive number, or both flags are given, THEN THE SYSTEM SHALL exit 1 with, respectively, `disk check: reserve "<name>" is not configured under disk.reserves in autopus.yaml`, `disk check: --need must be a positive number of GiB`, or `disk check: --need and --for are mutually exclusive`.

### Trim

**REQ-DISK-TRIM-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL run `auto disk trim` as a dry run unless `--apply` is given, SHALL print the header `disk trim: mode=<dry-run|apply> free=<GiB> GiB ci=<running|idle|forced-running|forced-idle|unknown> emergency=<yes|no> rules=<active rule list>`, then one line per rule with candidate count and bytes, then `disk trim: would reclaim <size> (dry-run; pass --apply to delete)` or `disk trim: reclaimed <size> (deleted=<n> skipped_refreshed=<n> skipped_symlink=<n> vanished=<n> failed=<n>)`, and with `--list` SHALL print every candidate path relative to its root.

**REQ-DISK-TRIM-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL select the trim mode as `emergency` when free space is below the floor, otherwise `ci` when CI is running, otherwise `idle`, SHALL apply the go-build file rule with `emergency_idle_minutes`, `ci_idle_minutes`, or `idle_minutes` accordingly over entries larger than `file_mb`, SHALL apply the large-file rule (`large_file_mb`, `large_file_idle_minutes`) in every mode whenever `large_file_below_gb` is 0 or free space is below it, SHALL apply the work dir rule in every mode, and SHALL evaluate no rule when `start_below_gb` is positive and free space is at or above it, printing `disk trim: inactive (free above start_below_gb)`.

**REQ-DISK-TRIM-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL consider as go-build candidates only regular files at `<root>/<xx>/<64 hex>-d`, SHALL compute idle from the `Lstat` mtime, and SHALL state in `--help` and in a note line of the dry-run output that an idle threshold of N minutes guarantees only N minus 60 minutes without use, because Go refreshes a reused entry's mtime at most hourly.

**REQ-DISK-TRIM-04**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL consider as work dir candidates only directories, not symlinks, directly under the work dir root whose name matches `^go-build[0-9]+$` and whose mtime is idle beyond `go_build_work_dirs.idle_minutes`, and SHALL leave every other entry of the work dir root untouched.

**REQ-DISK-TRIM-05**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL delete only through an `os.Root` opened on the canonical cache root or work dir root, SHALL `Lstat` each candidate again immediately before removal and skip it as `skipped_refreshed` when its mtime or size changed since the scan or as `skipped_symlink` when it is not a regular file or directory, SHALL count an entry that vanished before removal as `vanished` rather than a failure, and SHALL never follow a symlink, never delete `README`, `trim.txt`, `log.txt`, or any `-a` file, and never remove a path outside the root.

**REQ-DISK-TRIM-06**
Priority: Must
Type: Unwanted
IF the cache root does not exist, THEN THE SYSTEM SHALL report the consumer unavailable and exit 0; IF the cache root exists but lacks the `README` marker or resolves to `/`, the user's home directory, or the project directory, THEN THE SYSTEM SHALL delete nothing, print `disk trim: refused: <root> is not a Go build cache (<reason>)`, and exit 1.

**REQ-DISK-TRIM-07**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL NOT run `go clean` in any form, and SHALL run no `go` subprocess other than `go env GOCACHE`.

**REQ-DISK-TRIM-08**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL detect CI as running when the executable basename of any process equals a configured name exactly, using `ps -axo comm=` on darwin and `/proc/<pid>/comm` with `/proc/<pid>/exe` on linux through an injectable process lister, SHALL NOT match substrings of full command lines, SHALL honour `--ci running` and `--ci idle` as overrides reported as `forced-running` and `forced-idle`, and SHALL treat a lister failure as running with the warning line `disk trim: ci detection unavailable (<reason>); applying the CI rule set`.

**REQ-DISK-TRIM-09**
Priority: Must
Type: Event-driven
WHEN `--apply` completes, THE SYSTEM SHALL exit 1 when `failed` is greater than zero and 0 otherwise, SHALL print the per-rule and total counters of REQ-DISK-TRIM-01, and with `--json` SHALL emit the same counters per rule plus `mode`, `free_bytes`, `mount`, `ci`, `emergency`, `active`, and `total_bytes`, adding `files` only with `--list`.

### Worktrees

**REQ-DISK-WT-01**
Priority: Must
Type: Event-driven
WHEN the project directory is inside a git repository, THE SYSTEM SHALL enumerate `git --no-optional-locks worktree list --porcelain`, SHALL mark the main worktree `main`, and for every linked worktree SHALL report path, branch, `clean` (empty `git status --porcelain=v1 -z --untracked-files=normal`), `merged` (`git merge-base --is-ancestor HEAD <base>` where base is `origin/HEAD`, then `origin/main`, `main`, `master`, else `unknown`), `prunable` and `locked` flags, allocated bytes, `cargo_target_bytes` summed over directories named `target` whose parent holds `Cargo.toml` within depth 6, and the `harness` flag.

**REQ-DISK-WT-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL NOT remove, prune, or modify any worktree, branch, or `target` directory, SHALL print the suggestion `candidate: git worktree remove <path> (merged into <base>, clean, <size>)` only for a linked worktree that is merged and clean, and SHALL report the consumer unavailable as `not_a_git_repository` when the project is not in a git repository.

### Doctor

**REQ-DISK-DOC-01**
Priority: Must
Type: Event-driven
WHEN `auto doctor` runs, THE SYSTEM SHALL print a `Disk` section with `[OK] disk: <free> GiB free on <mount> (floor <floor> GiB)` at or above the floor and `[WARN] disk: <free> GiB free on <mount> is below the <floor> GiB floor (disk.min_free_gb); run 'auto disk status'` below it, SHALL keep the row advisory so the overall verdict is unchanged, and in JSON SHALL add the check `doctor.disk.free_space` with status `pass` or `warn`, severity `info` or `warning`, and fields `free_bytes`, `floor_bytes`, and `mount`, without changing the envelope status.

### Guidance

**REQ-DISK-GUIDE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL replace the `df` plus 5 GiB worker-hygiene bullet in `content/skills/references/agent-pipeline/delegation.md` with `auto disk check --need <GiB>` guidance that stops on exit 3, SHALL add the same preflight before the full-suite run in `content/skills/references/agent-pipeline/verification.md`, before `git worktree add` in `templates/shared/supervisor-worktree-isolation.md.tmpl`, and before parallel fan-out in `content/skills/worktree-isolation.md` and `templates/shared/omp-worktree-isolation.md.tmpl`, SHALL regenerate the gemini and codex mirrors with `make generate-templates`, and SHALL pin the token `auto disk check` on every one of those surfaces with a parity test.

### Verification

**REQ-DISK-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep every new or changed source file, tests included, at or under 300 comment-aware lines, SHALL cover REQ-DISK-CFG-01 through REQ-DISK-GUIDE-01 with the oracle scenarios of `acceptance.md`, and SHALL record live evidence from the dev Mac for status, check, and a dry-run trim, with `--apply` exercised only against a scratch `GOCACHE`.

## Generated And Changed Surfaces

| Surface | Role | Change |
|---------|------|--------|
| `[NEW] pkg/config/schema_disk.go`, `[NEW] pkg/config/schema_disk_test.go` | `disk` section, defaults, validation, accessor | new files; `HarnessConfig` in `pkg/config/schema.go` gains `Disk DiskConf` with `omitempty` |
| `[NEW] pkg/diskspace/free_darwin.go`, `free_linux.go`, `free_other.go`, `format.go` | free space, mount, GiB formatting | new package |
| `[NEW] pkg/diskspace/gobuild.go`, `workdirs.go`, `scratchpads.go`, `worktrees.go`, `sizes.go` | consumer scans | new files |
| `[NEW] pkg/diskspace/policy.go`, `ci.go`, `apply.go` | mode selection, candidates, CI lister, contained deletion | new files |
| `[NEW] internal/cli/disk.go`, `disk_status.go`, `disk_check.go`, `disk_trim.go` | `auto disk` namespace, text and JSON output, exit codes | new files; `root.go` registers `newDiskCmd()` |
| `[NEW] internal/cli/doctor_disk.go` | doctor text row and JSON check | new file; `doctor_text.go` and `doctor_json.go` call it |
| `content/skills/references/agent-pipeline/delegation.md`, `verification.md`, `content/skills/worktree-isolation.md`, `templates/shared/supervisor-worktree-isolation.md.tmpl`, `templates/shared/omp-worktree-isolation.md.tmpl` | guidance source of truth | preflight wording; gemini and codex mirrors regenerated |
| `[NEW] templates/disk_check_surface_test.go` | surface parity oracle | new test |
| `[NEW] docs/disk-management.md`, `README.md` | operator documentation | new doc; one row in the `All Commands` table |

## Related SPECs

None as siblings. Reused contracts: the JSON envelope of `SPEC-CLIJSON-001` (`internal/cli/output_json.go`), the `statfs` measurement precedent of `SPEC-SIGMABAND-002` (`internal/cli/react_band_localpatch_disk_darwin.go`), and the worktree lifecycle guidance of `SPEC-WORKTREE-001` (`templates/shared/supervisor-worktree-isolation.md.tmpl`), none of which is reopened.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-DISK-SCOPE-01 | T9, T10 | S9 | INV-D11 |
| REQ-DISK-CFG-01 | T1 | S1 | INV-D09 |
| REQ-DISK-CFG-02 | T1 | S1 | INV-D09 |
| REQ-DISK-CFG-03 | T7 | S10 | INV-D08 |
| REQ-DISK-FREE-01 | T2 | S2, S11 | INV-D01 |
| REQ-DISK-FREE-02 | T2, T7 | S2 | INV-D01 |
| REQ-DISK-STATUS-01 | T3, T7 | S4, S11 | INV-D03, INV-D07 |
| REQ-DISK-STATUS-02 | T3 | S4, S5 | INV-D05 |
| REQ-DISK-STATUS-03 | T3 | S4 | INV-D07 |
| REQ-DISK-CHECK-01 | T7 | S3 | INV-D02 |
| REQ-DISK-CHECK-02 | T7 | S3 | INV-D02 |
| REQ-DISK-CHECK-03 | T7 | S3 | INV-D02 |
| REQ-DISK-TRIM-01 | T5, T7 | S5 | INV-D03 |
| REQ-DISK-TRIM-02 | T5 | S5 | INV-D04 |
| REQ-DISK-TRIM-03 | T5 | S5 | INV-D03 |
| REQ-DISK-TRIM-04 | T5, T6 | S5 | INV-D03 |
| REQ-DISK-TRIM-05 | T6 | S5 | INV-D05 |
| REQ-DISK-TRIM-06 | T3, T6 | S5 | INV-D05 |
| REQ-DISK-TRIM-07 | T6 | S5 | INV-D12 |
| REQ-DISK-TRIM-08 | T5 | S6 | INV-D06 |
| REQ-DISK-TRIM-09 | T6, T7 | S5 | INV-D03, INV-D05 |
| REQ-DISK-WT-01 | T4 | S7 | INV-D07 |
| REQ-DISK-WT-02 | T4 | S7 | INV-D07 |
| REQ-DISK-DOC-01 | T8 | S8 | INV-D10 |
| REQ-DISK-GUIDE-01 | T9 | S9 | INV-D11 |
| REQ-DISK-VERIFY-01 | T11 | S11 | INV-D01, INV-D03 |
