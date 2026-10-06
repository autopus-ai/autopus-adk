# SPEC-PANERM-001: Retire the orchestra pane backend (subprocess and OMP only)

**Status**: draft
**Created**: 2026-10-06
**Domain**: PANERM
**Module**: autopus-adk
**PRD**: `prd.md` (same directory; FR-01–FR-16, FR-20–FR-22, FR-30). Where this SPEC and the PRD differ, `research.md` Design Decisions names the reason and this SPEC wins.
**Rollback anchor**: `7c781509` (last pane-capable commit)

## 목적

Since `7c781509` every orchestra entry point in `internal/cli` forces the subprocess backend (`orchestra.go:177-180`,
`orchestra_run.go`, `spec_review_loop.go`). The pane backend is unreachable, yet 60 pane-group files (8,732 physical
lines of the 17,945 non-test lines in `pkg/orchestra`), their CLI glue, five config keys, and per-platform completion
hooks are still compiled, defaulted, installed, and documented. The pane launch path also appends
`--dangerously-skip-permissions` and auto-answers permission prompts (`completion_poll.go`), so a one-line predicate
edit would bring a permission bypass back. This SPEC deletes that surface and keeps upgrades zero-touch: legacy
`autopus.yaml` files load with a quiet-aware notice, retired flags stay as hidden no-ops, retired subcommands print a
migration error, and `auto update` retracts only Autopus-managed completion hooks. `pkg/terminal`, `auto terminal`,
Agent Teams panes, and the OMP backend are untouched.

## Outcome Boundary

- Outcome Lock: autopus-adk ships no orchestra pane backend. Every provider of every `auto orchestra` command and of
  `auto spec review` runs as a headless subprocess, or through the OMP backend for `backend: omp`. A workspace generated
  by v0.50.123 (A34) keeps working: its `autopus.yaml` loads, `auto update` rewrites it without the removed keys and
  retracts managed completion hooks, installed skills that pass `--no-detach` behave identically, and `auto doctor`
  warns with a remedy before the update and passes after it.
- Mandatory requirements: REQ-01–REQ-18 (Priority Must). REQ-19 and REQ-20 are Should; REQ-21 is Nice.
- Explicit non-goals: `pkg/terminal` and its exported API, `auto terminal`, Agent Teams panes
  (`pkg/pipeline/{monitor,team_monitor,team_pane,team_layout}.go`); the OMP backend; subprocess engine semantics,
  strategies, judge logic, timeouts (SPEC-ORCH-019, SPEC-ORCH-024); the read-only provider policy (SPEC-REVIEWRO-001);
  a replacement live-progress UI; removing the hidden flags or the subcommand stubs; editing user-level settings files.
- Completion evidence: acceptance S1–S19 pass; RFP-1 has an operator receipt before any deletion task starts; RFP-2 and
  RFP-3 PASS; the clean-HEAD failure set and deadcode baseline in `research.md` are not exceeded; coverage of touched
  packages is at least 85%; every source file stays at or under 300 lines.

## Requirements

Priority uses Must, Should, and Nice. Type is the EARS type that `pkg/spec/parser.go` `detectEARSType` assigns
(WHEN…THEN event-driven, WHEN…IF…THEN optional, plain statements ubiquitous). Groups P, C, F, K, H, S, and I are
defined in the Retired Surface Inventory; quoted texts are defined in the Compatibility Contract.

| ID | Priority | Type | PRD | EARS requirement |
|----|----------|------|-----|------------------|
| REQ-01 | Must | EventDriven | FR-01 | WHEN any `auto orchestra` command (`brainstorm`, `plan`, `review`, `secure`, `run`, including the `recheck` strategy) or `auto spec review` executes providers, THEN THE SYSTEM SHALL dispatch each provider to the subprocess backend, or to the OMP backend for a provider configured with `backend: omp`, and SHALL NOT construct a pane backend, open a terminal surface, read a terminal screen, send terminal input, or create a detached job. |
| REQ-02 | Must | Ubiquitous | FR-02 | THE SYSTEM SHALL delete the group P files of `pkg/orchestra` and the group C callers in `internal/cli`, after moving every helper that retained code calls, at least `usesAntigravityPromptInteractive` and the `yield.go` output types `YieldOutput`, `WriteYieldOutput`, and `BuildYieldOutputFromResult`, into retained files. |
| REQ-03 | Must | Ubiquitous | FR-03 | THE SYSTEM SHALL remove the group F fields of `OrchestraConfig` and `ProviderConfig` together with the branches that read them, and SHALL keep the keys and values of `--format json` stdout, run receipts, and review receipts, including `executed_backend`, identical to `7c781509` for the same inputs. |
| REQ-04 | Must | EventDriven | FR-04 | WHEN `--no-detach` (brainstorm, plan, review, secure), `--subprocess` (brainstorm, plan, run, spec review), or `--plain` (spec review) is passed, THEN THE SYSTEM SHALL accept the flag, omit it from `--help`, and produce the same provider argv, backend, stdout, stderr, and exit code as the same invocation without the flag. |
| REQ-05 | Must | EventDriven | FR-05 | WHEN `--yield-rounds` is passed to `auto orchestra brainstorm`, THEN THE SYSTEM SHALL accept it, omit it from `--help`, run every round synchronously, write the yield notice to stderr exactly once, and keep stdout and the exit code identical to the same invocation without the flag. |
| REQ-06 | Must | EventDriven | FR-06 | WHEN `auto orchestra collect`, `inject`, `cleanup`, `status`, `wait`, or `result` is invoked with any arguments or flags, THEN THE SYSTEM SHALL fail with the retirement error and exit status 1, omit the command from `auto orchestra --help`, and touch no terminal session, workspace file, or child process. |
| REQ-07 | Must | Ubiquitous | FR-07 | THE SYSTEM SHALL remove the five group K schema fields and SHALL keep `prompt_via_args`, `features.cc21.monitor_enabled`, `orchestra.subprocess.max_concurrent`, `orchestra.subprocess.work_dir`, and `orchestra.subprocess.rounds` with unchanged YAML tags. |
| REQ-08 | Must | Optional | FR-08 | WHEN `autopus.yaml` contains a group K key at its declared path, with `*` matching exactly one mapping key, THEN THE SYSTEM SHALL load the file and ignore the value; IF the file contains any other key the schema does not declare, THEN THE SYSTEM SHALL keep failing with the existing strict-decode error that ends in `(unknown keys are rejected: fix the typo or delete the key)`. |
| REQ-09 | Must | EventDriven | FR-09 | WHEN a config load prunes at least one group K key, THEN THE SYSTEM SHALL write the config notice to stderr at most once per process, listing every pruned concrete path once in byte order, SHALL NOT write it to stdout or change the exit code, and SHALL suppress it for a command whose `--quiet` flag is set and for a process whose stderr is not a terminal. |
| REQ-10 | Must | EventDriven | FR-10 | WHEN `autopus.yaml` is written by `config.Save`, by `auto update`, by a raw-node save path in `internal/cli`, or by the loader's platform-name normalization rewrite, THEN THE SYSTEM SHALL write no group K key and SHALL preserve every retained key, value, and env placeholder, so that the v0.50.123 binary loads the written file. |
| REQ-11 | Must | Ubiquitous | FR-11 | THE SYSTEM SHALL emit no group K key from `DefaultFullConfig`, the codex and claude provider constructors, the migrations, `configs/autopus.yaml`, or `templates/shared/autopus.yaml.tmpl`, and the codex and claude default-entry detection SHALL return the same upgrade decision for every A34 default entry as at `7c781509`. |
| REQ-12 | Must | Ubiquitous | FR-12 | THE SYSTEM SHALL stop generating orchestra completion and ready hook entries and hook assets for claude-code, codex, antigravity-cli, gemini, and opencode, and SHALL delete the group H canonical assets. |
| REQ-13 | Must | EventDriven | FR-13 | WHEN `auto update` runs on a workspace that holds group S members, THEN THE SYSTEM SHALL remove exactly those settings entries and script files through the existing update transaction, leave every hook entry outside group S as the previous binary's update writes it, with user-authored entries byte-identical in command, matcher, timeout, and relative order, retract an `opencode.json` plugin entry only when its cleaned project-relative path equals a group S script path, report user-level settings files without editing them, and change no file on a second run. |
| REQ-14 | Must | EventDriven | FR-14 | WHEN `auto doctor` runs in text or `--json` mode, THEN THE SYSTEM SHALL drop the completion-hook and orchestra-monitor checks, stop expecting completion or ready hooks in `doctor.hooks.settings` and `doctor.hooks.configured`, report `doctor.legacy_orchestra_config` and `doctor.stale_completion_hooks` with status `warn` and the remedy `run "auto update"` for every present member, computed by the same functions that REQ-08 and REQ-13 use, and report both with status `pass` once no member remains. |
| REQ-15 | Must | Ubiquitous | FR-15 | THE SYSTEM SHALL remove pane execution, detach jobs, hook mode, yield rounds, pane-transport fallback steps, and group K keys from shipped instructions and docs by editing canonical sources and regenerating, SHALL keep `README.md` lines 497-509 (team panes) unchanged, and SHALL add a CHANGELOG entry that names every retired key, flag, subcommand, and hook with its migration path. |
| REQ-16 | Must | Ubiquitous | FR-16 | THE SYSTEM SHALL include a regression guard test that fails on any non-test Go file in `pkg/orchestra` or `internal/cli` that declares or references a group I identifier, on any `pkg/orchestra` file matching a group P pattern, and on any file under `content/`, `templates/`, or `configs/` that contains a retired flag, subcommand, or key token outside the guard allowlist. |
| REQ-17 | Must | Ubiquitous | security | THE SYSTEM SHALL contain no orchestra code path that launches a provider with `--dangerously-skip-permissions` or answers a provider permission prompt, and every remaining non-test `pkg/orchestra` mention of that flag SHALL be read-only bypass detection such as `provider_execution.go`. |
| REQ-18 | Must | Ubiquitous | FR-20 | THE SYSTEM SHALL remove the `--no-detach` and `--subprocess` tokens from shipped instruction sources and include both tokens in the REQ-16 instruction guard, while the binary keeps accepting both flags as REQ-04 requires. |
| REQ-19 | Should | Ubiquitous | FR-21 | THE SYSTEM SHALL delete or retarget `pkg/terminal/tmux_orchestra_regression_test.go` according to whether each assertion covers orchestra pane behavior or `pkg/terminal` behavior, and SHALL NOT change the exported API of `pkg/terminal`. |
| REQ-20 | Should | Ubiquitous | FR-22 | THE SYSTEM SHALL record `7c781509` as the last pane-capable commit and revert anchor in the CHANGELOG entry and in `research.md`. |
| REQ-21 | Nice | Ubiquitous | FR-30 | THE SYSTEM SHALL add a one-line `Superseded by SPEC-PANERM-001` or `Partially superseded by SPEC-PANERM-001` note to the status header of each pane-centric SPEC that PRD FR-30 lists, without editing the SPEC bodies. |

## Compatibility Contract

Exact texts. `<paths>` joins concrete dotted paths with `, ` in byte order; `<name>` is the invoked subcommand.

```text
config notice   : auto: warning: ignored removed autopus.yaml keys: <paths>; the orchestra pane backend was retired (SPEC-PANERM-001); run "auto update" or delete the keys
yield notice    : auto: warning: --yield-rounds was retired with the orchestra pane backend (SPEC-PANERM-001); all rounds run synchronously
retirement error: auto orchestra <name> was retired with the orchestra pane backend (SPEC-PANERM-001); orchestra commands now run synchronously and print their result directly
```

- The retirement error reaches stderr through the existing `Execute` path as `Error: <retirement error>`.
- Wildcard: `*` is legal only as a whole segment of a `removedConfigKeys` entry and matches every mapping key at that
  depth, one segment each; a scalar, sequence, or missing node at any segment is a miss, as `pruneNodeKey` does today.
- `workflow.team_default` keeps its silent prune; the config notice and REQ-14 cover group K paths only.
- A group S script that an `opencode.json` plugin entry references is deleted only in the transaction that retracts
  that entry; otherwise it is kept and reported by REQ-14, so no plugin entry is left dangling.
- A load that fails prints only its error, never the config notice.
- Guard allowlist: empty at merge; every later entry names a path, a token, and a reason inside the guard test.
- Doctor messages: `legacy orchestra keys: <paths>` and `stale completion hooks: <file> <event> <script>, ...`.

## Retired Surface Inventory

| Group | Members |
|-------|---------|
| P (pkg/orchestra files) | `pane_*.go`, `interactive*.go`, `hook_*.go`, `completion_{poll,file_ipc,signal,detector}.go`, `cc21_monitor.go`, `signal_emitter.go`, `round_signal.go`, `surface_manager.go`, `surface_tracker*.go`, `warm_pool.go`, `read_screen.go`, `screen_sanitizer.go`, `relay_pane.go`, `recovery_hook_launch.go`, `session*.go`, `yield_session.go`, `reviewer_response_file.go`, `detach.go`, `job.go` (60 files at `7c781509`; `relay.go` and `yield.go` stay) |
| C (internal/cli) | pane parts of `orchestra_terminal.go`; `orchestra_hookmode.go`, `orchestra_hook_discovery*.go`, `orchestra_cc21.go`, `cc21_runtime.go`; `ownStructuredReviewHookSession` in `spec_review_structured.go`; test-only `resolveSubprocessMode` in `orchestra_config.go`; job, collect, inject, and cleanup bodies replaced by REQ-06 stubs |
| F (fields) | `ProviderConfig.PaneArgs`, `.InteractiveInput`, `.WorkingPatterns`; `OrchestraConfig.Terminal`, `.NoDetach`, `.Interactive`, `.HookMode`, `.SessionID`, `.CompletionDetector`, `.YieldRounds`, `.MonitorEnabled`, `.MonitorTimeout`, `.SubprocessMode`; pane branches in `judge_session_evidence.go`, `provider_validation.go`, `reliability_preflight.go` |
| K (config keys) | `orchestra.providers.*.pane_args`, `orchestra.providers.*.interactive_input`, `orchestra.providers.*.working_patterns`, `orchestra.subprocess.enabled`, `features.cc21.monitor_pattern_timeout_ms` |
| H (assets) | `content/hooks/hook-{claude,codex,gemini}-{sessionstart,stop}.sh`, `content/hooks/hook-gemini-afteragent.sh`, `content/hooks/hook-opencode-complete.ts`, `templates/hooks/completion-hook.sh.tmpl` |
| S (stale hook set) | see the table below |
| I (identifiers) | `RunPaneOrchestra`, `RunPaneOrchestraDetached`, `RunInteractivePaneOrchestra`, `runInteractiveDebate`, `NewInteractivePaneBackend`, `InteractivePaneBackend`, `paneCapable`, `paneLaunchFor`, `buildPaneLaunchCommand`, `HookSession`, `ShouldDetach`, `ScreenPollDetector`, `FileIPCDetector`, `AUTOPUS_SESSION_ID`, `PaneArgs`, `InteractiveInput`, `WorkingPatterns`; T6 appends the remaining exported declarations of the deleted files |

| Platform | Settings file and event | Managed command ends in | Script files |
|----------|-------------------------|-------------------------|--------------|
| claude-code | `.claude/settings.json` Stop, SessionStart | `.claude/hooks/autopus/hook-claude-stop.sh`, `hook-claude-sessionstart.sh` | `.claude/hooks/autopus/` copies of all eight group H `content/hooks` files |
| codex | `.codex/hooks.json` Stop, SessionStart | `.codex/hooks/autopus/hook-codex-stop.sh"`, `hook-codex-sessionstart.sh"` | `.codex/hooks/autopus/hook-codex-{stop,sessionstart}.sh` |
| antigravity-cli | `.agents/hooks.json` Stop; `.gemini/settings.json` AfterAgent | `.gemini/hooks/autopus/hook-gemini-stop.sh"`, `hook-gemini-afteragent.sh` | `.gemini/hooks/autopus/hook-gemini-{stop,afteragent}.sh` |
| opencode | `opencode.json` `plugin` entries | equal to a script path of this table | none |

## 생성 파일 상세

All source files stay at or under 300 lines; tests sit next to each file.

| Path | Role |
|------|------|
| `pkg/orchestra/runner.go`, `backend.go`, `recheck.go`, `backend_routed.go` | subprocess-only dispatch; `selectRoutedBackend` keeps the OMP route |
| `pkg/orchestra/types.go`, `yield.go`, `provider_patterns.go`, `judge_session_evidence.go`, `provider_validation.go`, `reliability_preflight.go` | group F removal, relocated helpers, pane branches dropped |
| `internal/cli/orchestra*.go`, `spec_review*.go` | group C removal, hidden flags, glue trimming |
| [NEW] `internal/cli/orchestra_retired.go` | hidden no-op flag registration, yield notice, REQ-06 stubs |
| `pkg/config/loader_strict.go`, `loader.go`, `schema_orchestra.go`, `schema.go`, `defaults.go`, `migrate*.go`, `codex_provider.go`, `claude_provider.go` | group K removal, wildcard prune returning pruned paths, defaults |
| [NEW] `internal/cli/config_notice.go` | once-per-process notice with `--quiet` and stderr-TTY suppression |
| `pkg/content/hooks.go`, `hooks_completion.go`; `pkg/adapter/{claude,codex,antigravity,opencode}` | generation stop and group S retraction |
| [NEW] `pkg/adapter/stale_completion_hooks.go` | closed group S declaration shared by update and doctor |
| `internal/cli/doctor_json_checks.go`, `doctor_remediation.go`, `check_cc21.go`, [NEW] `doctor_legacy_orchestra.go` | REQ-14 checks |
| [NEW] `internal/cli/pane_retirement_guard_test.go` | REQ-16 guard |
| `content/`, `templates/`, `configs/`, `README.md`, `ARCHITECTURE.md`, `docs/README.ko.md`, `CHANGELOG.md` | REQ-15, REQ-18, REQ-20 |

## Related SPECs

None as sibling (see `research.md` Sibling SPEC Decision). SPEC-REVIEWRO-001 (approved) lands its T1 first; PANERM makes
its pane items moot (see `plan.md` Cross-SPEC Ordering). SPEC-SIGMABAND-001 (approved) consumes the shared read-only
policy and must not reintroduce group I identifiers. SPEC-ORCH-019 and SPEC-ORCH-021 oracles stay green.
SPEC-TEAMPANE-001 and SPEC-ORCH-002 (team panes) are untouched.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-01 | T1, T5, T6 | S1, S2, S3, S19 | INV-01, INV-13 |
| REQ-02 | T4, T6, T8 | S15 | INV-10 |
| REQ-03 | T7 | S16 | INV-11 |
| REQ-04 | T9 | S8 | INV-05 |
| REQ-05 | T9 | S9 | INV-05 |
| REQ-06 | T10 | S10 | INV-06 |
| REQ-07 | T11 | S4 | INV-02 |
| REQ-08 | T3, T12 | S4, S5 | INV-02 |
| REQ-09 | T13 | S6 | INV-03 |
| REQ-10 | T3, T14 | S7 | INV-04 |
| REQ-11 | T11 | S7, S14 | INV-04, INV-09 |
| REQ-12 | T15 | S11 | INV-07 |
| REQ-13 | T3, T16 | S11, S12 | INV-07 |
| REQ-14 | T17 | S13 | INV-08 |
| REQ-15 | T18 | S18 | INV-12 |
| REQ-16 | T19 | S15, S18 | INV-10, INV-12 |
| REQ-17 | T6, T19 | S15 | INV-10 |
| REQ-18 | T18, T19 | S18 | INV-12 |
| REQ-19 | T20 | S17 | INV-11 |
| REQ-20 | T18 | S18 | INV-12 |
| REQ-21 | T21 | S18 | INV-12 |
