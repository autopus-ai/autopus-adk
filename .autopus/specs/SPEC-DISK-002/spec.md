# SPEC-DISK-002: Shared-Machine Disk Management — Ownership Ledger, Reclaim, Disk Leases, and Alerts

**Status**: draft
**Created**: 2026-11-02
**Revised**: 2026-11-02 (codex review rounds 1 and 2, judge REVISE; security review M1–M2, L1–L5, I1, I3)
**Domain**: DISK
**Priority**: HIGH
**Change Class**: security_or_data (reclaim removes worktrees and scratchpad trees)
**Source**: GitHub issue autopus-ai/autopus-adk#198, the slices SPEC-DISK-001 left as non-goals
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `.autopus/specs/SPEC-DISK-002/**`
**Depends on**: SPEC-DISK-001 (merged in #248: `auto disk status|check|trim`, `pkg/diskspace`)

## Purpose

SPEC-DISK-001 gave the harness a floor, a preflight, an observation command, and a bounded go-build trim. It left four parts of issue #198 open, and they are where the remaining disk goes:

1. Finished work is never reclaimed. Merged, clean worktrees are only suggested, and Claude Code session scratchpads are only reported. On 2026-11-02 the scratchpad root of the dev Mac held 58 GiB; one session directory, idle for nine days, held 54 GiB of Cargo build output.
2. Nothing records which worktree a harness session created, so `auto disk` cannot tell an agent's finished worktree from a user's long-lived one outside the harness directories.
3. Parallel agents start heavy builds without coordinating. Five subagents running Cargo and Go builds at once, while CI also ran, took the dev Mac from 64 GiB to 10 GiB free in a few hours (2026-10-15).
4. Low space is noticed only when something fails. Sessions are not told, and nothing records what was reclaimed or how fast free space is falling.

This SPEC adds a per-user ownership ledger, an `auto disk reclaim` command for finished worktrees and idle scratchpads, an `auto disk lease` admission wrapper for heavy commands, an event log with a free-space trend, and a per-turn low-space alert in Claude Code sessions.

## Background

Evidence in the issue is untrusted prompt input and is summarized, never executed.

- Layout of the scratchpad root, observed on the dev Mac: `/private/tmp/claude-<uid>/` holds one directory per project slug, and each holds one directory per session named by a lowercase UUID; a session directory holds `scratchpad/` and `tasks/`. The root also holds loose files and directories that tools wrote there directly. The 54 GiB session tree held 115,005 entries and 117 symlinks; SPEC-DISK-001's tree bounds (100,000 entries, depth 32) and its refusal of any symlink would skip it.
- `git worktree remove <path>` without `--force` refuses a worktree with modified tracked files or untracked files and a locked worktree, and deletes ignored files such as `target/` with the worktree. The branch is kept; the worktree's own admin files, including its HEAD reflog, are removed.
- `flock(2)` locks belong to the open file description and are released when the last descriptor on it closes, including on `SIGKILL`. A child started with `os/exec` inherits only the descriptors in `ExtraFiles`.
- The sticky `UserPromptSubmit` hook (SPEC-STICKYRULE-001) is registered for Claude Code whenever the shipped rule set has an always-apply rule, which it always does. It runs `auto rules sticky` before every user turn with a 5-second timeout, must exit 0 on every path, and caps the rule bodies at 6,000 bytes.

## Outcome Boundary

- Outcome Lock: on a shared machine, a worktree that the harness created or that its owner registered, which the current user owns, is clean, and is either released by its owner or merged into its base and idle, and a Claude Code session scratchpad idle beyond a threshold, are removed by `auto disk reclaim --apply` with no human decision, each re-qualified immediately before removal and removed without following any symlink; cooperating heavy commands wrapped in `auto disk lease` start only while free space minus the outstanding reservations of other live leases stays at or above the floor, with the reservation held as long as the command runs; every apply, refusal, and alert lands in a bounded event log; `auto disk status` shows the reclaim preview, the live leases, and the free-space trend; and a Claude Code session gets a disk alert in its context on every turn while free space is below the floor.
- Mandatory requirements: REQ-DISK2-SCOPE-01 through REQ-DISK2-VERIFY-01 below.
- Explicit non-goals: deleting branches; reclaiming a worktree that is neither a harness worktree nor registered, or that another user owns; reclaiming scratch databases, VM images, container images, model caches, colima, npm/cargo/pip registries, or loose entries at the top of the scratchpad root; installing a launchd, systemd, or cron job (documentation shows how); throttling processes that do not use `auto disk lease`; alerts on platforms other than Claude Code (they keep the `auto disk check` preflight); Windows reclaim and leases beyond compiling.
- Completion evidence: focused Go tests for config, state, ledger, reclaim selection, re-qualification and removal, lease admission and lifetime, event log and trend, the sticky alert, and guidance parity; the fixture oracles in `acceptance.md`; `go run ./cmd/source-lines --max 300 --ext .go <changed Go files>` clean; and live evidence on the dev Mac recorded in `research.md`.

## Definitions

- State dir: `$XDG_STATE_HOME/autopus/disk` when `XDG_STATE_HOME` is an absolute path, else `<home>/.local/state/autopus/disk`, on both Linux and macOS; injectable for tests. Its parent, itself, and its `leases/` subdirectory are created with mode 0700 when missing.
- Usable state dir: the parent, the state dir, and `leases/` are each a real directory (not a symlink), owned by the current uid, and writable by neither group nor others. Lock files in it (`registry.lock`, `lease.lock`, `events.lock`, `samples.lock`) are created once and never replaced or unlinked, so every flock on a lock path names one inode.
- Ledger: `<state dir>/registry.json`, read only as a regular file owned by the current uid, written only by temp file and rename under `registry.lock`.
- Registered worktree: a ledger entry of kind `worktree` whose path still `Lstat`s to the recorded `(dev, ino)`. An entry whose path is missing or has another identity is stale: it is ignored, and it is dropped on the next ledger write.
- Harness worktree: a linked worktree under `.autopus/worktrees/`, `.claude/worktrees/`, or `.worktrees/` of its main worktree (SPEC-DISK-001), or directly under the system temp dir with a name matching `^autopus-worktree-[0-9]+$` (the pipeline `WorktreeManager`'s worktrees).
- Owned worktree: a linked worktree of a scanned repository that is a harness worktree or a registered worktree.
- Worktree activity time: the newest `Lstat` mtime among the worktree root directory, every entry of its tree except under the top-level `.git` (the whole tree, so a build writing deep inside `target/` counts), and the worktree's git admin files `HEAD`, `index`, and `logs/HEAD` (those that exist). The same walk notes any entry on another device than the worktree root. Worktree idle: now minus the activity time.
- Session directory: `<scratchpad root>/<slug>/<uuid>` where `<slug>` is a directory (not a symlink) directly under the root and `<uuid>` matches `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, is a directory (not a symlink), and is owned by the current uid. Scratchpad idle: now minus the newest `Lstat` mtime of any entry in the session tree, found by a bounded walk of at most 2,000,000 entries and depth 128.
- Lease: a file `<state dir>/leases/<pid>-<16 hex>.lease` created with `O_EXCL`, flocked, then filled with JSON `{pid, label, need_bytes, started}`, all before the admission lock is released. Live lease: one whose flock another descriptor cannot take. Reserved bytes: the sum of `need_bytes` over live leases, each clamped to `[0, 1 PiB]`.
- Event log: `<state dir>/events.jsonl`. Sample log: `<state dir>/samples.jsonl`, one `{time, free_bytes, mount}` per line.

## Requirements

### Scope And Source Ownership

**REQ-DISK2-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL make all changes inside `autopus-adk/` source-of-truth paths (`pkg/`, `internal/`, `content/`, `templates/`, `docs/`, `README.md`, `.autopus/specs/SPEC-DISK-002/`, and the codex native skill bodies under `pkg/adapter/codex/`), SHALL NOT edit the generated roots `.claude/**`, `.codex/**`, `.gemini/**`, `.opencode/**`, `.omp/**`, or `.agents/**`, and SHALL NOT install, edit, or remove any launchd, systemd, or cron job or the user's `trim.sh`.

### Configuration

**REQ-DISK2-CFG-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL bound every SPEC-DISK-001 and SPEC-DISK-002 key from above so no conversion to bytes or to a duration can overflow: `min_free_gb`, `reserves.*`, `trim.start_below_gb`, and `trim.go_build.large_file_below_gb` at 1048576 (1 PiB), the `*_mb` keys at 1073741824, and every `*idle_minutes` and `lease.wait_minutes` at 5256000 (ten years), failing configuration load with `<dotted key> must be <= <bound>`; and SHALL extend the optional `disk` section with `reclaim.worktree_idle_minutes` (default 360), `reclaim.scratchpad_idle_minutes` (default 1440), `lease.max_concurrent` (default 0, meaning no count limit), `lease.wait_minutes` (default 10), and `lease.reclaim_on_shortage` (default true), all `omitempty` so a configuration at defaults still writes no `disk` key, all resolved through `DiskConf.Effective()`.

**REQ-DISK2-CFG-02**
Priority: Must
Type: Unwanted
IF `reclaim.worktree_idle_minutes` or `reclaim.scratchpad_idle_minutes` is below 60, or `lease.max_concurrent` or `lease.wait_minutes` is negative, THEN THE SYSTEM SHALL fail configuration load with an error naming the dotted key and its bound.

### State Directory

**REQ-DISK2-STATE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL resolve and create the state dir as defined, SHALL refuse a state dir that is not usable without writing anything inside it or through a link, SHALL write the ledger only by temp file and rename, and SHALL open the event and sample logs for append only after checking that the path and the opened descriptor name the same regular file owned by the current uid.

**REQ-DISK2-STATE-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL apply this failure policy when the state dir is not usable: `register`, `release`, `registry`, `lease`, and `log` exit 1 naming the state dir; `reclaim` proceeds with an empty ledger (harness worktrees and scratchpads only) and its exit code is unaffected; `status`, `check`, `trim`, and the sticky hook skip their samples and events and their outcome is unaffected.

### Ownership Ledger

**REQ-DISK2-REG-01**
Priority: Must
Type: Event-driven
WHEN `auto disk register <path> [--owner <text>]` runs, THEN THE SYSTEM SHALL resolve the path to an absolute, symlink-free path, require that it is the top-level directory of a linked git worktree (its `git rev-parse --git-dir` differs from `--git-common-dir`) owned by the current uid, and record or refresh one ledger entry `{path, kind: "worktree", owner, registered_at, dev, ino}` with `released_at` cleared. A path that is not the top of a linked worktree SHALL exit 1 with `disk register: <path> is not the top of a linked git worktree`, one owned by another uid with `disk register: <path> is not owned by the current user`, and neither SHALL change the ledger. `--owner` is stored without control characters and cut to 200 bytes on a rune boundary.

**REQ-DISK2-REG-02**
Priority: Must
Type: Event-driven
WHEN `auto disk release <path>` runs, THEN THE SYSTEM SHALL set `released_at` on the matching live entry, print `disk release: <path>`, and exit 0; when no live entry matches it SHALL print `disk release: <path> is not registered` and exit 0, so a cleanup step never fails on it.

**REQ-DISK2-REG-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL serialize every ledger read-modify-write under `registry.lock` with a 10-second wait, SHALL drop stale entries on every write, and SHALL refuse to overwrite a ledger that fails to parse, exiting 1 with `disk: ledger <path> is unreadable: <error>` and leaving the file byte-identical.

**REQ-DISK2-REG-04**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL treat a linked worktree directly under the system temp dir named `autopus-worktree-<digits>`, which the pipeline `WorktreeManager` creates, as a harness worktree, so the pipeline needs no ledger write.

**REQ-DISK2-REG-05**
Priority: Must
Type: Event-driven
WHEN `auto disk registry [--json]` runs, THEN THE SYSTEM SHALL list every ledger entry with its path, owner, registration and release times, and its state `live`, `released`, or `stale`, and SHALL change nothing.

### Reclaim

**REQ-DISK2-RECLAIM-01**
Priority: Must
Type: Event-driven
WHEN `auto disk reclaim` runs without `--apply`, THEN THE SYSTEM SHALL print one header line with the candidate counts, sizes, and thresholds per kind, one line per candidate with its kind, path, reason, idle minutes, and size, one `kept <kind>:` line with the non-zero skip counters in name order, and `disk reclaim: would reclaim <size> (dry-run; pass --apply to delete)`, SHALL delete nothing, and with `--json` SHALL carry the same data in the standard envelope.

**REQ-DISK2-RECLAIM-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL select a worktree candidate only when every one of these holds: it is an owned worktree; the current uid owns it; it is linked, not locked, and not prunable; `Clean` is true; it does not contain the process working directory; no entry of its tree is on another device; and either it is a released registered worktree that has a branch or whose HEAD is merged into its base, or `Merged` is true and its idle exceeds `reclaim.worktree_idle_minutes`. The repositories scanned are the one holding the project directory and every repository a live ledger entry belongs to; every skipped owned worktree is counted under its reason (`owner`, `locked`, `prunable`, `unknown`, `dirty`, `cwd`, `device`, `detached_unmerged`, `unmerged`, `recent`), and an unowned one under `not_owned`.

**REQ-DISK2-RECLAIM-03**
Priority: Must
Type: Event-driven
WHEN a worktree candidate is applied, THEN THE SYSTEM SHALL, immediately before removal, re-read its identity, owner, HEAD, clean state, and device boundary and, for `merged_idle`, its merge into the base and its activity against the scan's idle cutoff, or, for `released`, a live released ledger entry with the same identity and a HEAD that is still on a branch or merged into the base; it SHALL skip the candidate as `skipped_device` when its tree now crosses a device and as `skipped_changed` when any other check no longer holds, and otherwise run `git -C <main worktree> worktree remove <path>` without `--force`, counting git's refusal as `failed`; it SHALL NOT delete the branch, and SHALL drop the ledger entry after a successful removal.

**REQ-DISK2-RECLAIM-04**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL select a scratchpad candidate only for a session directory whose idle exceeds `reclaim.scratchpad_idle_minutes`, that is on the root's device, and that does not contain the process working directory, and SHALL count a session tree past the walk bound as `unbounded`, one holding an entry on another device as `device`, one another uid owns as `owner`, one too recent as `recent`, and one holding the working directory as `cwd`.

**REQ-DISK2-RECLAIM-05**
Priority: Must
Type: Event-driven
WHEN a scratchpad candidate is applied, THEN THE SYSTEM SHALL remove it relative to the scratchpad root opened once and re-identified, re-check the session directory's identity, owner, and mtime and the tree's newest mtime against the scan, record the tree through descriptors without following any symlink, and remove exactly what it recorded bottom-up, re-identifying each entry: a non-directory entry of any kind, symlinks included, is unlinked as a name and never opened or followed; a device change skips the whole candidate before anything in it is deleted; and the slug directory is removed afterwards only when it is empty.

**REQ-DISK2-RECLAIM-06**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL refuse the scratchpad root, with no scratchpad deletion, when it is relative, a symlink, not a directory, not owned by the current uid, or writable by group or others, and SHALL report the refusal reason in the dry run and the apply.

**REQ-DISK2-RECLAIM-07**
Priority: Must
Type: Event-driven
WHEN an apply ends, THEN THE SYSTEM SHALL print `disk reclaim: reclaimed <size> (worktrees=<n> scratchpads=<n> skipped_changed=<n> skipped_symlink=<n> skipped_device=<n> vanished=<n> failed=<n>)`, append one `reclaim_apply` event, and exit 1 when any removal failed or the scratchpad root was refused, else 0.

### Leases

**REQ-DISK2-LEASE-01**
Priority: Must
Type: Event-driven
WHEN `auto disk lease (--need <GiB> | --for <reserve>) [--wait <duration>] [--label <text>] -- <command> [args...]` runs, THEN THE SYSTEM SHALL, under `lease.lock`, discard lease files whose flock it can take, measure free space, and admit the command only when `need + reserved + floor ≤ free` in saturating arithmetic (a floor that overflowed to a negative value admits nothing) and, when `lease.max_concurrent` is above 0, fewer than that many live leases exist; on admission it SHALL create, flock, and fill its own lease file before releasing the admission lock.

**REQ-DISK2-LEASE-02**
Priority: Must
Type: Event-driven
WHEN admission fails for space and `lease.reclaim_on_shortage` is true, THEN THE SYSTEM SHALL once per invocation run the go-build trim apply and the reclaim apply, record `trim_apply`, `reclaim_apply`, and `lease_reclaimed` events, and re-evaluate; a refusal by `lease.max_concurrent` alone SHALL NOT reclaim. While admission still fails it SHALL re-evaluate every 5 seconds until the wait (`--wait`, else `lease.wait_minutes`) ends, then exit 3 with `disk lease: <need> GiB needed above the floor, <available> GiB available after <reserved> GiB reserved by <n> leases; waited <duration>`, or `disk lease: <n> leases already run (disk.lease.max_concurrent=<m>); waited <duration>` for a count refusal, and record a `lease_refused` event.

**REQ-DISK2-LEASE-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL run the admitted command with the inherited standard streams and environment and with the lease file as an inherited descriptor, so the reservation lives while either the wrapper or the command holds it; it SHALL forward `SIGTERM` and `SIGHUP` to the command, catch `SIGINT` without forwarding it (a terminal already delivers it to the command's process group), and after the command exits close its own descriptor without unlocking and remove the lease file only when no other process still holds its lock (a process the command left running keeps the reservation; status and JSON mark such a lease `orphaned` once its wrapper pid is gone), and exit with the command's exit code, or 128 plus the signal number when a signal ended it; a command that cannot be started SHALL exit 127 with `disk lease: cannot start <command>: <error>`.

The label is `--label`, else the base name of the command, with control characters removed and cut to 200 bytes, because it is stored and printed.

**REQ-DISK2-LEASE-04**
Priority: Must
Type: Unwanted
IF `--need` and `--for` are both given or both missing, `--need` is not a number from 1 to 1048576 GiB (NaN included), the reserve is not configured or exceeds 1048576 GiB, `--wait` is negative, or no command follows `--`, THEN THE SYSTEM SHALL exit 1 with a one-line `disk lease:` usage error and start nothing.

### Events, Samples, And Trend

**REQ-DISK2-LOG-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL append events of kinds `trim_apply`, `reclaim_apply`, `lease_reclaimed`, `lease_refused`, and `low_space_alert`, each `{time, kind, free_bytes, reclaimed_bytes, count, message}`, under `events.lock`, rotating the log to `events.jsonl.1` (replacing an older one) inside the same lock before an append would make it exceed 1 MiB; a failure to write an event SHALL never change a command's exit code.

**REQ-DISK2-LOG-02**
Priority: Must
Type: Event-driven
WHEN `auto disk log [--limit <n>] [--json]` runs, THEN THE SYSTEM SHALL print the last `n` events (default 20, at least 1) of the rotated and current logs, oldest first, skipping unparseable lines, and change nothing.

**REQ-DISK2-LOG-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL append a free-space sample from `auto disk status`, `auto disk check`, `auto disk lease`, and the sticky alert, under `samples.lock`, only when the newest sample of the same mount is at least 5 minutes old, and SHALL rewrite the sample log keeping at most the newest 2,000 samples and none older than 7 days whenever it exceeds 256 KiB.

**REQ-DISK2-LOG-04**
Priority: Must
Type: Event-driven
WHEN `auto disk status` runs and the sample log holds, for the current mount, a sample between 10 minutes and 6 hours old, THEN THE SYSTEM SHALL print `free trend: <signed GiB>/h over <duration>` computed from the oldest such sample to the current measurement, and SHALL carry `{window_minutes, bytes_per_hour}` in JSON; otherwise it SHALL omit the line and carry `null`.

### Status

**REQ-DISK2-STATUS-01**
Priority: Must
Type: Event-driven
WHEN `auto disk status` runs, THEN THE SYSTEM SHALL add, after the SPEC-DISK-001 sections, the reclaim preview (candidate counts and bytes per kind), one `owned worktree:` line per owned worktree, the live leases (need, label, pid, age) or `disk leases: none`, and the trend line, in text and JSON, reading leases without discarding any.

### Alert

**REQ-DISK2-ALERT-01**
Priority: Must
Type: Event-driven
WHEN the sticky `UserPromptSubmit` hook runs, the project configuration loads, and free space on the project's file system is below the floor, THEN THE SYSTEM SHALL append to the hook's additional context, after the rule bodies when any are due and alone otherwise, on every turn regardless of the sticky cadence, exactly `Disk alert: <free> GiB free on <mount>, below the floor of <floor> GiB (disk.min_free_gb). Do not start builds, worktrees, or parallel agents until space recovers; run \`auto disk status\`, then \`auto disk reclaim --apply\` and \`auto disk trim --apply\`. If you lead other agents, tell each of them to pause heavy work.`, and SHALL record a `low_space_alert` event at most once per 30 minutes, the check and the append under one `events.lock`.

**REQ-DISK2-ALERT-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep the sticky hook's exit-zero and write-once contracts: the alert is outside the 6,000-byte rule cap and never displaces a rule body; a configuration that does not load or a free-space measurement that fails drops the alert; an unusable state dir drops only its sample and event, and the alert is still shown; and when free space is at or above the floor the output is byte-identical to SPEC-STICKYRULE-001's. The alert path adds one `statfs` and at most two appends whose lock waits are bounded at 250 ms each, inside the hook's 5-second budget.

### Guidance

**REQ-DISK2-GUIDE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL extend the shipping guidance surfaces that SPEC-DISK-001 changed (delegation, verification, worktree isolation, supervisor, gemini and omp templates, the codex native worktree-isolation body) so that an agent registers a worktree it creates outside the harness worktree directories with `auto disk register <path>` and releases it with `auto disk release <path>` when done, runs a full build or test suite as `auto disk lease --need <GiB> -- <command>`, and, as a lead, tells every running worker to pause heavy work when a disk alert or a failed preflight appears, without naming any platform-specific directory in a section that must stay byte-identical across platforms.

**REQ-DISK2-GUIDE-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL document the ledger, reclaim, lease, log, trend, alert, and the state failure policy in `docs/disk-management.md`, including a generic periodic-run example for launchd, systemd, and cron that runs `auto disk trim --apply` and `auto disk reclaim --apply` with no machine-specific path or threshold, and the residual risks.

### Verification

**REQ-DISK2-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL prove every Must requirement with tests that use only directories they create, never the real state dir or scratchpad root, and SHALL record live evidence on the dev Mac in which a dry run over a non-empty known candidate set and its apply agree, with the excluded entries and an outside symlink target preserved.

## Generated And Changed Surfaces

- `pkg/config/schema_disk.go`, `[NEW] pkg/config/schema_disk_reclaim.go`: the `reclaim` and `lease` keys.
- `pkg/diskspace/`: `[NEW] state.go`, `ledger.go`, `reclaim.go`, `reclaim_worktrees.go`, `reclaim_scratch.go`, `lease.go`, `events.go`, `samples.go`; `treeLimits` in `sizes.go` and `remove_tree_unix.go`; pipeline temp worktrees in `worktrees.go`.
- `internal/cli/`: `[NEW] disk_state.go`, `disk_register.go`, `disk_reclaim.go`, `disk_lease.go` (+ `_unix`, `_other`), `disk_log.go`, `disk_status_extra.go`, `rules_sticky_disk.go`; `disk.go`, `disk_status.go`, `disk_check.go`, `disk_trim.go`, `rules_sticky.go`.
- `pkg/rulecond/sticky.go`: `StickyOptions` and the alert append.
- Templates and content listed in REQ-DISK2-GUIDE-01, the codex native body, `docs/disk-management.md`, `README.md` rows; regenerated goldens recorded in `recapture.md`.

## Related SPECs

- SPEC-DISK-001: floor, check, status, trim; this SPEC reuses its roots, removal, and worktree classification.
- SPEC-STICKYRULE-001: the hook that carries the alert.

## Traceability Matrix

| Requirement | Plan task | Acceptance | Invariants |
|---|---|---|---|
| REQ-DISK2-SCOPE-01 | T9 | S10 | INV-E12 |
| REQ-DISK2-CFG-01, CFG-02 | T1 | S1 | INV-E11 |
| REQ-DISK2-STATE-01, STATE-02 | T2 | S2, S12 | INV-E01, INV-E10 |
| REQ-DISK2-REG-01..05 | T3 | S3 | INV-E01, INV-E02 |
| REQ-DISK2-RECLAIM-01..03 | T4 | S4 | INV-E02, INV-E03, INV-E04 |
| REQ-DISK2-RECLAIM-04..07 | T5 | S5 | INV-E05, INV-E06 |
| REQ-DISK2-LEASE-01..04 | T6 | S6 | INV-E07, INV-E08 |
| REQ-DISK2-LOG-01..04 | T7 | S7 | INV-E09 |
| REQ-DISK2-STATUS-01 | T7 | S7 | INV-E09 |
| REQ-DISK2-ALERT-01, ALERT-02 | T8 | S8 | INV-E10 |
| REQ-DISK2-GUIDE-01, GUIDE-02 | T9 | S9 | INV-E12 |
| REQ-DISK2-VERIFY-01 | T10 | S11 | INV-E03, INV-E05 |
