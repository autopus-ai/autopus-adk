# SPEC-DISK-001 Research

## Existing Code Analysis

| Path | Role | Finding |
|------|------|---------|
| `internal/cli/react_band_localpatch_disk_darwin.go`, `_linux.go`, `react_band_localpatch_prep.go` lines 26-29 | `lpStatfs`, `lpStatfsUnit`, `lpDiskSpace` | `syscall.Statfs` with `Bavail*Bsize` (darwin) and `Frsize` then `Bsize` (linux); health-band code, mirrored not imported |
| `internal/cli/doctor_text.go` lines 186-191, `doctor_json.go` line 140, `doctor_json_checks.go` lines 255-293 | advisory check pattern | text WARN without touching `allOK`; JSON `warn` check without flipping the envelope |
| `internal/cli/output_json.go` lines 17-54, 65-75, 110-133 | envelope, `jsonFatalError` with `Unwrap`, `writeJSONResultAndExit` | the cause's `ExitCode()` survives unwrapping; `jsonFatalError` is the only error `runRootCommand` does not print |
| `internal/cli/root.go` lines 169-178, 204-210; `rules.go` lines 38-50; `fix_lock.go` lines 21-39 | `exitCoder`, `runRootCommand`, namespace registration, `exitCodeFixUnverified = 3` | `runRootCommand` prints `Error: <msg>` for every error except `isJSONFatalError`, which the `[NEW] alreadyReported` marker generalizes; registration and exit-code shapes copied for `auto disk` |
| `pkg/config/schema.go` lines 65-99, `schema_health_band.go`, `loader.go` lines 16-30 and 54-97, `loader_strict.go` lines 27-41, `migrate.go` lines 181-200 | config | `omitempty` sections; `Load` persists normalization, `LoadPreview` does not; alias `claude` rewrites to `claude-code` |
| `pkg/workflow/merge_discover.go` lines 11-37, `internal/cli/git_porcelain_snapshot.go` lines 39-55, `doctor_runtime_processes.go` lines 34-39 | `parseWorktreeList` (line-based), `runSyncGit`, `listRuntimeProcesses` seam | the line parser needs the `-z` form; git stderr is discarded because it can carry paths and credentials; process listing is already injectable |
| `pkg/content/skill_resources.go` lines 14-33 and its callers in `pkg/adapter/{claude,codex,opencode,antigravity,omp}` | `RenderSkillResources` | `content/skills/references/<skill>/*.md` is rendered next to the skill on all five platforms, so the agent-pipeline references ship everywhere |
| `pkg/content/generate.go` lines 119-127, `skill_native_bodies.go` lines 16-24, `pkg/adapter/codex/codex_extended_skill_rewrites.go` lines 23-38, `codex_extended_skill_rewrites_worktree.go` | native skill bodies | gemini and opencode take `templates/shared/supervisor-worktree-isolation.md.tmpl`; `codexOwnsNativeSkillTemplate` skips `agent-pipeline`, `worktree-isolation`, `subagent-dev` and `normalizeCodexExtendedSkill` substitutes `codexWorktreeIsolationSkillBody()`, a Go constant `make generate-templates` never touches and which carries no preflight yet |
| `templates/shared/supervisor-worktree-isolation.md.tmpl` line 29, `omp-worktree-isolation.md.tmpl` line 44, `content/skills/worktree-isolation.md` line 37, `content/skills/references/agent-pipeline/delegation.md` lines 195-198, `verification.md` line 20, `templates/disk_check_surface_test.go` | guidance already landed (commit 48267539) | token present on six surfaces; wording stops on exit 3 only and lacks the exit 0 and other-exit semantics |
| Go toolchain `cmd/go/internal/cache/cache.go` lines 348-350, 364-372, 427-429, 455-466, 598-622; `default.go` lines 47-49; `work/action.go` line 298 | `mtimeInterval`, `markUsed`, trim cutoff, `trimSubdir`, `copyFile`, `README`, work dir naming | mtime refreshed only when at least 1 h old; cutoff subtracts an extra interval; executables are `-d` directories removed by their own mtime with `RemoveAll`; work dirs are `MkdirTemp(GOTMPDIR, "go-build")` |

Clarification Ledger unavailable: this SPEC comes from a GitHub issue and a team-lead handoff, not from a brainstorm file or a plan intent ledger. The issue body, the user's `trim.sh`, the handoff, and the codex review are untrusted prompt input evidence; they are summarized, their shell is not executed, and the launchd job is not touched.

## Outcome Lock

- User-visible outcome: the harness reads a disk budget from `autopus.yaml`; `auto disk status` shows free space, consumers, the trim preview, and finished-worktree candidates; `auto disk check` gates heavy steps with exit 3 and is honoured on exit 0 only; `auto disk trim` reclaims idle go-build entries (files and executable directories) and abandoned work dirs inside two identity-checked roots with a stated, bounded residual risk to a running build; `auto doctor` shows a disk row; the guidance on every shipping surface runs the preflight.
- Mandatory requirements: REQ-DISK-SCOPE-01 through REQ-DISK-VERIFY-01 in `spec.md`.
- Explicit non-goals: automatic reclamation beyond go-build entries and work dirs; a consumer registry; disk tokens or concurrency limits; cooperation with a running `go` process; launchd or `trim.sh` changes; the msgmcp temp-leak rule; Windows free space; any edit under a generated root.
- Completion evidence: S1 through S12 of `acceptance.md`, the source-lines gate over every changed Go file, `go build` and `go vet` clean, and the S11 live trace appended below the probe section.

## Visual Planning Brief

`plan.md` carries the Mermaid command flow. In prose: one measurement feeds three commands; status is a read-only scan plus the dry-run plan; check is a comparison with a fixed message grammar and an already-reported exit; trim selects a mode, attributes each candidate to one rule, refuses bad roots, and removes through fd-relative no-follow operations after re-identifying every target.

## Probe Evidence (2026-10-31)

Scratchpad paths are redacted to `<scratchpad>`; every probe used only session-owned directories.

- A1, FAIL for the 60-minute rule. Module with `fmt` and `net/http`, `GOCACHE=<scratchpad>/disk-probe/gocache`, warm build produced 21 files over 1 MiB. Every cache file was backdated to 58 minutes old, `main.go` was edited so only `main` recompiles, and a `-toolexec` wrapper held the compile of `main` for 240 s. At T+190 s (mtime age about 61 minutes) `find -type f -size +1M -mmin +60 -delete` removed 21 of 21 files; the held build then failed: `./main.go:4:2: could not import fmt (open ../gocache/aa/aae54b96...-d: no such file or directory)`, `could not import net/http (...)`, `exit=1`. The control run with `-mmin +120` at the same point removed 0 of 21 files and the build exited 0. The control shows that entries looked up 58 minutes after their last refresh survive a 120-minute rule for the next hour; it does not show that any threshold is safe for a build that holds an entry longer than the guaranteed window. Logs: `<scratchpad>/disk-probe/probe-a1.log`, `build-+60.log`, `build-+120.log`.
- A2, PASS. `syscall.Statfs` on the worktree path returned `avail_bytes=158571536384 bsize=4096 mount=/System/Volumes/Data`; `df -k` on the same path in the same second reported `Available` 154855288 KiB, a 272 KiB difference while builds were writing. `/System/Volumes/Data`, `/tmp`, and `$TMPDIR` all resolved to the same mount. The two samples are not simultaneous; the oracle tolerates the drift and the deterministic arithmetic is fixed in S2.
- A3, PASS on the restated oracle. With 2 `Runner.Worker` jobs and 6 `Runner.Listener` daemons live, `ps -axo comm= | grep -c '/Runner.Worker$'` printed 2, equal to the live job count, and never matched a Listener; `ps -axo command | grep -c '[R]unner.Worker'` printed 4 because two `zsh -c` shells carried the name in their arguments. Also observed: six `$TMPDIR/go-build*` directories aged 0 to 201 minutes, `/private/tmp/claude-502` holding 197 project directories and 10 GiB, the real cache at 33 GiB with 147 GiB free on a 926 GiB volume.

## Trust Boundary

The trim command deletes under two roots whose locations come from the environment (`GOCACHE`, `GOTMPDIR`, `TMPDIR`) and from `go env`, which reads the user's `go.env` and `GOENV` files. A hostile or mistaken root is the first attack surface: `GOTMPDIR` pointing at `$HOME` would make any old `go-build123` directory there a candidate by name alone. Both roots are therefore refused by identity (`/`, home, project, relative) and ownership, the cache root also needs Go's `README` marker, and each candidate must be owned by the current uid (REQ-DISK-TRIM-06, TRIM-10). The second surface is the gap between scan and removal: a replaced root, a swapped intermediate directory, a symlink planted inside an executable entry or work dir, or a bind mount under it. Identity `(dev, ino, size, mtime)` re-checked through fd-relative no-follow operations, root identity re-verified before the first unlink, same-device enforcement, and whole-candidate skips close these (REQ-DISK-TRIM-05); `os.Root` is not relied on because it follows internal relative symlinks and does not stop at mount boundaries. The third surface is the running build: mtime bounds how recently Go marked an entry used (N minus 60 minutes), and nothing in this design can see which entries a build currently holds, so the residual risk is stated in the Outcome Lock, the note line, `--help`, and the below-120 warning rather than claimed away. Process names from `autopus.yaml` are matched as bare basenames and rejected with path separators. Git output is read NUL-separated with stderr discarded; a pasted suggestion is single-quoted and withheld for control characters. Output names roots but never file contents, environment values, or credentials.

## Design Decisions

- **Every default idle threshold is at least 120 minutes** (idle 120, ci 180, emergency 120, large files 120); validation keeps 60 as the minimum so the script's values remain reproducible, and every key below 120 warns on each run. Probe A1 is the reason; the residual risk is documented, not eliminated.
- **Entries are what Go stores**: regular files and executable directories named `<xx>/<64 hex>-d`, judged by their own mtime as `markUsed` and `trimSubdir` do. The script's `find -type f` judged executables by an inner mtime Go never refreshes.
- **Containment without `os.Root` guarantees**: identity-checked fd-relative removal, whole-candidate skips, both roots refused by identity and ownership; **work dir liveness by newest tree mtime**, since a build keeps writing below `b001/` while the top directory's mtime stands still.
- **Attribution order large files, mode rule, work dirs** with one deletion per candidate; **required free is `max(floor, need)`**, GiB everywhere, mode order emergency then ci then idle, basename CI detection with unknown treated as running and emergency still winning below the floor.
- **Already-reported errors** (the check command writes its one line and returns an exit-coded error `runRootCommand` does not print), **advisory doctor row**, `LoadPreview` only, mirrored `lpStatfs`, suggestions as quoted commands only.

## Technology Stack Decision

| Mode | Selected stack | Resolved versions | Source refs | Checked at | Rejected alternatives |
|------|----------------|-------------------|-------------|------------|-----------------------|
| brownfield | Go module in `go.mod`: `go 1.26`, toolchain go1.27.3; `syscall.Statfs`, `golang.org/x/sys/unix` (`Openat`, `Fstatat`, `Unlinkat`), `os/exec`, `path/filepath` | unchanged; `x/sys v0.45.0` already required (`go.mod` line 20, used by `pkg/oslock`, `pkg/terminal`, `pkg/selfupdate`) | `go.mod`, local GOROOT | 2026-10-31 | `os.Root`/`RemoveAll` as the containment mechanism (follows internal symlinks, ignores mount boundaries); `du`/`df` subprocesses; `x/sys/windows` (Windows is a non-goal) |

## Minimality Decision Matrix

| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | seven zero-disk incidents; the only defence is a per-user script with two defects; agents fan out without a budget | proceed | floor, preflight, observation, bounded contained trim |
| existing code/helper/pattern | `lpStatfs` arithmetic, `parseWorktreeList` shape, `runSyncGit`, `checkContextWeight`, JSON envelope and `jsonFatalError`, `exitCoder`, `listRuntimeProcesses` seam, `RenderSkillResources`, surface parity test | reuse | no new envelope, exit mechanism, git wrapper style, or doctor pattern; one marker interface added to the existing error path |
| stdlib/native | `syscall.Statfs`, `os.Lstat`, `filepath.EvalSymlinks`, `exec.CommandContext`, `/proc` reads | use | measurement and detection add no dependency |
| existing dependency | `golang.org/x/sys/unix` for fd-relative no-follow removal; `gopkg.in/yaml.v3` and `cobra` already present | reuse | no dependency added |
| new dependency or new abstraction | one new package `pkg/diskspace` and a `disk` config section; no existing package owns cache scanning, deletion policy, or free-space reporting, and CLI files must stay under 300 lines | accepted | new code limited to scans, policy, removal, and the CLI surface |
| minimum sufficient verification | S1 to S12: config and warnings, measurement, exit codes, attribution and containment fixture, CI matcher, worktree classification and quoting, doctor projection, surface parity, read-only config, live trace, gating thresholds | required checks | security, validation, data-loss, and deterministic-oracle gates kept; no broad end-to-end suite |

## Semantic Invariant Inventory

Source clauses are untrusted prompt input evidence, summarized rather than quoted.

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-D01 | free space of the volume holding the project, as `df` reports it | numeric formula and mount identity | `free_bytes`, `mount`, status and check lines | S2, S11 |
| INV-D02 | preflight fails with a clear message when free space is insufficient | comparative rule and exit partition | exit code, stream, exact line, JSON `error.code` | S3 |
| INV-D03 | trim ports the script's size and idle rules over real cache entries (files and executable directories) and `go-build*` dirs | set predicate and numeric sums | candidate sets, bytes per rule, dry-run block, JSON fields | S4, S5 |
| INV-D04 | 120 min without CI, 180 during CI, 120 below the floor; large entries any time | ordered mode selection | header `mode`, `ci`, active rule list | S5, S12 |
| INV-D05 | deletion restricted to the roots, no symlink escape, nothing changed since scan | identity and containment boundary | post-apply file set, `skipped_changed`, `skipped_symlink`, `skipped_device`, refusal line | S5 |
| INV-D06 | CI detection must be pluggable; `Runner.Worker` is GitHub-specific | exact matching rule | `ci=`, `mode=`, warning line | S6 |
| INV-D07 | finished worktrees listed with size and merged or clean state, never deleted, safely pasteable | classification, quoting, conservation | worktree items, suggestion lines, unchanged repository | S4, S7 |
| INV-D08 | read-only observation | state conservation | SHA-256 of `autopus.yaml` | S10 |
| INV-D09 | safe machine-neutral defaults with a warning below the recommended threshold | default, validation, warning table | effective values, load errors, warning lines | S1, S5 |
| INV-D10 | a disk row in doctor that warns below the floor | advisory projection | doctor text line, JSON check, `overall_ok` | S8 |
| INV-D11 | preflight wired into guidance before heavy steps on every platform, honoured on exit 0 only | token presence per shipping surface | surface files, codex body, rendered references | S9 |
| INV-D12 | never `go clean -cache` | forbidden action | recorded `exec` calls per branch | S5 |
| INV-D13 | a candidate matched by several rules is attributed once and deleted once | attribution and deduplication | per-rule counts, `total_bytes`, `deleted` | S5, S12 |
| INV-D14 | a work dir is abandoned only when nothing in its tree was touched within the threshold | liveness by newest mtime | `idle_dirs`, work dir candidates | S4, S5 |
| INV-D15 | roots and candidates belong to the invoking user | ownership boundary | `skipped_owner`, refusal line | S5 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| `disk` configuration, defaults, validation, below-120 warning | T1, T7, S1 | covered |
| Free-space measurement and unsupported OS handling | T2, T8, S2, S8 | covered |
| Status with consumers, seams, JSON, trim preview, worktree listing and quoting | T3, T4, T7, S4, S7 | covered |
| Check exit codes, streams, messages | T2, T7, S3 | covered |
| Trim policy, attribution, containment, ownership, CI detection, no `go clean` | T5, T6, T7, S5, S6, S12 | covered |
| Doctor row | T8, S8 | covered |
| Guidance on every shipping surface including codex | T9, S9 | covered (six surfaces landed; codex body and exit semantics remain in T9) |
| Read-only config, docs, live evidence | T7, T10, T11, S10, S11 | covered |
| Automatic reclamation, registry, tokens, alerts, launchd, build cooperation | explicit non-goals | out of scope by Outcome Lock |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

The residual lookup-to-read risk of mtime-based trimming is a stated property with a user-visible warning and a documented bound, not an unfinished item; removing it would need cooperation from the running `go` process, which is an explicit non-goal.

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Reclaim merged and clean harness worktrees and stale scratchpads automatically | The user chose list-only for this slice; deleting a checkout needs an ownership record the harness does not keep | User explicitly requests it after living with the suggestions |
| A consumer registry recording what each run created, disk tokens or a build-concurrency cap, and a broadcast warning below the floor | Need a write path in every creator and orchestrator state shared across workers; this slice observes what exists | User explicitly requests it |
| Learn which cache entries running `go` processes hold (lock files or a cooperative protocol) to close the residual race | No such protocol exists in the toolchain; the bound and the warning are the honest substitute | Go exposes an in-use signal, or the user accepts a wrapper around `go` |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | One cohesive change story: a budget, a measurement, three commands, one doctor row, and the guidance that calls them | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| every `internal/cli`, `pkg/config`, `pkg/workflow`, `pkg/content`, `pkg/adapter/codex` path and `go.mod` line cited in Existing Code Analysis | existing | read at the cited lines or `rg` on 2026-10-31; `ProviderToPlatform("claude") == "claude-code"`; `codexOwnsNativeSkillTemplate` covers `worktree-isolation` |
| the guidance surfaces, `docs/disk-management.md`, and the `README.md` row of commit 48267539; `cmd/go/internal/cache/cache.go`, `default.go`, `work/action.go` in the go1.27.3 toolchain | existing (landed commit; platform) | `git show --stat` and `rg "auto disk check"`; `sed`/`rg` on the local GOROOT with `markUsed`, `trimSubdir`, `copyFile` read in full |
| `~/Library/Application Support/autopus-go-cache-trim/trim.sh` | existing (user file) | read once, read-only; never edited |
| `[NEW] pkg/config/schema_disk.go` and test, `[NEW] pkg/diskspace/*.go` and tests (`remove_unix.go` included), `[NEW] internal/cli/disk.go`, `disk_status.go`, `disk_check.go`, `disk_trim.go`, `doctor_disk.go` and tests, `[NEW] alreadyReported` marker, `[NEW] exitCodeDiskInsufficient`, `[NEW] newDiskCmd`, `[NEW] Disk DiskConf` on `HarnessConfig`, the codex body preflight step, the `exit 0` wording and token | planned addition | do not exist yet |

## Reviewer Brief

- Intended scope: the `disk` section, free-space measurement, `auto disk status|check|trim`, the doctor row, and the preflight on every shipping surface, as the first slice of issue #198, revised for the codex round-1 findings.
- Explicit non-goals: automatic reclamation beyond go-build entries and work dirs, registries, tokens, alerts, launchd, cooperation with a running `go` process, Windows, generated-root edits.
- Self-verified: Traceability Matrix over 29 requirements, fifteen invariants each mapped to a Must oracle, three executed probes (A1 FAIL recorded with its limits), Reference Discipline with every existing reference read or grepped, and the Review Response table below.
- Reviewer should focus on: whether REQ-DISK-TRIM-05 and TRIM-06 close root replacement, intermediate symlinks, inode replacement, sub-mounts, and foreign-owned roots; whether S5 fixes attribution and counters for every mode and race hook; whether the residual-risk wording is honest everywhere the policy is described; and whether the codex native body is the only remaining guidance gap.

## Open Questions For The User

1. Public floor default of 10 GiB: acceptable? The dev Mac would configure `min_free_gb: 45`, `start_below_gb: 80`, `large_file_below_gb: 60`.
2. Doctor row below the floor: advisory WARN (this SPEC) or a doctor failure?
3. Scratchpad root is Claude Code's `/tmp/claude-<uid>` only; Codex and other session scratch roots are not known and are not reported.

## Review Response (codex, 2026-10-31)

| Finding | Change made or reason rejected |
|---------|--------------------------------|
| DISK-REVIEW-001 | Accepted. Defaults raised to 120/180/120/120 (REQ-DISK-CFG-01); keys below 120 warn (CFG-04, S1); Background, Outcome Lock, TRIM-03 note and `--help`, plan A1 row, and Trust Boundary now state that mtime trimming bounds and cannot eliminate the risk and that re-stat narrows but does not close the lookup-to-read race; the A1 control is described as showing the window, not safety. |
| DISK-REVIEW-002 | Accepted. TRIM-05 rewritten: no guarantee rests on `os.Root`; file identity `(dev, ino, size, mtime)` re-checked before unlink; directories pre-walked and removed with `openat(O_NOFOLLOW)`/`fstatat(AT_SYMLINK_NOFOLLOW)`/`unlinkat`, same device, whole-candidate skip on symlink or device change, root identity re-verified; S5 adds the device-change and ownership hooks. |
| DISK-REVIEW-003 | Accepted. Work dir idle is the newest tree mtime from a bounded walk (Definitions, TRIM-04, STATUS-03); S5 `go-build222` has a fresh inner file under a stale top directory and stays. |
| DISK-REVIEW-004 | Accepted. TRIM-06 applies identity refusal to the work dir root; TRIM-10 requires root and candidate ownership by the current uid; S5 refuses `$GOTMPDIR` pointing at home. |
| DISK-REVIEW-005 | Accepted. S11 and T11 export `GOCACHE`, `GOTMPDIR`, and `TMPDIR` to scratch for the whole sequence, require the `roots:` line to name only scratch paths, and sample the real cache before and after. |
| DISK-REVIEW-006 | Accepted. Cache entries are regular files or directories (Definitions, TRIM-03); idle from the entry's own mtime; size as file size or tree sum; `markUsed` named with line numbers; the script's inner-file mtime defect recorded in Background; S5 adds h8, h9, h10 executable entries. |
| DISK-REVIEW-007 | Accepted. WT-01 parses `--porcelain -z`; WT-02 quotes the path as a POSIX single-quoted word after `--`, replaces control characters, and withholds the command for such paths; S7 adds `wt-f` and `wt-g`. |
| DISK-REVIEW-008 | Accepted. CHECK-02 and CHECK-03 fix text (exact stderr line, no prefix) and JSON (envelope only, empty stderr) with the same exit codes through an already-reported exit-coded error; `root.go` and `output_json.go` join the changed surfaces (T2, T7); S3 runs through `runRootCommand`. |
| DISK-REVIEW-009 | Accepted. TRIM-02 fixes the attribution order (large files, mode rule, work dirs), one deletion per candidate, totals as sums over attributed candidates; S5 and S12 pin per-rule counts where h3 matches two rules. |
| DISK-REVIEW-010 | Accepted. GUIDE-01 lists the surfaces that actually ship: references rendered by `RenderSkillResources` on all platforms, the Claude body, the shared gemini/opencode body and its generated mirror, the OMP body, and the codex native constant that `make generate-templates` never produces; the gemini agent-pipeline top-level body is no longer claimed; T9 and S9 updated. |
| DISK-REVIEW-011 | Accepted. WT-01 defines `clean` as exit 0 plus empty output, `merged` by exit code 0/1/other, `unknown` with a reason on errors or missing paths; WT-02 suggests only when both are true and the worktree is neither locked nor prunable; S7 adds `wt-d` and `wt-e`. |
| DISK-REVIEW-012 | Accepted. Added oracles: `failed` with exit 1 (S5), trim JSON and `--list` fields (S5), `large_file_below_gb` boundary (S12), scratchpad oldest and newest ages (S4), unsupported-OS doctor WARN (S8); source-lines runs over `git diff --name-only origin/main -- '*.go'` (VERIFY-01, T11). |
| DISK-REVIEW-013 | Accepted. STATUS-04 makes the scratchpad root, free function, lister, clock, and uid injection seams; S4 uses a test-owned root and asserts the real root is neither read nor created. |
| DISK-REVIEW-014 | Accepted. `run` (dry-run or apply) separated from `mode` (idle, ci, emergency) in Definitions, TRIM-01 header, and the TRIM-09 JSON contract, which now covers every run kind and is the object `status.trim_preview` reuses (STATUS-01). |
| DISK-REVIEW-015 | Accepted. TRIM-07 and S5: zero `go` invocations when `$GOCACHE` is set, exactly one `go env GOCACHE` when unset, never `clean`. |
| DISK-REVIEW-016 | Accepted. Every Event-driven requirement now reads `WHEN ..., THEN THE SYSTEM SHALL ...`, which `reEventDriven` recognizes; Q-CORR-03 re-verified against `pkg/spec/parser.go`. |
| DISK-REVIEW-017 | Accepted. A3 oracle restated as "basename count equals the live `Runner.Worker` count; command-line count may exceed it", which the recorded 2 versus 4 satisfies; the false positive is recorded as the design reason. |
| DISK-REVIEW-018 | Accepted. S2 fixes the arithmetic deterministically from `Statfs_t` fixtures and treats the live comparison as three timestamped sample pairs; S11 verifies no deletion by the existence of every dry-run candidate, not by file counts. |
| DISK-REVIEW-019 | Accepted. TRIM-08 warning now reads `treating CI as running`, `ci=unknown` is a header value, and the mode stays `emergency` below the floor; S6 adds the unknown-plus-5-GiB row. |
| DISK-REVIEW-020 | Accepted. GUIDE-01 requires three-way exit semantics on every surface (proceed only on exit 0, stop on 3, stop on any other exit) and the `exit 0` token in the parity test; T9 carries the rewording of the six landed surfaces. |

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: `markUsed`, `trimSubdir`, `copyFile` cited with line numbers from the local toolchain; executable directory entries reflected in Definitions and fixtures
- Q-CORR-02 | status: PASS | attempt: 2 | files: spec.md, plan.md, research.md | reason: new package, files, marker interface, command, constant, and config field carry `[NEW]`
- Q-CORR-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: Event-driven sentences carry `WHEN ..., THEN`; Unwanted carry `IF ..., THEN`; scenarios use `### S<N>:` with bare steps
- Q-CORR-04 | status: PASS | attempt: 2 | files: research.md | reason: Reference Discipline names the codex native source, the rendered references, the landed commit, and `[NEW]` items separately with the verification used
- Q-COMP-01 | status: PASS | attempt: 2 | files: all four | reason: spec fixes the contract, plan the tasks and probes, acceptance the oracles, research the evidence, decisions, and review response
- Q-COMP-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: all 29 requirements appear in the matrix; S12 and the added oracles cover failed exits, JSON fields, boundaries, scratchpad ages, unsupported OS
- Q-COMP-03 | status: PASS | attempt: 2 | files: spec.md | reason: JSON `run` and `mode` enums, worktree `unknown` states, and preflight exit semantics are observable in named outputs
- Q-COMP-04 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: Outcome Lock states the bounded residual risk, identity-checked containment, executable entries, and every shipping surface including codex
- Q-COMP-05 | status: PASS | attempt: 2 | files: research.md, spec.md, plan.md, acceptance.md | reason: INV-D01 to INV-D15 each map to a requirement, a task, and a Must oracle with exact values or an explicit tolerance, including attribution, liveness, and ownership
- Q-COMP-06 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: Traceability Matrix complete; Reviewer Brief names four focus areas and the non-goals
- Q-COMP-07 | status: PASS | attempt: 2 | files: research.md | reason: Completion Debt is empty with the residual risk explained as a stated property; Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md, research.md | reason: A1 FAIL with its limits, A2 and A3 PASS on oracles consistent with the recorded observations, each with an evidence ref
- Q-FEAS-01 | status: PASS | attempt: 2 | files: plan.md | reason: runtime in `pkg/diskspace` and `internal/cli`, config in `pkg/config`, guidance in `content/`, `templates/shared/`, and the codex adapter constant
- Q-FEAS-02 | status: PASS | attempt: 2 | files: spec.md, plan.md | reason: the codex native body is owned as source; generated roots excluded; references ship through `RenderSkillResources`
- Q-FEAS-03 | status: PASS | attempt: 2 | files: plan.md, acceptance.md | reason: live apply isolated by exported scratch roots; S4 uses injected fixture roots; live oracles tolerate concurrent writers
- Q-STYLE-01 | status: PASS | attempt: 2 | files: spec.md | reason: no should, might, could, possibly, maybe, perhaps in requirement text
- Q-STYLE-02 | status: PASS | attempt: 2 | files: spec.md | reason: Priority and Type are separate lines; Must only
- Q-STYLE-03 | status: PASS | attempt: 2 | files: acceptance.md | reason: bare step keywords; tables sit between When and Then
- Q-SEC-01 | status: PASS | attempt: 2 | files: research.md, spec.md | reason: Trust Boundary covers both environment-derived roots, the scan-to-removal gap, the running build, process names, git output, and pasted paths
- Q-SEC-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: root identity and ownership refusals, fd-relative no-follow removal, quoting and control-character handling, with S5 and S7 oracles
- Q-SEC-03 | status: PASS | attempt: 2 | files: spec.md | reason: no new persistent artifact; output carries roots but no contents or credentials
- Q-COH-01 | status: PASS | attempt: 2 | files: spec.md | reason: one problem, one package, one namespace
- Q-COH-02 | status: PASS | attempt: 2 | files: research.md | reason: follow-on asks are non-goals listed without identifiers; the safety findings were fixed inside this SPEC
- Q-COH-03 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: no sibling SPEC

## Security Review Response (security-auditor, 2026-11-01)

| Finding | Severity | Change |
|---|---|---|
| M1 `go env GOCACHE` follows a `go.mod`/`go.work` `toolchain` line above the temp dir and downloads/execs a toolchain (reproduced) | medium | REQ-DISK-TRIM-07: run it with `GOTOOLCHAIN=local` and `GOWORK=off` |
| M2 sticky work-dir root accepted for any owner; a sticky dir's owner can rename any entry into `go-build<N>` | medium | REQ-DISK-TRIM-10: only a uid-0 sticky root is accepted |
| M3 directory pre-walk re-opens by name and is not tied to the re-verified identity | medium | REQ-DISK-TRIM-05: walked top `(dev, ino)` must equal the candidate identity, else `skipped_changed` |
| M4 a user directory named `go-build<N>` under a mis-set `$GOTMPDIR` was deleted (reproduced, incl. the project dir) | medium | REQ-DISK-TRIM-04: refuse candidates equal to the project, home, or cwd; require every top-level child to be a `b[0-9]{3,}` directory (verified with `go build -work` and `go test -work`: only `bNNN` children); `skipped_shape` counter |
| L1 empty `$HOME` silently disables the home refusal | low | REQ-DISK-TRIM-06: fall back to `user.Current`, else refuse as `home_unknown` |
| L2 a foreign-owned non-cache root is walked before refusal | low | REQ-DISK-TRIM-06: README checked before owner; refused roots are not walked |
| L3 display sanitiser misses bidi overrides and other non-printables | low | REQ-DISK-WT-02: `strconv.IsPrint` and invalid UTF-8 |
| L4 `ps` prints `(name)` for some processes; hidepid hides other uids on Linux | low | REQ-DISK-TRIM-08: strip one pair of parentheses; hidepid left documented as a known false-idle path (window narrows 180 to 120 minutes, no boundary loss) |
| L5 a `go run`/`go test` binary running over 6 h without writes leaves its work dir looking abandoned | low | Not changed in this slice: the running inode survives unlink on unix; recorded as residual risk in docs |
| I1 a regular file on another device inside a directory candidate was not treated as a device crossing | info | REQ-DISK-TRIM-05: any entry on another device skips the candidate |
