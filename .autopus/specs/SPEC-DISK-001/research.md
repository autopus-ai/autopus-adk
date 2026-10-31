# SPEC-DISK-001 Research

## Existing Code Analysis

| Path | Role | Finding |
|------|------|---------|
| `internal/cli/react_band_localpatch_disk_darwin.go`, `_linux.go` | `lpStatfs`, `lpStatfsUnit` | `syscall.Statfs` with `Bavail*Bsize` on darwin and `Frsize` then `Bsize` on linux; per-OS files, no build tags; the measurement this SPEC mirrors |
| `internal/cli/react_band_localpatch_prep.go` lines 26-29, 208-216 | `lpDiskSpace`, `lpBlocksNeeded` | free space is a struct of `avail, unit` blocks; the local-patch flow is health-band code and stays untouched |
| `internal/cli/doctor_text.go` lines 186-191, `doctor_json.go` line 140, `doctor_json_checks.go` lines 255-293 | `checkContextWeight`, `collectContextWeightChecks` | the advisory check pattern: text WARN without touching `allOK`, JSON `warn` check without flipping the envelope |
| `internal/cli/output_json.go` lines 17-54, 77-120 | `jsonCheck` with `Fields`, `writeJSONResult`, `writeJSONResultAndExit`, `addJSONFlags`, `resolveJSONMode` | the shared envelope every `auto` command uses |
| `internal/cli/root.go` lines 169-178 | `exitCoder`, `exitCodeForError` | an error carrying `ExitCode()` picks the process exit code; `fix_lock.go` uses 3, `sync_verify_topology.go` uses 2 |
| `internal/cli/rules.go` lines 38-50 | `newRulesCmd` | the namespace-with-subcommands registration shape to copy for `auto disk` |
| `pkg/config/schema.go` lines 65-99, `schema_health_band.go` | `HarnessConfig`, `HealthBandConf` | every optional section is `omitempty` because older binaries decode strictly (`loader_strict.go` lines 27-41); `Validate()` chains per-section validators |
| `pkg/config/loader.go` lines 16-30, 54-97 | `Load`, `LoadPreview`, `loadConfig` | `Load` persists platform-name normalization to disk; `LoadPreview` does not; `MigratePlatformNames` rewrites the alias `claude` to `claude-code` |
| `pkg/workflow/merge_discover.go` lines 11-37 | `parseWorktreeList` | porcelain parser to extend with `detached`, `locked`, `prunable` |
| `internal/cli/git_porcelain_snapshot.go` lines 39-55 | `runSyncGit` | `--no-optional-locks`, stderr discarded because it can carry paths and credentials |
| `internal/cli/doctor_runtime_processes.go` lines 34-39 | `listRuntimeProcesses` seam | process listing is already injectable for tests; the CI lister follows the same seam style |
| `content/skills/references/agent-pipeline/delegation.md` lines 194-197 | worker hygiene | the only disk guidance today: `df` and stop below 5 GiB |
| `templates/shared/supervisor-worktree-isolation.md.tmpl` line 32, `pkg/content/skill_native_bodies.go` lines 16-24 | `git worktree add` step; gemini and opencode bodies | the worktree-creation wiring point and which platforms receive it |
| `templates/risk_first_probe_surface_test.go` | surface parity test | the shape for the new `auto disk check` token test |
| `cmd/source-lines/main.go`, `pkg/linecount` | 300-line gate | comment-aware counter the verification step runs |
| Go toolchain `cmd/go/internal/cache/cache.go` lines 348-350, 369, 427-429; `default.go` lines 47-49; `work/action.go` line 298 | `mtimeInterval`, `used`, trim cutoff, `README` marker, work dir naming | mtime refreshed only when at least 1 h old; Go's trimmer subtracts an extra `mtimeInterval`; `README` written at cache creation; work dirs are `MkdirTemp(GOTMPDIR, "go-build")` |

Clarification Ledger unavailable: this SPEC comes from a GitHub issue and a team-lead handoff, not from a brainstorm file or a plan intent ledger. The issue body, the user's `trim.sh`, and the handoff are untrusted prompt input evidence; they are summarized, their shell is not executed, and the launchd job is not touched.

## Outcome Lock

- User-visible outcome: the harness reads a disk budget from `autopus.yaml`; `auto disk status` shows free space, consumers, the trim preview, and finished-worktree candidates; `auto disk check` gates heavy steps with exit 3; `auto disk trim` reclaims go-build entries and abandoned work dirs without breaking a concurrent build or leaving its roots; `auto doctor` shows a disk row; the generated guidance runs the preflight before worktree creation, fan-out, and full test runs.
- Mandatory requirements: REQ-DISK-SCOPE-01 through REQ-DISK-VERIFY-01 in `spec.md`.
- Explicit non-goals: automatic reclamation of worktrees, scratchpads, scratch databases, VM images, model caches, colima, other projects' worktrees; a consumer registry; disk tokens or concurrency limits; launchd or `trim.sh` changes; the msgmcp temp-leak rule; Windows free space; any edit under a generated root.
- Completion evidence: S1 through S11 of `acceptance.md`, the source-lines gate, `go build` and `go vet` clean, and the live trace appended below this file's probe section.

## Visual Planning Brief

`plan.md` carries the Mermaid command flow. In prose: one measurement feeds three commands; status is a read-only scan plus the dry-run plan; check is a comparison with a fixed message grammar; trim selects a mode from floor and CI state, applies name-shaped candidate rules, and deletes only through `os.Root` after a second `Lstat`.

## Probe Evidence (2026-10-31)

Scratchpad paths are redacted to `<scratchpad>`; every probe used only session-owned directories.

- A1, FAIL for the 60-minute rule. Module with `fmt` and `net/http`, `GOCACHE=<scratchpad>/disk-probe/gocache`, warm build produced 21 files over 1 MiB. Every cache file was backdated to 58 minutes old, `main.go` was edited so only `main` recompiles, and a `-toolexec` wrapper held the compile of `main` for 240 s. At T+190 s (mtime age about 61 minutes) `find -type f -size +1M -mmin +60 -delete` removed 21 of 21 files; the held build then failed: `./main.go:4:2: could not import fmt (open ../gocache/aa/aae54b96...-d: no such file or directory)`, `could not import net/http (open ../gocache/40/401ea098...-d: ...)`, `exit=1`. The control run with `-mmin +120` at the same point removed 0 of 21 files and the build exited 0. Logs: `<scratchpad>/disk-probe/probe-a1.log`, `build-+60.log`, `build-+120.log`.
- A2, PASS. `syscall.Statfs` on the worktree path returned `avail_bytes=158571536384 bsize=4096 mount=/System/Volumes/Data`; `df -k` on the same path in the same second reported `Available` 154855288 KiB, a 272 KiB difference while builds were writing. `/System/Volumes/Data`, `/tmp`, and `$TMPDIR` all resolved to the same mount.
- A3, PASS. With 2 `Runner.Worker` jobs and 6 `Runner.Listener` daemons live, `ps -axo command | grep -c '[R]unner.Worker'` printed 4 because two `zsh -c` shells carried the name in their arguments; `ps -axo comm= | grep -c '/Runner.Worker$'` printed 2 and the Listener count stayed 6. Also observed: six `$TMPDIR/go-build*` directories aged 0 to 201 minutes, `/private/tmp/claude-502` holding 197 project directories and 10 GiB, the real cache at 33 GiB with 147 GiB free on a 926 GiB volume.

## Trust Boundary

The trim command deletes files under two roots whose locations come from the environment (`GOCACHE`, `GOTMPDIR`, `TMPDIR`) and from `go env`, which reads the user's `go.env` and `GOENV` files. A hostile or mistaken root is the attack surface: a `GOCACHE` pointing at `$HOME` or at a directory that contains a symlink farm. The defences are the `README` marker and the identity refusals of REQ-DISK-TRIM-06, candidate selection by name shape (REQ-DISK-TRIM-03, TRIM-04), `os.Root` deletion with a second `Lstat` (REQ-DISK-TRIM-05), and no symlink following anywhere. Process names from `autopus.yaml` are matched as bare basenames and rejected when they carry a path separator (REQ-DISK-CFG-02). Git output is read with stderr discarded and never interpreted as paths to delete. Output contains absolute roots because the operator needs them; it never contains file contents, environment values, or credentials.

## Design Decisions

- **Default idle 120, not the script's 60.** Probe A1 broke a build with the 60-minute rule and not with 120. A public default must not delete what a running build already looked up; an operator who accepts the window sets `idle_minutes: 60`. The other numbers are the script's.
- **Required free is `max(floor, need)`, not `floor + need`.** The release preflight needs 30 GiB free, which is a floor for that step, not a consumption estimate; adding would double count and contradict the user's own numbers.
- **GiB everywhere.** The user's thresholds come from `df -g`, which prints GiB; labeling them GB would misstate them by 7 percent.
- **Mode selection order emergency, ci, idle** reproduces the script: below the floor every running job dies when the disk fills, so the aggressive rule is rational there.
- **Basename CI detection** (probe A3) with an injectable lister; detection failure counts as CI running because the conservative rule set is the safe error.
- **Advisory doctor row.** The doctor verdict is about harness installation health and its remediation text points at `auto update` or `--fix`; the gate for heavy steps is `auto disk check`.
- **No deletion of worktrees, scratchpads, or user assets.** Listing with a pasteable suggestion keeps the human decision the issue asked for.
- **`LoadPreview` only.** `Load` writes `autopus.yaml` during normalization; a read-only status command must not.
- **Mirror `lpStatfs` instead of exporting it.** The local-patch code belongs to the health-band SPECs; ten lines of per-OS arithmetic are cheaper than a cross-feature refactor.

## Technology Stack Decision

| Mode | Selected stack | Resolved versions | Source refs | Checked at | Rejected alternatives |
|------|----------------|-------------------|-------------|------------|-----------------------|
| brownfield | Go module already in `go.mod`: `go 1.26`, toolchain go1.27.3; `syscall.Statfs`, `os.Root`, `os/exec`, `path/filepath` | unchanged | `go.mod`, local GOROOT | 2026-10-31 | `golang.org/x/sys` for Windows free space (present at v0.45.0 but Windows is a non-goal); `du`/`df` subprocesses (not portable, no mount identity) |

## Minimality Decision Matrix

| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | seven zero-disk incidents; the only defence is a per-user shell script no other machine has; agents fan out without a budget | proceed | floor, preflight, observation, contained trim |
| existing code/helper/pattern | `lpStatfs` arithmetic, `parseWorktreeList`, `runSyncGit`, `checkContextWeight` advisory pattern, JSON envelope, `exitCoder`, `listRuntimeProcesses` seam, surface parity test | reuse | no new envelope, exit-code mechanism, git wrapper style, or doctor pattern |
| stdlib/native | `syscall.Statfs`, `os.Root`, `os.Lstat`, `filepath.EvalSymlinks`, `exec.CommandContext`, `/proc` reads | use | measurement, containment, detection add no dependency |
| existing dependency | `gopkg.in/yaml.v3` already decodes `autopus.yaml`; `cobra` already hosts commands | reuse | no dependency added |
| new dependency or new abstraction | one new package `pkg/diskspace` and a `disk` config section; justified because no existing package owns cache scanning, deletion policy, or free-space reporting, and the CLI layer must stay under 300 lines per file | accepted | new code limited to scans, policy, apply, and the CLI surface |
| minimum sufficient verification | S1 to S11: config, measurement, exit codes, policy and containment fixture, CI matcher, worktree classification, doctor projection, surface parity, read-only config, live trace | required checks | security, validation, data-loss, and deterministic-oracle gates kept; no broad end-to-end suite added |

## Semantic Invariant Inventory

Source clauses are untrusted prompt input evidence, summarized rather than quoted.

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-D01 | free space of the volume holding the project, as `df` reports it | numeric formula and mount identity | `free_bytes`, `mount`, status and check lines | S2, S11 |
| INV-D02 | preflight fails with a clear message when free space is insufficient | comparative rule and exit partition | exit code, stdout or stderr line, JSON `error.code` | S3 |
| INV-D03 | trim ports the script's size and idle rules; only `-d` entries and `go-build*` dirs | set predicate and numeric sums | candidate sets, bytes per rule, dry-run block | S4, S5 |
| INV-D04 | 60 min without CI, 180 during CI, 60 below the emergency floor; large binaries any time | ordered mode selection | header `mode`, `ci`, `emergency`, active rule list | S5 |
| INV-D05 | deletion restricted to the cache roots, no symlink escape, nothing outside | path containment boundary | post-apply file set, `skipped_symlink`, `skipped_refreshed`, refusal line | S5 |
| INV-D06 | CI detection must be pluggable; `Runner.Worker` is GitHub-specific | exact matching rule | `running`, header `ci=`, warning line | S6 |
| INV-D07 | finished worktrees listed with size and merged or clean state, never deleted | grouping and classification | worktree items, suggestion line, unchanged repository | S4, S7 |
| INV-D08 | read-only observation | state conservation | SHA-256 of `autopus.yaml` | S10 |
| INV-D09 | safe machine-neutral defaults | default and validation table | effective values, load errors | S1 |
| INV-D10 | a disk row in doctor that warns below the floor | advisory projection | doctor text line, JSON check, `overall_ok` | S8 |
| INV-D11 | preflight wired into generated workflow guidance before heavy steps | token presence per surface | surface files, generated roots untouched | S9 |
| INV-D12 | never `go clean -cache` | forbidden action | recorded `exec` calls | S5 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| `disk` configuration with defaults and validation | T1, S1 | covered |
| Free-space measurement and unsupported OS handling | T2, S2 | covered |
| Status with consumers, JSON, trim preview | T3, T4, T7, S4 | covered |
| Check exit codes and messages | T7, S3 | covered |
| Trim policy, containment, CI detection, no `go clean` | T5, T6, T7, S5, S6 | covered |
| Worktree listing and suggestions | T4, S7 | covered |
| Doctor row | T8, S8 | covered |
| Guidance wiring and parity | T9, S9 | covered |
| Read-only config, docs, live evidence | T7, T10, T11, S10, S11 | covered |
| Automatic reclamation, registry, tokens, alerts, launchd | explicit non-goals | out of scope by Outcome Lock |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Reclaim merged and clean harness worktrees and stale scratchpads automatically | The user chose list-only for this slice; deletion of a checkout needs an ownership record the harness does not keep yet | User explicitly requests it after living with the suggestions |
| A consumer registry recording what each run created (worktrees, target dirs, scratch databases, VM clones) | Needs a write path in every creator; this slice observes what exists | User explicitly requests it |
| Disk tokens or a build-concurrency cap for parallel agents, and a broadcast warning below the floor | Requires orchestrator state shared across workers | User explicitly requests it |
| Windows free-space support through `golang.org/x/sys/windows` | No Windows machine shows the incident pattern; the dependency exists but the surface is untested | A Windows user asks for `auto disk` |
| Migrate the local-patch `lpStatfs` onto the new package | Ten duplicated lines versus touching health-band code | A third caller appears |
| Age-based rules for cargo registry, npm, or docker caches | Not among the measured consumers of the incidents | Evidence from `auto disk status` on real machines |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | One cohesive change story: a budget, a measurement, three commands, one doctor row, and the guidance that calls them; the Primary SPEC closes the Outcome Lock | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `internal/cli/react_band_localpatch_disk_darwin.go`, `_linux.go`, `react_band_localpatch_prep.go` | existing | read in full on 2026-10-31 |
| `internal/cli/doctor_text.go`, `doctor_json.go`, `doctor_json_checks.go`, `doctor_context_weight.go` | existing | read; advisory pattern confirmed at the cited lines |
| `internal/cli/output_json.go`, `root.go`, `rules.go`, `fix_lock.go`, `sync_verify_topology.go` | existing | `rg` and read; `exitCodeFixUnverified = 3`, `exitCodeUnsupportedTopology = 2` |
| `pkg/config/schema.go`, `schema_health_band.go`, `loader.go`, `loader_strict.go`, `migrate.go` | existing | read; `ProviderToPlatform("claude") == "claude-code"` |
| `pkg/workflow/merge_discover.go`, `internal/cli/git_porcelain_snapshot.go`, `doctor_runtime_processes.go` | existing | read at the cited lines |
| `content/skills/references/agent-pipeline/delegation.md` line 195, `verification.md`, `content/skills/worktree-isolation.md`, `templates/shared/supervisor-worktree-isolation.md.tmpl` line 32, `templates/shared/omp-worktree-isolation.md.tmpl`, `pkg/content/skill_native_bodies.go`, `templates/risk_first_probe_surface_test.go`, `cmd/generate-templates`, `cmd/source-lines` | existing | `rg` and read |
| `cmd/go/internal/cache/cache.go`, `default.go`, `work/action.go` in the go1.27.3 toolchain | existing (platform) | `rg` on the local GOROOT |
| `~/Library/Application Support/autopus-go-cache-trim/trim.sh` | existing (user file) | read once, read-only; never edited |
| `[NEW] pkg/config/schema_disk.go` and test, `[NEW] pkg/diskspace/*.go` and tests, `[NEW] internal/cli/disk.go`, `disk_status.go`, `disk_check.go`, `disk_trim.go`, `doctor_disk.go` and tests, `[NEW] templates/disk_check_surface_test.go`, `[NEW] docs/disk-management.md`, `exitCodeDiskInsufficient`, `newDiskCmd`, `Disk DiskConf` on `HarnessConfig` | planned addition | do not exist yet |

## Reviewer Brief

- Intended scope: the `disk` section, free-space measurement, `auto disk status|check|trim`, the doctor row, and the preflight wiring, as the first slice of issue #198.
- Explicit non-goals: automatic reclamation of anything but go-build entries and abandoned go-build work dirs, registries, tokens, alerts, launchd, Windows, generated-root edits. Do not reopen the user's slice decision.
- Self-verified: Traceability Matrix over 26 requirements, twelve invariants each mapped to a Must oracle, three probes executed with evidence (one FAIL that changed a default), Reference Discipline with every existing reference read or grepped.
- Reviewer should focus on: whether the containment chain (name shape, `README` marker, `os.Root`, second `Lstat`) closes every route outside the roots; whether the mode table in S5 matches REQ-DISK-TRIM-02 for every input; whether exit 3 versus exit 1 is partitioned correctly in S3; and whether the 120-minute default should stand given probe A1 (an open question for the user, not a defect).

## Open Questions For The User

1. Default no-CI idle threshold: 120 minutes (this SPEC, backed by probe A1) or the script's 60 minutes? The user's own machine can set 60 either way.
2. Machine-neutral floor default of 10 GiB: acceptable for the public default? The dev Mac would configure `min_free_gb: 45` and `start_below_gb: 80`.
3. Doctor row below the floor: advisory WARN (this SPEC) or a doctor failure?
4. Scratchpad root is Claude Code's `/tmp/claude-<uid>` only; Codex and other session scratch roots are not known and are not reported.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 1 | files: spec.md, plan.md, research.md | reason: every existing path, symbol, line range, and Go toolchain fact was read or grepped on 2026-10-31
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md, research.md | reason: new package, files, command, constant, and config field carry `[NEW]`
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: EARS lines carry WHEN/IF/THEN or THE SYSTEM SHALL; scenarios use `### S<N>:` headings with bare Given/When/Then/And
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing, platform, user-file, and `[NEW]` references with the verification used
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: spec fixes the contract, plan the tasks and probes, acceptance the oracles, research the evidence and decisions
- Q-COMP-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: all 26 requirements appear in the matrix with a task, a scenario, and an invariant
- Q-COMP-03 | status: PASS | attempt: 1 | files: spec.md | reason: each requirement names its EARS type, trigger, result, and an observable (line, exit code, JSON field, file set)
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Outcome Lock covers config, measurement, three commands, doctor, and guidance; nothing mandatory is deferred
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, spec.md, plan.md, acceptance.md | reason: INV-D01 to INV-D12 each map to a requirement, a task, and a Must oracle with exact values or an explicit tolerance
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix complete; Reviewer Brief names four focus areas and the non-goals
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt is empty; Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 1 | files: plan.md, research.md | reason: three probe rows with honest FAIL, PASS, PASS verdicts, each with an evidence ref produced by a real execution; plan statements are classed
- Q-FEAS-01 | status: PASS | attempt: 1 | files: plan.md | reason: runtime code in `pkg/diskspace` and `internal/cli`, config in `pkg/config`, guidance in `content/` and `templates/shared/`
- Q-FEAS-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: sources of truth named; generated roots excluded; mirrors regenerated with `make generate-templates`
- Q-FEAS-03 | status: PASS | attempt: 1 | files: plan.md | reason: every verification command exists in the repository or the Go toolchain
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: no should, might, could, possibly, maybe, perhaps in requirement text
- Q-STYLE-02 | status: PASS | attempt: 1 | files: spec.md | reason: Priority and Type are separate lines; Must only
- Q-STYLE-03 | status: PASS | attempt: 1 | files: acceptance.md | reason: bare step keywords; tables sit between When and Then
- Q-SEC-01 | status: PASS | attempt: 1 | files: research.md, spec.md | reason: Trust Boundary names environment-derived roots, hostile symlinks, process names, and git output
- Q-SEC-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: containment requirements and the S5 symlink and refusal rows; scratchpad paths redacted here
- Q-SEC-03 | status: PASS | attempt: 1 | files: spec.md | reason: no new persistent artifact; output carries roots but no contents or credentials
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md | reason: one problem, one package, one namespace
- Q-COH-02 | status: PASS | attempt: 1 | files: research.md | reason: follow-on asks are non-goals of the user's slice, listed without identifiers
- Q-COH-03 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: no sibling SPEC
