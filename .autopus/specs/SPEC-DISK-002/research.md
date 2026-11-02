# SPEC-DISK-002 Research

## Existing Code Analysis

- `pkg/diskspace/worktrees.go`: `ScanWorktrees` lists worktrees with `--porcelain -z`, classifies `Clean` and `Merged` as tristates against a base ladder (`origin/HEAD`, `origin/main`, `main`, `master`), and marks harness worktrees. Reclaim reuses it and adds the pipeline temp worktrees to the harness definition.
- `pkg/diskspace/remove_unix.go`, `remove_tree_unix.go`: descriptor-relative removal with a pre-walk that stops on any symlink or special file, a device change, or 100,000 entries / depth 32, and a removal pass that required every recorded leaf to still be a regular file. Reclaim adds `treeLimits`, records each leaf's file type, and re-identifies by identity and type.
- `pkg/diskspace/scratchpads.go`: observation of the scratchpad root only.
- `pkg/filelock` (exclusive lock with wait, inode-checked), `pkg/oslock` (raw `flock` try-lock; `LockFileEx` on Windows).
- `pkg/rulecond/sticky.go`: `stickyRun` buffered the payload and returned early on a missing manifest, an undecodable payload, an unusable counter, or an off-cadence index; the rule bodies are capped at `MaxStickyContextBytes` (6,000).
- `pkg/pipeline/worktree.go`: creates `autopus-worktree-*` worktrees under the temp dir, never recorded anywhere.
- `pkg/spec/quality_preflight.go`, `quality_preflight_probe.go`, `parser.go`, `gherkin_parser.go`: required sections, probe table shape, EARS and Gherkin parsing that this SPEC's format follows.

## Outcome Lock

- User-visible outcome: finished, owned worktrees and idle session scratchpads are reclaimed by `auto disk reclaim --apply` with no human decision and with re-qualification before every removal; heavy commands wrapped in `auto disk lease` wait for space instead of racing; status shows the reclaim preview, leases, and trend; a Claude Code session is told on every turn while free space is below the floor; every apply, refusal, and alert is logged.
- Mandatory requirements: REQ-DISK2-SCOPE-01 through REQ-DISK2-VERIFY-01 in `spec.md`.
- Explicit non-goals: branch deletion, unowned or foreign-owned worktrees, scratch databases, VM and container images, model caches, other registries, loose scratchpad-root entries, installing schedulers, throttling unwrapped processes, alerts outside Claude Code.
- Completion evidence: S1 through S12 of `acceptance.md`, the source-lines gate, `go vet` for darwin, linux, and windows, and `## Live Evidence` below.

## Visual Planning Brief

`plan.md` carries the Mermaid flow. In prose: one measurement feeds every command; register and release edit a locked ledger; reclaim selects owned worktrees and idle sessions, re-qualifies each right before removal, and removes worktrees through git and sessions through descriptors; lease admits under one lock and holds a flock the command inherits; status adds three read-only sections; the sticky hook appends an alert below the floor.

## Probe Evidence (2026-11-02, dev Mac)

- `/private/tmp/claude-502` held 58 GiB. One session directory of the autopus-desktop project held 54 GiB (`scratchpad/` with Cargo `target/` trees), 115,005 entries, 117 symlinks; its top directory was last modified 2026-10-24.
- Loose entries at the root (`s3/` with two Cargo targets of 30 and 24 GiB, `av/_lima`) are outside the session layout and stay report-only.
- B1 to B3 of the plan's probe table ran as the named tests on 2026-11-02 and passed.

## Trust Boundary

Paths come from git output, the ledger (written by the same uid into a state dir only that uid can write), and directory listings. Every scratchpad removal is relative to an opened, re-identified root descriptor. Worktree removal is performed by git on a path re-identified, owner-checked, and re-qualified immediately before. A process that changes a worktree between that re-check and git's own checks is the residual race; git still refuses it when the change leaves the worktree dirty.

## Design Decisions

- Ownership by harness location plus ledger, not by path pattern alone: a user's long-lived worktree outside the harness directories is never removed unless its owner registered it, and never when another uid owns it.
- Released means "owner says done" and skips the merge and idle gates but never the clean gate; a released worktree with no branch and an unmerged HEAD is kept, because removing it would drop the only ref to its commits.
- Git performs worktree removal; it is the authority on dirty and locked state.
- Scratchpad idle defaults to 24 hours because sessions are resumable; the newest-mtime walk makes any write inside the tree reset it.
- Lease reservations are not decremented as a command consumes space, so admission is conservative. The command inherits the lease descriptor so the reservation outlives a killed wrapper; `SIGINT` is not forwarded because a terminal already delivers it to the command, and a second copy reads as a forced abort to tools such as cargo.
- Automatic reclaim on a lease shortage runs only for space refusals: deleting files cannot lower the number of live leases.
- The alert travels through the existing sticky hook, outside its rule cap, rather than a second per-prompt process.
- Pipeline worktrees are recognised by location instead of a ledger write, so pipeline tests never write a user's state dir.

## Technology Stack Decision

Go standard library, `golang.org/x/sys/unix` (already a dependency for SPEC-DISK-001's removal), `pkg/filelock`, and `pkg/oslock`. No new module.

## Minimality Decision Matrix

| Option | Chosen | Why |
|---|---|---|
| Ledger plus harness locations | yes | Smallest ownership record that separates agent work from user work |
| Path-pattern ownership only | no | Cannot tell an agent's `wt/` checkout from a user's |
| Pid files for leases | no | Pid reuse and crash cleanup need bookkeeping; a flock needs none |
| A second hook for alerts | no | One more process before every prompt; the sticky hook already runs |
| Scheduler installation | no | User-level persistence; documented instead |

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-E01 | the state that grants deletion belongs to the user alone | ownership and permission boundary | state dir refusal, ledger read | S2, S3, S12 |
| INV-E02 | only worktrees the harness created or the owner registered, owned by the current uid, are reclaimable | ownership predicate | candidate set, `not_owned`, `owner` | S3, S4 |
| INV-E03 | a worktree is removed only while every gate that selected it still holds | re-qualification before effect | `skipped_changed`, surviving worktrees | S4, S11 |
| INV-E04 | reclaim never loses a commit | conservation of refs | branches after apply, `detached_unmerged` | S4 |
| INV-E05 | scratchpad removal never follows a link or leaves the root | containment boundary | surviving outside target, `skipped_device` | S5, S11 |
| INV-E06 | a session in use is not removed | liveness by newest mtime and cwd | `recent`, `cwd`, `skipped_changed` | S5 |
| INV-E07 | admitted commands never take free space below the floor together | numeric admission rule | admission, refusal line | S6 |
| INV-E08 | a reservation lives exactly as long as its command | lifetime | admission after wrapper or command death | S6 |
| INV-E09 | the log and trend are bounded and consistent under concurrency | serialization and bounds | rotated file, event counts, samples | S7 |
| INV-E10 | the alert never weakens the hook | exit-zero, write-once, byte identity | sticky stdout, exit code | S8, S12 |
| INV-E11 | safe, machine-neutral defaults | default and validation table | effective values, load errors | S1 |
| INV-E12 | guidance tells every platform to register, lease, and pause | token presence and parity | guidance surfaces, harness eval | S9, S10 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---|---|---|
| Configuration | T1, S1 | covered |
| State dir and failure policy | T2, S2, S12 | covered |
| Ledger, ownership, pipeline worktrees | T3, S3, S4 | covered |
| Worktree reclaim with re-qualification | T4, S4 | covered |
| Scratchpad reclaim | T5, S5 | covered |
| Leases | T6, S6 | covered |
| Log, trend, status | T7, S7 | covered |
| Alert | T8, S8 | covered |
| Guidance and docs | T9, S9 | covered |
| Live evidence | T10, S11 | covered |
| Scratch DBs, VM and container images, registries, scheduler install | explicit non-goals | out of scope by Outcome Lock |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Reclaim scratch databases, Tart VM clones, and container images the harness created | Needs creator hooks in tools outside this repository | A harness command starts creating them |
| Decrement a lease's reservation as its command writes | Needs per-command disk accounting | Leases prove too conservative in practice |
| Alerts on Codex, Gemini, OpenCode, and OMP | No per-prompt hook surface there | Those platforms ship one |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | One change story: own, reclaim, coordinate, and tell, on one state dir | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `pkg/diskspace/worktrees.go`, `remove_unix.go`, `remove_tree_unix.go`, `sizes.go`, `apply.go`, `scratchpads.go`, `internal/cli/disk*.go`, `pkg/rulecond/sticky.go`, `internal/cli/rules_sticky.go`, `pkg/pipeline/worktree.go`, `pkg/filelock`, `pkg/oslock` | existing | read on 2026-11-02 |
| `pkg/spec/quality_preflight.go`, `quality_preflight_probe.go`, `parser.go`, `gherkin_parser.go`, `cmd/source-lines/main.go` | existing | cited by the review; formats followed |
| every file marked `[NEW]` in `plan.md` | planned addition, implemented on branch `feat/disk-reclaim` | `go test` over the packages |

## Reviewer Brief

- Intended scope: ledger, reclaim, leases, events and trend, status additions, sticky alert, guidance, as the remaining slices of issue #198.
- Explicit non-goals: see the Outcome Lock.
- Self-verified: Traceability over 25 requirements with invariants, three executed probes, the Review Response table below.
- Reviewer should focus on: whether REQ-DISK2-RECLAIM-03's re-qualification closes every gate that selected a worktree, whether the leaf-unlink removal can reach outside the root, whether the lease descriptor inheritance has a lifetime gap, and whether the alert can ever change the sticky output at or above the floor.

## Live Evidence

Recorded on the dev Mac on 2026-11-02 at branch `feat/disk-reclaim`.

**Read-only dry run against the real scratchpad root** (`auto disk reclaim --dir autopus-adk`, built from the branch, `XDG_STATE_HOME` pointed at a scratch dir): 35 idle session directories selected, all empty (0 B; `du -sk` confirms 0), 14 kept as `recent`, and 3 unowned worktrees of the workspace kept as `not_owned`. The 54 GiB autopus-desktop session was among the `recent` ones: a background task of a live session had written its `tasks/*.output` within the day, so the newest-mtime rule kept it, as intended. Nothing was applied to the real root.

**Apply on copies** (a throwaway driver over `pkg/diskspace`, deleted after the run; scratch root mode 0700):

| Entry | Source | Dry run | After apply |
|---|---|---|---|
| `-Users-…-memoreel/56589337-…` | copied from the real root with mtimes | candidate, 0 B, idle 2935m | removed |
| `-Users-…-msgmcp/6362d5e6-…` | copied | candidate, 0 B, idle 2945m | removed |
| `-private-var-…-judgment-032336cb-…/b3d968ca-…` | copied | candidate, 0 B, idle 2853m | removed |
| `-proj-live/aaaaaaaa-…` | synthetic: 64 MiB `target/debug/deps/libbig.rlib`, `scratchpad/link -> ../outside.txt` | candidate, 67108864 B, idle 7813m | removed; `outside.txt` still reads `keep` |
| `-proj-live/bbbbbbbb-…` | synthetic, written now | kept `recent` | present |
| `loose` | directory at the root | never considered | present |
| `repo/.worktrees/done` | merged harness worktree with 8 MiB ignored `target/` | candidate `merged_idle`, 8400896 B | removed; branch `done` kept |
| `other` | unowned linked worktree | kept `not_owned` | present |

Counters: scratchpads `{Scratchpads:4 DeletedBytes:67108864 Failed:0}`, worktrees `{Worktrees:1 DeletedBytes:8400896 Failed:0}`. The dry-run set equals the removed set.

## Review Response (codex, 2026-11-02, judge REVISE, 18 accepted)

| Finding | Resolution |
|---|---|
| F-01 state dir permissions | Usable-state-dir definition refuses group/other-writable components; ledger read checks regular file and owner; S2 adds the 0770 case |
| F-02 foreign-owned worktrees | Register, selection, and the pre-removal re-check require the current uid; S3 and S4 oracles |
| F-03 narrow re-check | REQ-DISK2-RECLAIM-03 re-checks HEAD, clean, merge and idle cutoff, or the live released entry; S4 race oracles |
| F-04 detached released worktree | `detached_unmerged` skip; S4 oracle |
| F-05 lease outlived by its command | The command inherits the lease descriptor; probe B1 and S6 oracle |
| F-06 lock files and atomic replace | Lock files defined as never replaced; lease create-flock-fill order defined |
| F-07 log concurrency | Rotation, rate-limited alert append, and sample compaction all under their locks; S7 concurrency oracles |
| F-08 removeTree leaf type | Leaves keep their type and are re-identified by identity and type |
| F-09 alert vs cap and early returns | REQ-DISK2-ALERT-01/02 place the alert outside the cap, independent of every early return, with a time budget |
| F-10 state failure precedence | REQ-DISK2-STATE-02 table; S12 |
| F-11 need overflow, negative wait | 1 PiB bound, NaN rejected, clamped lease needs, `--wait` negative rejected; S6 |
| F-12 EARS and Gherkin parsing | Event-driven requirements use WHEN ... THEN; S9 to S12 are Given/When/Then |
| F-13 required sections | Added to `research.md` and `plan.md`; invariant column in traceability |
| F-14 probe table | Ten-column table with executed status and evidence |
| F-15 missing oracles | Added: cross-repository registration, FIFO leaf, signal exit, state failure, broken config, non-empty live set |
| F-16 scope allowlist | README and this SPEC directory added |
| F-17 source-lines arguments | Command passes the changed Go files |
| F-18 `[NEW]` markers | Added in `plan.md` and `spec.md` |
| F-19 reclaim on count refusal | Reclaim runs only for space refusals |
| F-20 sampling across mounts | Interval per mount and a 4,000-sample cap on compaction |

## Review Response (codex round 2, 2026-11-02, judge REVISE, 6 accepted)

| Finding | Resolution |
|---|---|
| Released worktree can detach and lose its branch after the scan | Re-check requires a HEAD still on a branch or merged; `TestApplyWorktrees_ReleasedLosingItsBranchIsKept` |
| Lease release unlocked the description a grandchild still shares | Release closes without unlocking and removes the file only when nobody holds it; `TestLease_OrphanGrandchildKeepsReservation`; orphaned flag in status |
| Floor and reservation sums can overflow | Saturating admission, negative floor admits nothing, upper bounds on every size and idle key; `TestAdmit_SaturatingArithmetic`, config table |
| Self-Verify Summary lacked the Q-IDs | This section |
| Missing oracles: TERM forwarding, event write failure, unusable state in the alert, device preservation | `TestDiskLease_ForwardsSIGTERM`, `TestDiskReclaim_EventWriteFailureKeepsExitCode`, `TestRulesSticky_UnusableStateStillAlerts`; device: the worktree walk skips a tree crossing a device and the scratchpad pre-walk reuses SPEC-DISK-001's device stop (see Oracle Acceptance Notes) |
| Compaction may stay above 256 KiB | The rewrite keeps at most 2,000 samples (about 200 KB) |

## Security Review Response (2026-11-02, 0 critical, 0 high)

| Finding | Resolution |
|---|---|
| M1 idle values overflow `time.Duration` into "always idle" | Every idle key bounded at 5,256,000 minutes |
| M2 a daemon that inherits the lease holds it forever | Documented; status and JSON mark the lease `orphaned` once the wrapper pid is gone |
| L1 `min_free_gb` and reserves overflow bytes | Bounded at 1 PiB |
| L2 ext4 may reuse a removed worktree's inode at the same path | Residual risk, below; macOS APFS does not reuse inodes |
| L3 hook lock waits up to 2 s each | Bounded at 250 ms on the hook path |
| L4 full argv stored as the label | Default label is the program's base name, cleaned and cut to 200 bytes |
| L5 `git worktree remove` crosses a mount inside the worktree | A worktree whose tree crosses a device is kept (`device`, `skipped_device`) |
| I1 a status probe races a fresh lease's lock | Up to 50 retries of 2 ms |
| I2 `.worktrees/` is also a human convention | Kept as owned, documented in `docs/disk-management.md` |
| I3 depth-2 activity misses deep writes | Activity walks the whole tree |

## Residual Risks

- Linux ext4 can give a worktree recreated at a registered path the removed worktree's inode, so a registered and released entry would match it; the new worktree is then removed once clean (its branch is kept). Releasing before removing a worktree by hand avoids it.
- See also `docs/disk-management.md` `## Known residual risks`.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: every cited symbol was read on 2026-11-02; git, flock, and sticky hook behaviour is backed by probes B1–B3 and tests
- Q-CORR-02 | status: PASS | attempt: 3 | files: spec.md, plan.md | reason: new files carry `[NEW]` in Generated And Changed Surfaces and the tasks
- Q-CORR-03 | status: PASS | attempt: 3 | files: spec.md, acceptance.md | reason: Event-driven requirements use `WHEN ..., THEN`; Unwanted use `IF ..., THEN`; every scenario is Given/When/Then
- Q-CORR-04 | status: PASS | attempt: 3 | files: research.md | reason: Reference Discipline separates existing references from `[NEW]` additions with the verification used
- Q-COMP-01 | status: PASS | attempt: 3 | files: all four | reason: contract, tasks and probes, oracles, evidence and decisions each live in their file
- Q-COMP-02 | status: PASS | attempt: 3 | files: spec.md, acceptance.md | reason: 25 requirements in the matrix; refusals, races, exit codes, and state failures have oracles
- Q-COMP-04 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: Outcome Lock states ownership, re-qualification, lease lifetime, and the alert; residual risks are stated
- Q-COMP-05 | status: PASS | attempt: 3 | files: research.md, spec.md, acceptance.md | reason: INV-E01 to INV-E12 each map to requirements, tasks, and Must oracles with exact values
- Q-COMP-06 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: Traceability Matrix carries the invariant column; Reviewer Brief names the focus areas
- Q-COMP-07 | status: PASS | attempt: 3 | files: research.md | reason: Completion Debt is empty; residual risks are stated properties, and Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 3 | files: plan.md | reason: three probe rows with PASS status and test evidence
