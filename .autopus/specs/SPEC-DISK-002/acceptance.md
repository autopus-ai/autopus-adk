# SPEC-DISK-002 Acceptance Criteria

Units and fixture rules follow SPEC-DISK-001. The state dir, the scratchpad root, the clock, the uid, free space, and the process working directory enter through seams; tests never touch `~/.local/state/autopus` or `/tmp/claude-<uid>` (a test-binary guard points the state dir at a per-process temp dir before any test runs).

## Test Scenarios

### S1: AC-DISK2-001 - Configuration defaults and bounds
Priority: Must
Given an `autopus.yaml` without a `disk` key
When the configuration is loaded and `Disk.Effective()` is read
Then worktree idle is 360, scratchpad idle 1440, max concurrent 0, wait 10, and reclaim on shortage true, and saving writes no `disk` key
And `reclaim: {worktree_idle_minutes: 59}` fails with `disk.reclaim.worktree_idle_minutes must be >= 60`, `reclaim: {scratchpad_idle_minutes: 30}` with `disk.reclaim.scratchpad_idle_minutes must be >= 60`, `lease: {max_concurrent: -1}` with `disk.lease.max_concurrent must be >= 0`, `lease: {wait_minutes: -1}` with `disk.lease.wait_minutes must be >= 0`, and `lease: {max_parallel: 1}` with the strict-decoder unknown-key error
And `lease: {max_concurrent: 2, wait_minutes: 0, reclaim_on_shortage: false}` loads with exactly those values
And `reclaim: {scratchpad_idle_minutes: 153722868}` fails with `disk.reclaim.scratchpad_idle_minutes must be <= 5256000`, `trim: {go_build: {idle_minutes: 153722868}}` with `disk.trim.go_build.idle_minutes must be <= 5256000`, `min_free_gb: 8589934592` with `disk.min_free_gb must be <= 1048576`, and `reserves: {release: 8589934592}` with `disk.reserves.release must be <= 1048576`.

### S2: AC-DISK2-002 - State dir resolution and refusal
Priority: Must
Given `XDG_STATE_HOME` absolute, relative, or empty
When the state dir path is resolved
Then it is `<XDG_STATE_HOME>/autopus/disk` for the absolute value and `<home>/.local/state/autopus/disk` otherwise
And opening it creates the dir and `leases/` with mode 0700
And a symlinked parent is refused with `is a symlink` and the link target stays empty, a state dir that is a file is refused, a relative path is refused with `not an absolute path`, a dir owned by another uid is refused with `is not owned by uid`, and an existing dir with mode 0770 is refused with `writable by group or others`
And an atomic write leaves no temporary file, and an append or read through a planted symlink is refused with `not a regular file`.

### S3: AC-DISK2-003 - Ledger register, release, listing, staleness, corruption, ownership
Priority: Must
Given a fixture repository with a linked worktree W, a plain directory D, and a subdirectory of W
When `auto disk register W --owner agent-1` runs
Then stdout is `disk register: W` and `auto disk registry --json` shows one entry for W with state `live` and owner `agent-1`
And registering W again keeps one entry, with control characters dropped from the owner
And `auto disk register D` exits 1 with stderr exactly `disk register: D is not the top of a linked git worktree`, and so does the subdirectory and the main worktree
And registering into a state whose uid differs from the worktree owner fails with `is not owned by the current user`
And `auto disk release W` prints `disk release: W` and the registry shows `released`; `auto disk release D` prints `disk release: D is not registered` and exits 0
And after W is removed and recreated at the same path, the old entry is not live, and the next register of another worktree drops it
And a ledger file containing `{` makes register exit 1 with stderr starting `disk: ledger <path> is unreadable: ` and leaves the file byte-identical
And six concurrent registers of different worktrees all land.

### S4: AC-DISK2-004 - Worktree reclaim selection, re-qualification, and apply
Priority: Must
Given a repository whose `main` contains branches `br-a` through `br-e`, `br-g`, `br-h` and not `br-f`, with linked worktrees and the clock 400 minutes after their creation:

| worktree | location | registered | released | state | idle | expected |
|---|---|---|---|---|---|---|
| a | `.worktrees/a` | no | — | merged, clean, ignored `target/` 3 MiB | 400m | candidate `merged_idle` |
| b | `.worktrees/b` | no | — | merged, clean, a file touched at 300m | 100m | kept `recent` |
| c | `.worktrees/c` | no | — | merged, untracked file | 400m | kept `dirty` |
| d | `../elsewhere/d` | no | — | merged, clean | 400m | kept `not_owned` |
| e | `../elsewhere/e` | yes | no | merged, clean | 400m | candidate `merged_idle` |
| f | `../elsewhere/f` | yes | yes | `br-f` one commit ahead, clean | 400m | candidate `released` |
| g | `.worktrees/g` | no | — | merged, clean, locked | 400m | kept `locked` |
| h | `.worktrees/h` | no | — | merged, clean, holds the cwd | 400m | kept `cwd` |

When `auto disk reclaim` runs
Then the dry run names exactly a, e, f and the skip counters are exactly `cwd=1 dirty=1 locked=1 not_owned=1 recent=1`, and nothing is deleted
When `auto disk reclaim --apply` runs
Then a, e, f no longer exist, `git worktree list` no longer lists a, branches `br-a`, `br-e`, `br-f` still exist, `a/target` is gone, the ledger is empty, and b, c, d, g, h exist
And when, between scan and apply, a gets a clean commit, e gets a file touched after the cutoff, and f is registered again, all three are `skipped_changed` and survive
And when an untracked file is added to a between scan and apply, a is `skipped_changed`
And a released worktree with no branch whose HEAD is not merged is kept as `detached_unmerged`, while one with a branch is selected as `released`
And a worktree registered from another repository is reclaimed from a project in a different repository
And a released worktree that detaches and deletes its branch between scan and apply is `skipped_changed` and survives
And a pipeline worktree directly under the temp dir named `autopus-worktree-12345` is a harness worktree, while `autopus-worktree-x` and the same name one level deeper are not.

### S5: AC-DISK2-005 - Scratchpad reclaim selection and descriptor-safe removal
Priority: Must
Given a scratchpad root fixture with mode 0700 and the clock now:

| path | newest mtime | contents | expected |
|---|---|---|---|
| `p1/<uuid1>` | 2000m | 3 MiB file, symlink `link -> <outside file>` | candidate |
| `p1/<uuid2>` | 30m (one file) | | kept `recent` |
| `p2/<uuid3>` | 2000m | small file | candidate; `p2` removed afterwards |
| `p1/not-a-uuid` | 2000m | | never considered |
| `loose-dir` | 2000m | | never considered |
| `p3/<uuid4>` | 2000m | holds the cwd | kept `cwd` |

When `ApplyScratch` runs over the scan
Then `p1/<uuid1>` and `p2/<uuid3>` are gone, `p2` is gone, `p1`, `loose-dir`, `p1/not-a-uuid`, and the outside file survive, and the counters show `scratchpads=2 failed=0`
And a FIFO inside `p2/<uuid3>` is unlinked without blocking
And a root with mode 0770, a symlinked root, a relative root, and a root owned by another uid are refused; a missing root is unavailable but not refused
And a tree that gains a file between the scan and the pre-walk is `skipped_changed` and survives
And a scan whose walk bound is 1 entry counts the candidates as `unbounded`.

### S6: AC-DISK2-006 - Lease admission, lifetime, shortage, and exit codes
Priority: Must
Given floor 10 GiB and free 50 GiB
When a lease of 30 GiB is held and a second request of 15 GiB arrives with `--wait 0s` and `reclaim_on_shortage: false`
Then the second exits 3 with stderr exactly `disk lease: 15.0 GiB needed above the floor, 10.0 GiB available after 30.0 GiB reserved by 1 leases; waited 0s`, starts nothing, and logs `lease_refused` with the command as its message
And a request of 10 GiB is admitted, exactly reaching the floor
And with `lease.max_concurrent: 1` and one live lease, a request exits 3 with `disk lease: 1 leases already run (disk.lease.max_concurrent=1); waited 0s`
And with `reclaim_on_shortage: true` a space refusal logs exactly `trim_apply`, `reclaim_apply`, `lease_reclaimed`, `lease_refused` in that order, the trim confined to the fixture cache
And a lease whose holder was killed with SIGKILL is discarded and its file removed at the next admission
And a lease whose wrapper was killed while its command still runs keeps refusing admission until the command dies, then admits within 5 seconds
And a lease whose command exited after starting a background process stays live after the wrapper's release until that process dies
And an overflowed (negative) floor admits nothing
And `SIGTERM` sent to the wrapper reaches a command that traps it with `exit 9`, and the lease exits 9
And `sh -c 'echo hi; exit 7'` under a lease exits 7 with stdout `hi`, `true` exits 0, `sh -c 'kill -TERM $$'` exits 143, a missing command exits 127 with `disk lease: cannot start `, and afterwards no lease file remains
And `--need 0`, `--need 1048577`, `--need NaN`, `--wait -1s`, both `--need` and `--for`, neither, an unknown reserve, no command, and a command without `--` each exit 1 with a `disk lease: ` line and start nothing, while `--for release` with a configured reserve runs the command.

### S7: AC-DISK2-007 - Events, rotation, samples, trend, status additions
Priority: Must
Given an event log just under 1 MiB holding one event and one unparseable line
When one more event is appended
Then the log was rotated into `events.jsonl.1`, and `TailEvents(3)` returns the three events oldest first without the unparseable line
And eight concurrent rate-limited `low_space_alert` appends within 30 minutes log exactly one event, and one 31 minutes later logs again
And samples one minute apart on the same mount append once, alternating mounts each keep their own five-minute interval, and a sample log over 256 KiB of 8-day-old samples is rewritten to the one new sample
And with a sample of 60 GiB free 120 minutes ago and 40 GiB now the trend is `window_minutes=120` and `bytes_per_hour=-10737418240`, and samples younger than 10 minutes, older than 6 hours, or of another mount give no trend
When `auto disk status` runs with a 70 GiB sample two hours old, 50 GiB free, a live 5 GiB lease labelled `build`, and one owned merged idle worktree
Then stdout contains `disk reclaim: worktrees=1 `, `  owned worktree: <path>`, `disk lease: 5.0 GiB for build (pid `, and `free trend: -10.0 GiB/h over 2h0m0s`, after the SPEC-DISK-001 trim block, and JSON carries the trend, one lease, and one worktree candidate
And `auto disk trim --apply` appends one `trim_apply` event.

### S8: AC-DISK2-008 - Sticky alert
Priority: Must
Given a project with the sticky manifest installed and the default floor of 10 GiB
When the hook runs with 5 GiB free on prompt 1 of a session
Then the additional context holds the rule bodies followed by exactly `Disk alert: 5.0 GiB free on /fixture, below the floor of 10.0 GiB (disk.min_free_gb). Do not start builds, worktrees, or parallel agents until space recovers; run \`auto disk status\`, then \`auto disk reclaim --apply\` and \`auto disk trim --apply\`. If you lead other agents, tell each of them to pause heavy work.`
And on prompt 2 of the same session, off cadence, the additional context is exactly that alert
And the two invocations log one `low_space_alert` event
And with 20 GiB free, and with a failing free-space measurement, the output is byte-identical to the output with free space far above the floor
And with an `autopus.yaml` that does not parse and 1 GiB free, the rule bodies are emitted with no alert and the command exits 0
And with a state dir whose parent is a symlink and 5 GiB free, the alert is still emitted.

### S9: AC-DISK2-009 - Guidance parity
Priority: Must
Given the shipping guidance surfaces of REQ-DISK2-GUIDE-01
When they are read and the generated references are compared across platforms
Then each contains `auto disk register`, `auto disk release`, and `auto disk lease --need`
And the delegation surface tells the lead to pause running workers on a disk alert or a failed preflight
And the harness eval task GT-PROMPT-WORKER-HYGIENE still finds the Worker hygiene section byte-identical on every platform
And `docs/disk-management.md` contains the launchd, systemd, and cron examples with no `/Users/` path.

### S10: AC-DISK2-010 - Scope
Priority: Must
Given the branch diff against `origin/main`
When its paths are listed
Then every path is inside the REQ-DISK2-SCOPE-01 allowlist, none is under a generated root, and none is a launchd, systemd, or cron file.

### S11: AC-DISK2-011 - Live evidence
Priority: Must
Given a copy of real session directories under a scratch root with mode 0700 and a fixture repository with one merged harness worktree, one unowned worktree, and a symlink to a file outside the root
When `auto disk reclaim` and then `auto disk reclaim --apply` run on the dev Mac with the scratch seams
Then the dry-run candidate set is non-empty and equals the removed set, the unowned worktree and the outside file survive, and the sizes are recorded in `research.md`.

### S12: AC-DISK2-012 - State failure policy
Priority: Must
Given a state dir path whose parent is a symlink
When `auto disk reclaim --apply`, `auto disk check`, and `auto disk register` run
Then reclaim exits 0 and removes the harness worktree, check exits 0, register exits 1 with a message containing `is refused`, and nothing is written through the link
And with a usable state dir whose `events.jsonl` is a directory, `auto disk reclaim --apply` still exits 0 and removes the harness worktree.

## Oracle Acceptance Notes

S4 and S5 oracles are fixed tables; a test that recomputes the expected set from the implementation's own predicates does not satisfy them. Device-change preservation (REQ-DISK2-RECLAIM-05) reuses SPEC-DISK-001's `childProblem` device oracle, because a second file system cannot be mounted in a unit test; the live run shows no `skipped_device` on a single-volume root.
