# SPEC-PANERM-001: Retire the orchestra pane backend (subprocess and OMP only)

**Status**: approved
**Created**: 2026-10-06
**Revised**: 2026-10-06 (rev 3: review rounds 1 and 2; see Review Resolution)
**Domain**: PANERM
**Module**: autopus-adk
**PRD**: `prd.md` (same directory). Where this SPEC and the PRD differ, `research.md` Design Decisions names the reason and this SPEC wins.

## 목적

Pane execution is unreachable from the CLI: `7c781509` forces the subprocess backend for every orchestra command and
`16b50216` removed the last detach branch (guarded by `TestCLIProductionCodeNeverCallsPaneEntryPoints`). The pane
code is still compiled, defaulted, installed, and documented: 60 group P files (8,732 of 17,945 non-test lines in
`pkg/orchestra`), CLI glue, five config keys, and per-platform completion hooks, plus a launch path that adds
`--dangerously-skip-permissions` and answers permission prompts. This SPEC deletes that surface with zero-touch upgrades.

## Outcome Boundary

- Outcome Lock, non-goals, and completion evidence: `research.md` Outcome Lock (canonical).
- Mandatory requirements: REQ-01–REQ-18 (Must). REQ-19, REQ-20 are Should; REQ-21 is Nice and does not gate sync.
- B is the main commit the first PANERM merge is based on (`16b50216` at authoring); O is `auto` built from `c447badc`
  (v0.50.123, A34). Parity oracles compare with B; upgrade oracles use O.

## Requirements

Type is the EARS type that `pkg/spec/parser.go` `detectEARSType` assigns (WHEN…THEN event-driven, WHEN…IF…THEN
optional, plain statements ubiquitous). Groups and quoted texts are defined in the sections below.

| ID | Priority | Type | PRD | EARS requirement |
|----|----------|------|-----|------------------|
| REQ-01 | Must | EventDriven | FR-01 | WHEN any `auto orchestra` command (`brainstorm`, `plan`, `review`, `secure`, `run`, including the `recheck` strategy) or `auto spec review` executes providers, THEN THE SYSTEM SHALL dispatch each provider to the subprocess backend, or to the OMP backend for a provider configured with `backend: omp`, and SHALL NOT construct a pane backend, open a terminal surface, read a terminal screen, send terminal input, or create a detached job. |
| REQ-02 | Must | Ubiquitous | FR-02 | THE SYSTEM SHALL delete the group P files of `pkg/orchestra` and the group C code of `internal/cli` only after every retained consumer has stopped referencing them, and SHALL first move each helper that retained code calls, at least `usesAntigravityPromptInteractive`, into a retained file. |
| REQ-03 | Must | Ubiquitous | FR-03 | THE SYSTEM SHALL remove the group F fields together with the branches that read them, and SHALL keep the keys and values of `--format json` stdout (`schema`, `merged`, `receipt.provider_receipts[]`) and of `.autopus/specs/<SPEC-ID>/review-receipt.json` (`providers[].executed_backend`) identical to B for the same inputs. |
| REQ-04 | Must | EventDriven | FR-04 | WHEN `--no-detach` (brainstorm, plan, review, secure), `--subprocess` (brainstorm, plan, run, spec review), or `--plain` (spec review) is passed, THEN THE SYSTEM SHALL accept the flag, omit it from `--help`, and produce the same provider argv, backend, stdout, stderr, and exit code as the same invocation without the flag. |
| REQ-05 | Must | EventDriven | FR-05 | WHEN `--yield-rounds` is passed to `auto orchestra brainstorm`, THEN THE SYSTEM SHALL accept it, omit it from `--help`, run every round synchronously, write the yield notice to stderr exactly once, and keep stdout and the exit code identical to the same invocation without the flag. |
| REQ-06 | Must | EventDriven | FR-06 | WHEN `auto orchestra collect`, `inject`, `cleanup`, `status`, `wait`, or `result` is invoked with any arguments or flags, including root flags placed before the subcommand, THEN THE SYSTEM SHALL fail with the retirement error and exit status 1 without running the root pre-run, omit the command from `auto orchestra --help`, and touch no terminal session, workspace file, or child process. |
| REQ-07 | Must | Ubiquitous | FR-07 | THE SYSTEM SHALL remove the five group K schema fields and SHALL keep `prompt_via_args`, `features.cc21.monitor_enabled`, `orchestra.subprocess.max_concurrent`, `orchestra.subprocess.work_dir`, and `orchestra.subprocess.rounds` with unchanged YAML tags. |
| REQ-08 | Must | Optional | FR-08 | WHEN `autopus.yaml` contains a group K key at its declared path, with `*` matching exactly one mapping key, THEN THE SYSTEM SHALL load the file and ignore the value; IF the file contains a key that the schema does not declare and that neither `removedConfigKeys` (group K plus `workflow.team_default`) nor `reservedConfigKeys` tolerates, THEN THE SYSTEM SHALL keep failing with the existing strict-decode error that ends in `(unknown keys are rejected: fix the typo or delete the key)`. |
| REQ-09 | Must | EventDriven | FR-09 | WHEN a config load prunes at least one group K key, THEN THE SYSTEM SHALL write the config notice to stderr at most once per process, listing every pruned concrete path once in byte order, SHALL NOT write it to stdout or change the exit code, and SHALL suppress it for a command whose `--quiet` flag is set and for a process whose stderr is not a terminal. |
| REQ-10 | Must | EventDriven | FR-10 | WHEN `autopus.yaml` is written by `config.Save`, by `auto update`, by a raw-node writer (`saveQualityScalar` through `auto quality supervisor`, `persistQualityProvider` through `auto quality provider`, `replaceAutopusConfigSection` through `applyOMPProfile`), or by the loader's platform-name normalization rewrite, THEN THE SYSTEM SHALL write no group K key, SHALL preserve everything else that the writer preserves at B, and SHALL make `auto update` remove pruned group K keys from a file with no other migration through a raw-node rewrite that keeps comments, env placeholders, and `reservedConfigKeys` blocks, so that binary O loads every written file. |
| REQ-11 | Must | Ubiquitous | FR-11 | THE SYSTEM SHALL emit no group K key from `DefaultFullConfig`, the codex and claude provider constructors, the migrations, `configs/autopus.yaml`, or `templates/shared/autopus.yaml.tmpl`, and the codex and claude default-entry detection SHALL return the same upgrade decision as B for every A34 default entry. |
| REQ-12 | Must | Ubiquitous | FR-12 | THE SYSTEM SHALL stop generating orchestra completion and ready hook entries and hook assets for claude-code, codex, antigravity-cli, gemini, and opencode, and SHALL delete the group H canonical assets. |
| REQ-13 | Must | EventDriven | FR-13 | WHEN `auto update` runs on a workspace that holds group S members, THEN THE SYSTEM SHALL remove exactly those handlers, plugin entries, and script files inside each platform's update transaction, drop a settings entry only when no handler remains, keep user-authored handlers and entries byte-identical in command, matcher, timeout, and relative order, match opencode plugin entries by the identity rule of the Compatibility Contract, report user-level settings files without editing them, and change no file outside the transaction records on a second run. |
| REQ-14 | Must | EventDriven | FR-14 | WHEN `auto doctor` runs in text or `--json` mode, THEN THE SYSTEM SHALL expect no completion or ready hook in any check, including `doctor.hooks.settings` and `doctor.hooks.configured`, report `doctor.legacy_orchestra_config` and `doctor.stale_completion_hooks` with status `warn` and the remedy `run "auto update"` for every present member, computed by the same functions that REQ-08 and REQ-13 use, and report both with status `pass` once no member remains. |
| REQ-15 | Must | Ubiquitous | FR-15 | THE SYSTEM SHALL remove pane execution, detach jobs, hook mode, yield rounds, pane-transport fallback steps, and group K keys from shipped instructions and docs by editing canonical sources and regenerating, SHALL keep `README.md` lines 497-509 (team panes) unchanged, and SHALL add a CHANGELOG entry that names every retired key, flag, subcommand, and hook with its migration path. |
| REQ-16 | Must | Ubiquitous | FR-16 | THE SYSTEM SHALL extend `TestCLIProductionCodeNeverCallsPaneEntryPoints` (`internal/cli/orchestra_pane_unreachable_test.go`) so that it fails on any non-test Go file in `pkg/orchestra` or `internal/cli` that declares or references a group I identifier, on any `pkg/orchestra` file matching a group P pattern, and on any file under `content/`, `templates/`, or `configs/` that contains a retired flag, subcommand, or key token outside the guard allowlist. |
| REQ-17 | Must | Ubiquitous | security | THE SYSTEM SHALL contain no orchestra code that adds `--dangerously-skip-permissions`, `--dangerously-bypass-approvals-and-sandbox`, or `--yolo` to a provider launch or that answers a provider permission prompt, and SHALL pass provider-configured args through unchanged under the existing bypass detection in `provider_execution.go`, while the read-only projection of plan and brainstorm keeps rejecting them (`validateReadOnlyProviderArgv`). |
| REQ-18 | Must | Ubiquitous | FR-20 | THE SYSTEM SHALL remove the `--no-detach` and `--subprocess` tokens from shipped instruction sources and include both tokens in the REQ-16 instruction guard, while the binary keeps accepting both flags as REQ-04 requires. |
| REQ-19 | Should | Ubiquitous | FR-21 | THE SYSTEM SHALL delete or retarget `pkg/terminal/tmux_orchestra_regression_test.go` according to whether each assertion covers orchestra pane behavior or `pkg/terminal` behavior, and SHALL NOT change the exported API of `pkg/terminal`. |
| REQ-20 | Should | Ubiquitous | FR-22 | THE SYSTEM SHALL record B as the revert anchor in the CHANGELOG entry and in `research.md`, together with `7c781509` and `16b50216` as the commits that made pane execution unreachable. |
| REQ-21 | Nice | Ubiquitous | FR-30 | THE SYSTEM SHALL add a one-line `Superseded by SPEC-PANERM-001` or `Partially superseded by SPEC-PANERM-001` note to the status header of each pane-centric SPEC that PRD FR-30 lists, without editing the SPEC bodies. |

## Compatibility Contract

`<paths>` joins concrete dotted paths with `, ` in byte order; `<name>` is the invoked subcommand.

```text
config notice   : auto: warning: ignored removed autopus.yaml keys: <paths>; the orchestra pane backend was retired (SPEC-PANERM-001); run "auto update" or delete the keys
yield notice    : auto: warning: --yield-rounds was retired with the orchestra pane backend (SPEC-PANERM-001); all rounds run synchronously
retirement error: auto orchestra <name> was retired with the orchestra pane backend (SPEC-PANERM-001); orchestra commands now run synchronously and print their result directly
```

- Stubs: `Hidden`, `DisableFlagParsing`, `cobra.ArbitraryArgs`, and their own no-op `PersistentPreRunE`, so the root
  pre-run (`collectGlobalFlags`, `validateQualityPreset`, config load) never runs; cobra runs only the nearest pre-run
  because this repo does not set `EnableTraverseRunHooks`. The error prints as `Error: <retirement error>`, exit 1.
- Wildcard: `*` is legal only as a whole segment of a `removedConfigKeys` entry and matches every mapping key at that
  depth; a scalar, sequence, or missing node at any segment is a miss, as `pruneNodeKey` does today.
- Tolerated keys: group K, the existing `workflow.team_default` (silent), and `reservedConfigKeys` (`future_extension`,
  `operator_extension`, unchanged). The notice and REQ-14 list group K only; a failing load prints only its error.
- Retraction unit: a handler. Atomic unit: one platform's `adapter.ApplyTransaction` (removes, then writes, then the
  manifest; `transaction.go:129-145`), which owns that platform's settings, scripts, and manifest. `config.Save` runs
  before the platform loop (`update.go:180-183`); the loop prints `  ✗ <platform>: <error>` and the command fails with
  `플랫폼 업데이트 실패: <platform>: <error>` (`update.go:232-276`). A failed platform keeps its files byte-identical.
  Tests inject faults through `[NEW] adapter.transactionStepHook` (nil in production), called before each step.
- Transaction records change on every run by design and are excluded from no-change oracles: `.autopus/txns/`
  (journals, `transaction.go:104`), `.autopus/backup/` (snapshots, `:105`), and the `generated_at` field of
  `.autopus/<platform>-manifest.json` (`writeManifest`, `:216`; `manifest.go:75`).
- OpenCode: retraction reads the project `opencode.json` through `effectivePluginConfig` (`opencode_config_v2.go:11`),
  so it accepts exactly what the adapter accepts: on V2, `plugins` strings and `{"package": ...}` objects; otherwise
  `plugin` strings and `[path, {options}]` tuples (`validatePluginEntries`, `:54`). An entry, and on V2 a coexisting
  legacy `plugin` entry as `mergePluginConfig` treats it, matches when `managedEntry(pluginEntryPath(entry), t, root)`
  holds for t = a group S script's root-relative slash path or `./` plus it (bare, `./`, absolute, and `file:` forms).
  A config that `effectivePluginConfig` rejects is never rewritten; its scripts stay and REQ-14 reports them. A script
  that an entry references is deleted only in the transaction that retracts the entry.
- Guard allowlist: empty at merge; a later entry names a path, a token, and a reason inside the guard test.
- Doctor messages: `legacy orchestra keys: <paths>` and `stale completion hooks: <file> <event> <script>, ...`.

## Retired Surface Inventory

| Group | Members |
|-------|---------|
| P (pkg/orchestra files) | `pane_*.go`, `interactive*.go`, `hook_*.go`, `completion_{poll,file_ipc,signal,detector}.go`, `cc21_monitor.go`, `signal_emitter.go`, `round_signal.go`, `surface_manager.go`, `surface_tracker*.go`, `warm_pool.go`, `read_screen.go`, `screen_sanitizer.go`, `relay_pane.go`, `recovery_hook_launch.go`, `session*.go`, `yield_session.go`, `reviewer_response_file.go`, `detach.go`, `job.go` (60 files at B; `relay.go` and `yield.go` stay, `BuildYieldOutput` goes; `usesAntigravityPromptInteractive`, `noneBackendMarker`, and every other group P declaration that retained code uses move out first) |
| C (internal/cli) | pane parts of `orchestra_terminal.go`; `orchestra_hookmode.go`, `orchestra_hook_discovery*.go`, `orchestra_cc21.go` (`resolveCC21MonitorRuntime`; `cc21_runtime.go` is TaskCreated runtime and stays); `ownStructuredReviewHookSession` (`spec_review_structured.go:92`); `resolveSubprocessMode` (`orchestra_config.go:245`); job, collect, inject, cleanup bodies (replaced by stubs); retired `OrchestraFlags` fields |
| F (fields) | `ProviderConfig.PaneArgs`, `.InteractiveInput`, `.WorkingPatterns`; `OrchestraConfig.Terminal`, `.NoDetach`, `.Interactive`, `.HookMode`, `.SessionID`, `.CompletionDetector`, `.YieldRounds`, `.MonitorEnabled`, `.MonitorTimeout`, `.SubprocessMode` |
| K (config keys) | `orchestra.providers.*.pane_args`, `orchestra.providers.*.interactive_input`, `orchestra.providers.*.working_patterns`, `orchestra.subprocess.enabled`, `features.cc21.monitor_pattern_timeout_ms` |
| H (assets) | `content/hooks/hook-{claude,codex,gemini}-{sessionstart,stop}.sh`, `hook-gemini-afteragent.sh`, `hook-opencode-complete.ts`, `templates/hooks/completion-hook.sh.tmpl` |
| I (identifiers) | `RunPaneOrchestra`, `RunPaneOrchestraDetached`, `RunInteractivePaneOrchestra`, `runInteractiveDebate`, `NewInteractivePaneBackend`, `InteractivePaneBackend`, `paneCapable`, `paneLaunchFor`, `buildPaneLaunchCommand`, `HookSession`, `ShouldDetach`, `ScreenPollDetector`, `FileIPCDetector`, `AUTOPUS_SESSION_ID`, `PaneArgs`, `InteractiveInput`, `WorkingPatterns`, plus the exported declarations of deleted files (T10) |

| Group S platform | Settings handlers | Script files |
|------------------|-------------------|--------------|
| claude-code | `.claude/settings.json` Stop and SessionStart handlers ending in `.claude/hooks/autopus/hook-claude-stop.sh` or `hook-claude-sessionstart.sh` | `.claude/hooks/autopus/` copies of the 7 group H `.sh` files (`content/embed.go` embeds `hooks/*.sh`), plus a legacy `hook-opencode-complete.ts` |
| codex | `.codex/hooks.json` Stop and SessionStart handlers for `.codex/hooks/autopus/hook-codex-{stop,sessionstart}.sh` | `.codex/hooks/autopus/hook-codex-{stop,sessionstart}.sh` |
| antigravity-cli | `.agents/hooks.json` Stop (`hook-gemini-stop.sh`); `.gemini/settings.json` AfterAgent (`hook-gemini-afteragent.sh`) | `.gemini/hooks/autopus/hook-gemini-{stop,afteragent}.sh` |
| opencode | `opencode.json` plugin entries matching a script of this table | none |

New (all `[NEW]`): `internal/cli/orchestra_retired.go`, `config_notice.go`, `doctor_legacy_orchestra.go`;
`pkg/adapter/stale_completion_hooks.go`; `adapter.transactionStepHook`; fixtures `pkg/config/testdata/legacy_pane/`,
`internal/cli/testdata/stale_hooks/`.
Every source file stays at or under 300 lines.

## Related SPECs

No sibling. Ordering with SPEC-REVIEWRO-001 and SPEC-SIGMABAND-001 (both approved): `plan.md` Cross-SPEC Ordering.
SPEC-ORCH-021 keeps S15, S16, S18, S20; its pane argv oracles S17 and S19 retire. SPEC-TEAMPANE-001 and SPEC-ORCH-002
(team panes) are untouched.

## Review Resolution

| Finding | Resolution |
|---------|------------|
| F-01 | RFP-1 and S19 read `receipt.provider_receipts[]` (`backend`, `exit_code`, `timed_out`, `usable`, `failure_class`) |
| F-02 | `16b50216` removed the CLI detach branch; the verified fact cites it with `7c781509`; REQ-16 extends its AST test |
| F-03 | consumers-first waves: W1 edits consumers only, W2 deletes declarations in one atomic merge (`plan.md`) |
| F-04 | S17 names the two pane-argv tests that retire with ORCH-021 S17/S19 and the allowed table-field removals |
| F-05 | REQ-08 and the invariants keep `reservedConfigKeys` |
| F-06 | OpenCode matching reuses `effectivePluginConfig`, `pluginEntryPath`, `managedEntry`; entry forms corrected in rev 2 |
| F-07 | handler-level retraction (REQ-13); S11 adds mixed claude and codex entries |
| F-08 | atomic unit defined; S12 injects the failure inside the first update and adds the O revert path |
| F-09 | stubs own a no-op pre-run; S10 adds root-flag inputs |
| F-10 | REQ-17 narrowed to code-added flags and prompt answering; S1 checks argv pass-through |
| F-11 | S7 covers the four writers and their storage paths; `auto update` saves when keys were pruned |
| F-12 | S16 adds `secure`, `auto spec review`, and `review-receipt.json` |
| F-13 | S3 uses `selectRoutedBackend` (`internal/cli/omp_review_backend.go:180`) and the CLI native-binary boundary |
| F-15 | REQ-21 moves to Nice scenario S20 and does not gate sync |
| F-03 (rev 2) | W1 is consumer-only with every test compiling; all declaration, asset, and test deletions land in the W2 atomic merge; `noneBackendMarker` moves first; the census is a trial deletion |
| F-016 | `cc21_runtime.go` (TaskCreated runtime) leaves group C and stays |
| F-06 (rev 2) | OpenCode forms follow `validatePluginEntries`: V2 objects, legacy tuples; rejected configs are never rewritten |
| F-08 (rev 2) | `[NEW] adapter.transactionStepHook` seam, the real `✗ claude-code:` output, and a hash snapshot replace `git status` |
| F-017 | `auto update` prunes through a raw-node rewrite, not `config.Save`, so comments and reserved blocks stay |
| F-11 (rev 2) | S7 runs `persistQualityProvider` and `applyOMPProfile` with `operator_extension` |
| F-05 (rev 2) | REQ-08 names `removedConfigKeys`, including `workflow.team_default` |
| F-018 | REQ-17 keeps the read-only projection's rejection for plan and brainstorm; S1 checks it with a supported provider name, since `validateReadOnlyProviderArgs` checks the name first (`orchestra_readonly_policy.go:76-91`) |
| F-08 (rev 3) | no-change oracles exclude the transaction records listed in the Compatibility Contract |
| F-019 | fixtures pin the OpenCode major through a fake `opencode --version` per workspace |

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-01 | T1, T5, T7 | S1, S2, S3, S19 | INV-01, INV-13 |
| REQ-02 | T4, T5, T7, T9 | S15, S17 | INV-10, INV-11 |
| REQ-03 | T2, T7 | S16 | INV-11 |
| REQ-04 | T6 | S8 | INV-05 |
| REQ-05 | T6 | S9 | INV-05 |
| REQ-06 | T6 | S10 | INV-06 |
| REQ-07 | T8 | S4 | INV-02 |
| REQ-08 | T3, T8 | S4, S5 | INV-02 |
| REQ-09 | T12 | S6 | INV-03 |
| REQ-10 | T3, T13 | S7 | INV-04 |
| REQ-11 | T8 | S7, S14 | INV-04, INV-09 |
| REQ-12 | T10 | S11 | INV-07 |
| REQ-13 | T3, T11 | S11, S12 | INV-07 |
| REQ-14 | T14 | S13 | INV-08 |
| REQ-15 | T15 | S18 | INV-12 |
| REQ-16 | T16 | S15, S18 | INV-10, INV-12 |
| REQ-17 | T5, T7, T16 | S1, S15 | INV-01, INV-10 |
| REQ-18 | T15, T16 | S18 | INV-12 |
| REQ-19 | T17 | S17 | INV-11 |
| REQ-20 | T15 | S18 | INV-12 |
| REQ-21 | T18 | S20 | INV-12 |
