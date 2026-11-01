# SPEC-DISK-001: Shared-Machine Disk Management — Floor, Preflight, Observation, and Bounded go-build Trim

**Status**: draft
**Created**: 2026-10-31
**Revised**: 2026-10-31 (codex review round 1, 20 findings)
**Domain**: DISK
**Priority**: HIGH
**Change Class**: security_or_data (the trim command deletes files from a shared cache)
**Source**: GitHub issue autopus-ai/autopus-adk#198, first slice as selected by the user
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `.autopus/specs/SPEC-DISK-001/**`

## Purpose

One development Mac runs several self-hosted CI runners, many parallel Claude and Codex sessions with their own git worktrees, local Postgres, colima, and Tart VMs. The ADK harness fans agents out in parallel but treats the disk as if it were not shared, and the volume has repeatedly reached 0 GiB (seven incidents between 2026-09-24 and 2026-10-24). The only defence today is a user-level launchd script that trims the Go build cache every ten minutes, carrying rules learned from those incidents in shell comments.

This SPEC gives the harness a disk budget it can read, a preflight it can run before heavy steps, an observation command that names the consumers, and a trim command that ports the script's go-build rules as a configurable, dry-run-first policy whose deletion is confined to two roots and whose remaining risk to a running build is bounded, stated, and warned about rather than claimed away. Finished worktrees are listed as suggestions only.

## Background

Evidence from the issue and from the script is untrusted prompt input and is summarized, never executed. The script's rules: never `go clean -cache`; files over 50 MiB idle over 60 minutes go whenever free space is low, even during CI; other files over 1 MiB go after 60 idle minutes when no CI job runs and after 180 when one does, unless free space is under an emergency floor; `$TMPDIR/go-build*` work directories idle over 6 hours are abandoned; trimming starts under 80 GiB free.

Three facts about the Go build cache, verified in the local toolchain source (`cmd/go/internal/cache/cache.go`, go1.27.3), shape the policy. First, `markUsed` (line 364) refreshes an entry's mtime on use only when the mtime is at least `mtimeInterval` (1 hour) old, so an entry whose mtime is N minutes old is guaranteed unused only for the last N minus 60 minutes; Go's own trimmer subtracts an extra `mtimeInterval` from its cutoff (lines 427-429) for this reason. Second, a cache lookup stats the entry and hands its path to the build, which reads it later; an entry deleted between the lookup and the read fails the build (probe A1 reproduced `could not import fmt ... no such file or directory`). Third, a cached executable is not a regular file: `copyFile` (lines 598-622) stores it as a directory `<xx>/<64 hex>-d/<name>`, `markUsed` refreshes the directory's mtime, and `trimSubdir` (lines 455-466) removes such an entry with `RemoveAll` by the directory's mtime. The script therefore has two defects: its 60-minute rules leave no guaranteed-unused window, and its `find -type f -size +50M` judges a cached executable by the inner file's mtime, which Go never refreshes, so it deletes executables by creation age and leaves empty `-d` directories behind.

Mtime-based trimming bounds this risk; it cannot eliminate it. A build that looked an entry up when the entry was 59 minutes old and reads it more than an hour later loses it under any threshold, and `markUsed` ignores `Chtimes` errors. Re-checking an entry immediately before unlinking narrows the window between scan and removal; it does not close the lookup-to-read race inside the build. The policy therefore keeps every default idle threshold at or above 120 minutes, warns when a configured threshold is lower, and documents the residual risk instead of promising its absence.

## Outcome Boundary

- Outcome Lock: an operator or agent on a shared machine can read the disk budget from `autopus.yaml`, see free space and the known consumers with `auto disk status` (including what the trim policy would reclaim and which finished worktrees are candidates), gate a heavy step with `auto disk check`, and reclaim idle go-build cache entries and abandoned go-build work directories with `auto disk trim`, where deletion never leaves the two resolved roots, never follows a symlink, never touches an entry that changed since it was scanned, and never removes an entry whose own mtime is younger than the configured idle threshold; `auto doctor` shows a disk row; the generated workflow guidance on every platform runs the preflight before worktree creation, parallel fan-out, and full test runs and proceeds only on exit 0.
- Mandatory requirements: REQ-DISK-SCOPE-01 through REQ-DISK-VERIFY-01 below.
- Explicit non-goals: automatic reclamation of worktrees, scratchpads, scratch databases, VM images, model caches, colima, or other projects' worktrees; a consumer registry; disk tokens or build-concurrency limits; cooperation with a running `go` process to learn which entries it holds; installing or editing a launchd job or the user's `trim.sh`; the msgmcp temp-leak rule; Windows free-space support; editing any generated root.
- Completion evidence: focused Go tests for config, measurement, policy, containment, CI detection, exit codes, worktree classification, doctor projection, and surface parity; the fixture oracles in `acceptance.md`; `go run ./cmd/source-lines --max 300 --ext .go` clean over every changed Go file; and the live evidence on the dev Mac recorded in `research.md`.

## Definitions

- GiB: 1073741824 bytes; MiB: 1048576 bytes. Every `*_gb` value is GiB (matching `df -g`), every `*_mb` value is MiB; human output prints one decimal.
- Floor: `disk.min_free_gb`. Required free space of a check: the larger of the floor and the requested need or named reserve.
- Cache root: the resolved Go build cache directory. Cache entry: a path `<root>/<xx>/<64 hex>-d` (`xx` two lowercase hex digits) that `Lstat` reports as a regular file (data entry) or a directory (executable entry). `-a` files, `README`, `trim.txt`, `log.txt`, and files inside an executable entry are never entries on their own.
- Work dir root: `$GOTMPDIR` when set, otherwise `os.TempDir()`. Work dir: a directory (not a symlink) directly under the work dir root whose name matches `^go-build[0-9]+$`.
- Idle of a cache entry: now minus the entry's own `Lstat` mtime. Idle of a work dir: now minus the newest mtime of any entry in its tree, found by a bounded walk. A threshold of N minutes selects idle strictly greater than N minutes.
- Entry size: the file size of a data entry; the allocated byte sum of an executable entry's or work dir's tree. Allocated bytes: `st_blocks * 512` where exposed, otherwise the file size.
- CI running: at least one process whose executable basename equals one of `disk.trim.ci_process_names`. Run: `dry-run` or `apply`. Mode: `emergency` (free below the floor), `ci`, or `idle`, selected in that order.
- Identity of a path: its `(dev, ino)` pair; for a candidate also its size and mtime, all from `Lstat` at scan time.
- Scratchpad root: `EvalSymlinks("/tmp")/claude-<uid>` in production; injectable for tests.
- Harness worktree: a linked worktree under `.autopus/worktrees/`, `.claude/worktrees/`, or `.worktrees/` of the main worktree.

## Requirements

### Scope And Source Ownership

**REQ-DISK-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL make all changes inside `autopus-adk/` source-of-truth paths (`pkg/`, `internal/`, `content/`, `templates/`, `docs/`, including the codex native skill bodies under `pkg/adapter/codex/`), SHALL NOT edit the generated roots `.claude/**`, `.codex/**`, `.gemini/**`, `.opencode/**`, `.omp/**`, or `.agents/**`, and SHALL NOT read, write, install, or remove the user's launchd job or `trim.sh`.

### Configuration

**REQ-DISK-CFG-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL read an optional `disk` section of `autopus.yaml` with the keys `min_free_gb`, `reserves` (reserve name to GiB), and `trim` holding `start_below_gb`, `ci_process_names`, `go_build` (`file_mb`, `idle_minutes`, `ci_idle_minutes`, `emergency_idle_minutes`, `large_file_mb`, `large_file_idle_minutes`, `large_file_below_gb`) and `go_build_work_dirs` (`idle_minutes`), SHALL resolve an absent key to the defaults 10, empty, 0, `["Runner.Worker"]`, 1, 120, 180, 120, 50, 120, 0, and 360 respectively, SHALL mark every key `omitempty` so a configuration at defaults writes no `disk` key, and SHALL expose the resolved values through one `Effective()` accessor.

**REQ-DISK-CFG-02**
Priority: Must
Type: Unwanted
IF `min_free_gb`, `start_below_gb`, or `large_file_below_gb` is negative, a reserve value is not a positive integer, a reserve name does not match `^[a-z][a-z0-9_-]{0,31}$`, `file_mb` or `large_file_mb` is below 1, any `*idle_minutes` value is below 60, or a `ci_process_names` entry is empty or contains a path separator, THEN THE SYSTEM SHALL fail configuration load with an error naming the dotted key and the bound, and SHALL leave an unknown nested key to the existing strict decoder.

**REQ-DISK-CFG-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL load configuration for `auto disk status`, `auto disk check`, and `auto disk trim` through `config.LoadPreview`, and SHALL NOT write `autopus.yaml` from any of them.

**REQ-DISK-CFG-04**
Priority: Must
Type: Event-driven
WHEN any resolved `*idle_minutes` value is below 120, THEN THE SYSTEM SHALL print, from `auto disk status` and `auto disk trim`, one line per such key reading `disk trim: warning: <dotted key>=<value> is below 120 and may delete an entry a running build has already looked up`, and in JSON SHALL add one envelope warning per key with code `idle_threshold_below_120`.

### Free Space

**REQ-DISK-FREE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL measure free space as `statfs` `Bavail` times the block unit of the file system holding the project directory (`Bsize` on darwin; `Frsize`, or `Bsize` when `Frsize` is zero, on linux), SHALL report that file system's mount point (`Mntonname` on darwin; the longest matching mount point from `/proc/self/mounts` on linux), and SHALL compute the same bytes `df -k` derives from the same `statfs` fields.

**REQ-DISK-FREE-02**
Priority: Must
Type: Unwanted
IF the operating system is neither darwin nor linux, THEN THE SYSTEM SHALL exit 1 from `auto disk status`, `auto disk check`, and `auto disk trim` with the single stderr line `disk: free space measurement is not supported on <GOOS>`, and the doctor row SHALL print a WARN line with the same text instead of a measurement.

### Status

**REQ-DISK-STATUS-01**
Priority: Must
Type: Event-driven
WHEN `auto disk status` runs, THEN THE SYSTEM SHALL print free space, mount point, the floor verdict, one row per consumer kind (`go_build_cache`, `go_build_work_dirs`, `worktrees`, `scratchpads`) with allocated bytes, item counts, and an availability reason where a consumer cannot be read, followed by the trim preview, and with `--json` SHALL emit the same data through the shared envelope with sizes in bytes and `trim_preview` equal to the `data` object `auto disk trim --json` produces for a dry run at the same instant, including `run` set to `dry-run`.

**REQ-DISK-STATUS-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL resolve the cache root as `$GOCACHE` when that variable is set, otherwise as the output of `go env GOCACHE` under a 5-second timeout, otherwise as `os.UserCacheDir()/go-build`, SHALL canonicalize it with `EvalSymlinks`, SHALL report the value `off` as `gocache_off`, a relative path as `relative_root`, and a missing directory as `root_missing`, and SHALL report an existing directory without a regular `README` file as `not_a_go_build_cache`.

**REQ-DISK-STATUS-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL compute consumer sizes by an `Lstat` walk that never follows symlinks and never crosses onto another device than the root, summing allocated bytes, SHALL count work dirs and the subset idle beyond `go_build_work_dirs.idle_minutes` by the newest-mtime rule, and SHALL report the scratchpad root with directory count, bytes, and the oldest and newest directory mtime as ages in minutes, without deleting anything there.

**REQ-DISK-STATUS-04**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL expose the scratchpad root, the free-space function, the process lister, the clock, and the current uid as injection seams of the scan so that tests supply fixture roots and values, and the production command SHALL resolve the scratchpad root to `EvalSymlinks("/tmp")/claude-<uid>` only.

### Check

**REQ-DISK-CHECK-01**
Priority: Must
Type: Event-driven
WHEN `auto disk check` runs with no flag, with `--need <GiB>`, or with `--for <reserve>`, THEN THE SYSTEM SHALL compute the required free space as the larger of the floor and the need or reserve, SHALL label the winning source `disk.min_free_gb`, `--need`, or `disk.reserves.<name>`, and with free space at or above the requirement SHALL print `disk check: ok: <free> GiB free, <required> GiB required (<source>) on <mount>` to stdout and exit 0.

**REQ-DISK-CHECK-02**
Priority: Must
Type: Unwanted
IF free space is below the requirement, THEN THE SYSTEM SHALL in text mode write exactly `disk check: insufficient free space: <free> GiB free, <required> GiB required (<source>) on <mount>; free <deficit> GiB or lower the requirement` to stderr with no `Error: ` prefix and exit 3, and in JSON mode SHALL write the envelope with status `error` and `error.code` `disk_insufficient` to stdout, write nothing to stderr, and exit 3, by writing its own diagnostic and returning an already-reported exit-coded error that `runRootCommand` neither prints nor re-codes.

**REQ-DISK-CHECK-03**
Priority: Must
Type: Unwanted
IF `--for` names a reserve absent from `disk.reserves`, `--need` is not a positive number, both flags are given, or free space cannot be measured, THEN THE SYSTEM SHALL exit 1 through the same already-reported mechanism with, respectively, `disk check: reserve "<name>" is not configured under disk.reserves in autopus.yaml`, `disk check: --need must be a positive number of GiB`, `disk check: --need and --for are mutually exclusive`, or `disk check: free space measurement failed: <reason>` on stderr in text mode, and in JSON mode with `error.code` `disk_check_invalid` for the first three and `disk_measurement_failed` for the last.

### Trim

**REQ-DISK-TRIM-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL run `auto disk trim` as a dry run unless `--apply` is given, SHALL print the header `disk trim: run=<dry-run|apply> mode=<idle|ci|emergency> free=<GiB> GiB ci=<running|idle|forced-running|forced-idle|unknown> rules=<active rules in attribution order>`, then `  roots: cache=<path or unavailable reason> work=<path or unavailable reason>`, then `  note: idle>Nm guarantees N-60m without use; mtime trimming bounds the risk to a running build, it does not remove it` with N the mode rule's threshold, then one line per active rule `  <rule>: candidates=<n> size=<size>`, then `  skipped_symlink=<n> (go_build_cache=<n>, go_build_work_dirs=<n>)` plus `  skipped_device=<n>`, `  skipped_owner=<n>`, `  skipped_shape=<n>`, and `  skipped_unbounded=<n>` when non-zero, then `disk trim: would reclaim <size> (dry-run; pass --apply to delete)` or `disk trim: reclaimed <size> (deleted=<n> skipped_changed=<n> skipped_symlink=<n> skipped_device=<n> skipped_owner=<n> vanished=<n> failed=<n>)`, and with `--list` SHALL print every candidate path relative to its root under its rule line.

**REQ-DISK-TRIM-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL select the mode as `emergency` when free space is below the floor, otherwise `ci` when CI is running or unknown, otherwise `idle`; SHALL evaluate rules in the fixed attribution order `go_build_large_files` (entries larger than `large_file_mb` idle beyond `large_file_idle_minutes`, active when `large_file_below_gb` is 0 or free space is below it), then `go_build_files` (entries larger than `file_mb` idle beyond `emergency_idle_minutes`, `ci_idle_minutes`, or `idle_minutes` by mode), then `go_build_work_dirs`; SHALL attribute each candidate to the first rule that matches it, count and delete it exactly once, and report totals as the sum over attributed candidates; and SHALL evaluate no rule and print only `disk trim: inactive (free above start_below_gb)` when `start_below_gb` is positive and free space is at or above it.

**REQ-DISK-TRIM-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL consider as go-build candidates only cache entries as defined (regular files or directories named `<xx>/<64 hex>-d`), SHALL compute idle from the entry's own `Lstat` mtime and size as the entry size, SHALL never select `-a` files, markers, or files inside an executable entry individually, and SHALL state in `--help` that an idle threshold of N minutes guarantees only N minus 60 minutes without use and that trimming by mtime bounds but does not eliminate the chance of removing an entry a running build has already looked up.

**REQ-DISK-TRIM-04**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL consider as work dir candidates only directories (not symlinks) directly under the work dir root whose name matches `^go-build[0-9]+$`, whose canonical path is not the project directory, the user's home directory, or the current working directory, whose top-level children are all directories named `^b[0-9]{3,}$` (an empty directory qualifies; Go's `NewObjdir` creates only these), whose newest tree mtime from a walk bounded to 100000 entries and depth 32 is idle beyond `go_build_work_dirs.idle_minutes`, and whose walk completed within the bound, SHALL report a work dir whose walk exceeded the bound as `skipped_unbounded`, and SHALL leave every other entry of the work dir root untouched, counting a name match that fails the shape or path check as `skipped_shape`.

**REQ-DISK-TRIM-05**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record the identity of each root and candidate at scan time, SHALL re-verify each root's identity before the first removal and refuse the run as `root_changed` on mismatch, SHALL for a file candidate `Lstat` it again immediately before unlink through the root and skip it as `skipped_changed` when `(dev, ino, size, mtime)` differs from the scan, SHALL for a directory candidate (executable entry or work dir) first walk it with fd-relative no-follow operations (`openat` with `O_NOFOLLOW|O_DIRECTORY`, `fstatat` with `AT_SYMLINK_NOFOLLOW`) and require the walked top directory's `(dev, ino)` to equal the candidate identity re-verified just before and skip as `skipped_changed` otherwise, skip the whole candidate as `skipped_symlink` or `skipped_device` before deleting anything when any symlink, any entry (file or directory) on another device, or a non-regular non-directory entry appears, then remove bottom-up with `unlinkat` re-checking each directory's identity, counting a mid-removal anomaly as `failed`, SHALL count an entry that vanished before removal as `vanished`, SHALL never follow a symlink, never cross the root's device, and never delete `README`, `trim.txt`, `log.txt`, or any `-a` file, and MAY wrap these operations in `os.Root` as an additional layer without relying on it for any guarantee above.

**REQ-DISK-TRIM-06**
Priority: Must
Type: Unwanted
IF a root (cache or work dir) does not exist, THEN THE SYSTEM SHALL report that consumer unavailable, evaluate the other root, and exit 0 for a dry run; IF a root exists but resolves to `/`, the user's home directory, or the project directory, is a relative path, is not owned by the current uid (a sticky work dir root excepted, REQ-DISK-TRIM-10), or (for the cache root) lacks the `README` marker, THEN THE SYSTEM SHALL delete nothing from either root, print `disk trim: refused: <root> is not an acceptable <cache|work dir> root (<reason>)`, and exit 1; the user's home directory comes from `$HOME`, falling back to the user database (`user.Current`), and a run that can determine neither refuses every root as `home_unknown`; the cache root's `README` marker is checked before its owner, and a refused root is not walked.

**REQ-DISK-TRIM-07**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL NOT run `go clean` in any form, SHALL run `go env GOCACHE` only when `$GOCACHE` is unset and then with `GOTOOLCHAIN=local` and `GOWORK=off` in its environment so a `go.mod` or `go.work` above the temp directory cannot make it download or exec another toolchain, and SHALL run no other `go` subprocess.

**REQ-DISK-TRIM-08**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL detect CI as running when the executable basename of any process equals a configured name exactly, using `ps -axo comm=` on darwin and `/proc/<pid>/comm` with `/proc/<pid>/exe` on linux through an injectable process lister, SHALL NOT match substrings of full command lines, SHALL strip one pair of surrounding parentheses that `ps` prints for some processes before matching, SHALL honour `--ci running` and `--ci idle` as overrides reported as `forced-running` and `forced-idle`, and on a lister failure SHALL report `ci=unknown`, print `disk trim: ci detection unavailable (<reason>); treating CI as running`, and still select `emergency` when free space is below the floor.

**REQ-DISK-TRIM-09**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL emit for every run (dry run, apply, inactive, refused) a JSON `data` object with `run` in `{dry-run, apply}`, `mode` in `{idle, ci, emergency}`, `active`, `refused` (null or `{root, kind, reason}`), `free_bytes`, `floor_bytes`, `mount`, `ci` (`{state, source}`), `roots` (`{cache, work}` each `{path, available, reason}`), `rules` (per rule `id`, `min_bytes`, `idle_minutes`, `candidates`, `bytes`, `deleted`, `deleted_bytes`, `skipped_changed`, `vanished`, `failed`, and `files` only with `--list`), `skipped` (`symlink` per root, `device` per root, `owner`, `shape`, `unbounded`), and `total_bytes`, and SHALL exit 1 for an apply with `failed` greater than zero and for any refused run, 0 otherwise.

**REQ-DISK-TRIM-10**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL require each root and each candidate to be owned by the current uid, except that a work dir root owned by uid 0 is accepted when it is a directory with the sticky bit set (the shared `/tmp` of Linux, used when `$GOTMPDIR` and `$TMPDIR` are unset); a sticky directory owned by any other uid stays refused, because its owner may rename any entry in it, SHALL skip a candidate owned by another uid as `skipped_owner` without deleting anything under it, and SHALL treat any other root owned by another uid as refused under REQ-DISK-TRIM-06.

### Worktrees

**REQ-DISK-WT-01**
Priority: Must
Type: Event-driven
WHEN the project directory is inside a git repository, THEN THE SYSTEM SHALL enumerate `git --no-optional-locks worktree list --porcelain -z`, SHALL mark the main worktree `main`, and for every linked worktree SHALL report path, branch, `locked`, `prunable`, `clean` (true only when `git status --porcelain=v1 -z --untracked-files=normal` exits 0 with empty output, false on non-empty output, `unknown` with a reason on any error or missing path), `merged` (true when `git merge-base --is-ancestor HEAD <base>` exits 0, false on exit 1, `unknown` with a reason on any other exit or when no base among `origin/HEAD`, `origin/main`, `main`, `master` exists), allocated bytes, `cargo_target_bytes` over directories named `target` whose parent holds `Cargo.toml` within depth 6, and the `harness` flag.

**REQ-DISK-WT-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL NOT remove, prune, lock, or modify any worktree, branch, or `target` directory, SHALL print `candidate: git worktree remove -- <quoted path> (merged into <base>, clean, <size>)` only for a linked worktree with `merged` true, `clean` true, `locked` false, and `prunable` false, where the path is quoted as a POSIX single-quoted word (`'` becomes `'\''`), SHALL replace every non-printable rune (as `strconv.IsPrint` judges it, including bidirectional overrides, and invalid UTF-8) with `?` in every displayed path and print `suggestion withheld: path contains control characters` instead of the command for such a worktree, and SHALL report the consumer unavailable as `not_a_git_repository` outside a repository.

### Doctor

**REQ-DISK-DOC-01**
Priority: Must
Type: Event-driven
WHEN `auto doctor` runs, THEN THE SYSTEM SHALL print a `Disk` section with `[OK] disk: <free> GiB free on <mount> (floor <floor> GiB)` at or above the floor, `[WARN] disk: <free> GiB free on <mount> is below the <floor> GiB floor (disk.min_free_gb); run 'auto disk status'` below it, or the REQ-DISK-FREE-02 WARN line on an unsupported OS, SHALL keep the row advisory so the overall verdict is unchanged, and in JSON SHALL add the check `doctor.disk.free_space` with status `pass` or `warn`, severity `info` or `warning`, and fields `free_bytes`, `floor_bytes`, and `mount` (or `reason` on an unsupported OS), without changing the envelope status.

### Guidance

**REQ-DISK-GUIDE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL carry the preflight on every surface where a heavy step starts and that actually ships: the agent-pipeline references `content/skills/references/agent-pipeline/delegation.md` (worker hygiene) and `verification.md` (full-suite run), rendered on every platform by `RenderSkillResources`; the Claude body `content/skills/worktree-isolation.md`; the shared gemini and opencode body `templates/shared/supervisor-worktree-isolation.md.tmpl` and its generated mirror `templates/gemini/skills/worktree-isolation/SKILL.md.tmpl`; the OMP body `templates/shared/omp-worktree-isolation.md.tmpl`; and the codex native body `codexWorktreeIsolationSkillBody` in `pkg/adapter/codex/codex_extended_skill_rewrites_worktree.go`, which `make generate-templates` does not produce; each surface SHALL state that the step proceeds only on exit 0, stops and reports insufficient space on exit 3, and stops and reports a measurement or configuration failure on any other exit; and a parity test SHALL pin the tokens `auto disk check --need`, `exits 3`, and `exit 0` on each surface.

### Verification

**REQ-DISK-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep every new or changed Go file of this change set, tests included, at or under 300 comment-aware lines as measured by `go run ./cmd/source-lines --max 300 --ext .go` over `git diff --name-only origin/main -- '*.go'`, SHALL cover REQ-DISK-CFG-01 through REQ-DISK-GUIDE-01 with the oracle scenarios of `acceptance.md`, and SHALL record live evidence from the dev Mac with `GOCACHE`, `GOTMPDIR`, and `TMPDIR` exported to scratch directories for the whole apply-and-rebuild sequence.

## Generated And Changed Surfaces

| Surface | Role | Change |
|---------|------|--------|
| `[NEW] pkg/config/schema_disk.go`, `[NEW] pkg/config/schema_disk_test.go` | `disk` section, defaults, validation, accessor | new files; `HarnessConfig` in `pkg/config/schema.go` gains `Disk DiskConf` with `omitempty` |
| `[NEW] pkg/diskspace/free_darwin.go`, `free_linux.go`, `free_other.go`, `format.go` | free space, mount, GiB formatting | new package |
| `[NEW] pkg/diskspace/gobuild.go`, `workdirs.go`, `scratchpads.go`, `worktrees.go`, `sizes.go` | consumer scans with injection seams | new files |
| `[NEW] pkg/diskspace/policy.go`, `ci.go`, `apply.go`, `remove_unix.go` | mode selection and attribution, CI lister, identity-checked contained deletion | new files; `remove_unix.go` uses `golang.org/x/sys/unix` (already a dependency) |
| `[NEW] internal/cli/disk.go`, `disk_status.go`, `disk_check.go`, `disk_trim.go` | `auto disk` namespace, text and JSON output, exit codes | new files; `root.go` registers `newDiskCmd()` |
| `internal/cli/root.go`, `internal/cli/output_json.go` | already-reported error handling | `runRootCommand` skips printing any error carrying the `[NEW] alreadyReported` marker; `jsonFatalError` implements it |
| `[NEW] internal/cli/doctor_disk.go` | doctor text row and JSON check | new file; `doctor_text.go` and `doctor_json.go` call it |
| `content/skills/references/agent-pipeline/delegation.md`, `verification.md`, `content/skills/worktree-isolation.md`, `templates/shared/supervisor-worktree-isolation.md.tmpl`, `templates/shared/omp-worktree-isolation.md.tmpl`, `pkg/adapter/codex/codex_extended_skill_rewrites_worktree.go` | guidance sources | preflight with exit semantics; gemini mirror regenerated; `templates/disk_check_surface_test.go` extended to the codex body and the `exit 0` token |
| `docs/disk-management.md`, `README.md`, `docs/README.ko.md` | operator documentation | aligned to the revised defaults, entry kinds, counters, and residual-risk statement |

## Related SPECs

None as siblings. Reused contracts: the JSON envelope of `SPEC-CLIJSON-001` (`internal/cli/output_json.go`), the `statfs` measurement precedent of `SPEC-SIGMABAND-002` (`internal/cli/react_band_localpatch_disk_darwin.go`), and the worktree lifecycle guidance of `SPEC-WORKTREE-001` (`templates/shared/supervisor-worktree-isolation.md.tmpl`), none of which is reopened.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-DISK-SCOPE-01 | T9, T10 | S9 | INV-D11 |
| REQ-DISK-CFG-01 | T1 | S1 | INV-D09 |
| REQ-DISK-CFG-02 | T1 | S1 | INV-D09 |
| REQ-DISK-CFG-03 | T7 | S10 | INV-D08 |
| REQ-DISK-CFG-04 | T1, T7 | S1, S5 | INV-D09 |
| REQ-DISK-FREE-01 | T2 | S2, S11 | INV-D01 |
| REQ-DISK-FREE-02 | T2, T7, T8 | S2, S8 | INV-D01 |
| REQ-DISK-STATUS-01 | T3, T7 | S4, S11 | INV-D03, INV-D07 |
| REQ-DISK-STATUS-02 | T3 | S4, S5 | INV-D05 |
| REQ-DISK-STATUS-03 | T3 | S4 | INV-D07, INV-D14 |
| REQ-DISK-STATUS-04 | T3, T7 | S4 | INV-D07 |
| REQ-DISK-CHECK-01 | T7 | S3 | INV-D02 |
| REQ-DISK-CHECK-02 | T7 | S3 | INV-D02 |
| REQ-DISK-CHECK-03 | T7 | S3 | INV-D02 |
| REQ-DISK-TRIM-01 | T5, T7 | S5 | INV-D03 |
| REQ-DISK-TRIM-02 | T5 | S5, S12 | INV-D04, INV-D13 |
| REQ-DISK-TRIM-03 | T5 | S5 | INV-D03 |
| REQ-DISK-TRIM-04 | T5, T6 | S5 | INV-D14 |
| REQ-DISK-TRIM-05 | T6 | S5 | INV-D05 |
| REQ-DISK-TRIM-06 | T3, T6 | S5 | INV-D05 |
| REQ-DISK-TRIM-07 | T6 | S5 | INV-D12 |
| REQ-DISK-TRIM-08 | T5 | S6 | INV-D06 |
| REQ-DISK-TRIM-09 | T6, T7 | S5, S12 | INV-D03, INV-D13 |
| REQ-DISK-TRIM-10 | T6 | S5 | INV-D15 |
| REQ-DISK-WT-01 | T4 | S7 | INV-D07 |
| REQ-DISK-WT-02 | T4 | S7 | INV-D07 |
| REQ-DISK-DOC-01 | T8 | S8 | INV-D10 |
| REQ-DISK-GUIDE-01 | T9 | S9 | INV-D11 |
| REQ-DISK-VERIFY-01 | T11 | S11 | INV-D01, INV-D03 |
