# SPEC-PANERM-001 수락 기준

## Test Scenarios

Fixtures (T3). O, B, and goldens follow `spec.md` Outcome Boundary. C1: the A34 default config written by O; its group
K paths are P1 = `features.cc21.monitor_pattern_timeout_ms` (30000), `orchestra.providers.claude.pane_args`
(`[--print, --model, claude-fable-5-1, --effort, max]`), `orchestra.providers.codex.pane_args` (`[-m, gpt-6-astra, -c,
model_reasoning_effort="max"]`), `orchestra.providers.gemini.interactive_input` (`stdin`). C2: all five group K keys
under providers `claude`, `codex`, `my-local`, plus `my-local.prompt_via_args: true`, `orchestra.subprocess.max_concurrent:
2`, `orchestra.subprocess.rounds: 3`, `orchestra.subprocess.work_dir: "${AUTOPUS_WORKDIR}"`, `features.cc21.monitor_enabled:
true`, `future_extension: {note: keep}`, and the comment `# keep-me`; its pruned paths are P2 =
`features.cc21.monitor_pattern_timeout_ms, orchestra.providers.claude.pane_args, orchestra.providers.claude.working_patterns,
orchestra.providers.codex.interactive_input, orchestra.providers.my-local.interactive_input,
orchestra.providers.my-local.pane_args, orchestra.providers.my-local.working_patterns, orchestra.subprocess.enabled`.
C3: `orchestra.providers.codex.pane_argz: [x]`. C4: `orchestra.providers.codex: "x"`; separately `orchestra.providers:
[a, b]`. C5: `orchestra.pane_args: [x]`; separately `orchestra.providers.codex.subprocess.pane_args: [x]`. C6: C2 plus
C3's line. C7: C2 with `platforms: [claude]`, which `MigratePlatformNames` rewrites to `claude-code`.
Workspaces from O `auto init`: W-claude (plus an unreferenced legacy `.claude/hooks/autopus/hook-opencode-complete.ts`),
W-codex, W-agy (antigravity-cli), and three OpenCode ones with that `.ts` present: W-oc2 (V2; `plugins` holds
`{"package": "file://<root>/.claude/hooks/autopus/hook-opencode-complete.ts", "options": {}}`, `{"package": "my-plugin"}`,
`./plugins/mine.ts`), W-oc1 (legacy `plugin` holds `[".claude/hooks/autopus/hook-opencode-complete.ts", {}]`, the
`.ts` absolute path, `./plugins/mine.ts`), and W-oc-bad (V2 `plugins` holding a tuple, which `validatePluginEntries`
rejects); W-mix is claude-code with an `opencode.json` that references the `.ts` while opencode is not installed. Every
settings file gets a separate user entry before the managed ones; W-claude and W-codex Stop also get a mixed entry (a
group S handler, then `./scripts/notify.sh`). OpenCode runtime: a fake `opencode` on PATH prints `opencode 2.0.0` for
W-oc2 and W-oc-bad and `opencode 1.0.0` for W-oc1, for O's `auto init` and the new `auto update` alike
(`opencode_version.go:26` reads the major); W-mix has no `opencode` on PATH. H(w) is the sorted (path, mode, sha256)
list of every file under w, ignored files included, except `.git/`, `.autopus/txns/`, and `.autopus/backup/`, with
`generated_at` removed from each `.autopus/<platform>-manifest.json` before hashing. Fake providers record argv; clock,
terminal, and models are fakes; text compares exactly.

### S1: Every command dispatches to subprocess or OMP
Priority: Must
Given `TMUX` set, a recording fake terminal, CLI providers claude, codex, gemini, and one `backend: omp` provider, all with args free of bypass flags; and a second config C-byp whose `orchestra.providers.claude` has binary `claude` and args `[--print, --dangerously-skip-permissions]`
When `auto orchestra brainstorm`, `plan`, `review`, `secure`, `run`, the `recheck` strategy, and `auto spec review` execute
Then every CLI provider reports backend `subprocess` (`receipt.provider_receipts[].backend`; `review-receipt.json` `providers[].executed_backend`) and the OMP provider reports `omp`
And the fake terminal records 0 calls, and snapshots of `$TMPDIR` and `.autopus/` show no new job or session file
And no recorded argv holds `--dangerously-skip-permissions`, `--dangerously-bypass-approvals-and-sandbox`, or `--yolo`
And with C-byp, `auto orchestra review a.go --providers claude` executes claude with `--dangerously-skip-permissions` exactly once, while `auto orchestra plan "x" --providers claude` fails with `read-only provider policy: provider "claude" contains unsafe argv "--dangerously-skip-permissions"` and 0 executions, as `validateReadOnlyProviderArgv` does at B

### S2: Backend selection has no pane branch
Priority: Must
Given orchestra configs for strategies consensus, debate, pipeline, relay, fastest, and recheck
When `SelectBackend`, `recheckTransport`, and `RunOrchestra` run with the `newCommand` seam recorded
Then `SelectBackend` and `recheckTransport` return backend name `subprocess` for every config
And a one-round consensus run with 3 providers and no judge records exactly 3 provider processes, one per provider

### S3: OMP routing and the native-binary boundary stay as at B
Priority: Must
Given an orchestra config with one `backend: omp` provider, and a `plan` call whose codex binary is a non-native wrapper
When `selectRoutedBackend` (`internal/cli/omp_review_backend.go:180`) builds the backend and `runOrchestraCommand` runs the plan
Then the OMP provider routes to the OMP review backend and the others to `subprocess`
And the plan fails with an error containing `requires native binary "codex"`, with 0 catalog probes and 0 provider executions, as `orchestra_codex_quality_test.go` asserts at B

### S4: Legacy configs load and removed values are ignored
Priority: Must
Given workspaces holding C1 and C2
When `config.Load` runs
Then both load with a nil error and the pruned path lists equal P1 and P2 in that order
And for C2 the loaded values are `prompt_via_args` true, `max_concurrent` 2, `rounds` 3, `monitor_enabled` true, and `future_extension` is tolerated as at B

### S5: Typos and misplaced keys still fail
Priority: Must
Given C3, C4, C5, and C6
When `config.Load` runs
Then C3 fails with `yaml: unmarshal errors:` followed by `line 4: field pane_argz not found in type config.ProviderEntry (unknown keys are rejected: fix the typo or delete the key)`
And C4 fails with the same error text as B for the same input, without a panic
And both C5 inputs fail with an error that names `pane_args` and ends in `(unknown keys are rejected: fix the typo or delete the key)`
And C6 fails like C3, and stderr carries no config notice

### S6: The config notice is exact, single, and quiet-aware
Priority: Must
Given C2 and C2' (C2 without its group K lines) in two workspaces, and stderr attached to a pseudo-terminal
When `auto check --arch` runs, then `auto check --arch --quiet`, then `auto check --arch` with stderr redirected to a pipe
Then the first run's stderr holds exactly one line `auto: warning: ignored removed autopus.yaml keys: <P2>; the orchestra pane backend was retired (SPEC-PANERM-001); run "auto update" or delete the keys`
And the `--quiet` run and the pipe run hold 0 notice lines
And for every run the expected stdout and exit code equal those of the same run on C2'
And two loads in one process through the same notifier emit the notice once

### S7: Every writer drops group K and keeps the rest
Priority: Must
Given workspaces holding C1, C2, and C7, and C2 plus `operator_extension: {credential_ref: ${OMP_SECRET}}` (C2o)
When (a) `config.Save` writes C2's loaded config, (b) `auto update` runs on C1 and C2 with no other migration condition, (c) `auto --config <path> quality supervisor quality` edits C2, (d) `auto --config <path> quality provider claude ultra --apply` edits C2, (e) `applyOMPProfile` with the fake OMP runner of `TestPlatformOMPProfileApplyPersistsAndRollsBackAtomically` edits C2o, and (f) `config.Load` rewrites C7, each writing `autopus.yaml` at its `--config` path or `<dir>`
Then each written file contains 0 occurrences of `pane_args`, `interactive_input`, `working_patterns`, `monitor_pattern_timeout_ms`, and no `enabled` key under `orchestra.subprocess`
And for (b) the written YAML node tree equals the input tree minus exactly the P1 or P2 entries
And (b), (c), (d), and (e) keep `# keep-me`, `${AUTOPUS_WORKDIR}`, and `future_extension` byte for byte, (e) keeps `operator_extension`, and (f) lists `claude-code`
And binary O loads every written file with exit status 0

### S8: Hidden no-op flags change nothing
Priority: Must
Given pairs with and without the flag: brainstorm `--no-detach` and `--subprocess`; plan `--no-detach` and `--subprocess`; review and secure `--no-detach`; run `--subprocess`; spec review `--subprocess` and `--plain`
When each pair runs with the same fake providers and inputs
Then recorded argv slices, backends, stdout bytes, stderr bytes, and exit code are equal within each pair
And the `templates/codex/skills/auto-review.md.tmpl:69` invocation at B, filled as `auto orchestra review a.go --risk-tier high --strategy debate --providers claude,codex --no-detach --format json`, exits 0 with the same expected stdout as without `--no-detach`
And the `--help` text of each command contains none of `--no-detach`, `--subprocess`, `--plain`, `--yield-rounds`

### S9: --yield-rounds runs every round and warns once
Priority: Must
Given `auto orchestra brainstorm "x" --providers claude,codex --rounds 2 --format json` with and without `--yield-rounds`
When both run
Then stdout bytes and exit codes are equal, and each provider records 2 rounds of calls in both runs
And the flagged run's stderr equals the unflagged stderr plus exactly one line `auto: warning: --yield-rounds was retired with the orchestra pane backend (SPEC-PANERM-001); all rounds run synchronously`

### S10: Retired subcommands fail with the migration message
Priority: Must
Given each name in collect, inject, cleanup, status, wait, result, invoked as `auto orchestra <name>`, as `auto orchestra <name> job-123 --timeout 60`, and as `auto --quality x --config /nonexistent/autopus.yaml orchestra <name> job-123`, in a workspace holding C7
When the command runs
Then stderr is exactly `Error: auto orchestra <name> was retired with the orchestra pane backend (SPEC-PANERM-001); orchestra commands now run synchronously and print their result directly`, stdout is empty, and the exit status is 1 for every form
And `autopus.yaml` keeps its bytes, stderr holds no config notice, the fake terminal records 0 calls, and snapshots of the workspace and `$TMPDIR` are unchanged
And `auto orchestra --help` lists brainstorm, plan, review, secure, run and none of the six names

### S11: auto update removes exactly the stale hook set
Priority: Must
Given W-claude, W-codex, W-agy, W-oc2, and W-oc1
When the new binary runs `auto update` once in each
Then the deleted files are exactly: W-claude the 7 `.claude/hooks/autopus/hook-*.sh` and the legacy `.ts`; W-codex the 2 `.codex/hooks/autopus/` scripts; W-agy the 2 `.gemini/hooks/autopus/` scripts; W-oc2 and W-oc1 the `.ts`
And the removed handlers are exactly the group S handlers of claude Stop and SessionStart, codex Stop and SessionStart, `.agents/hooks.json` Stop, and `.gemini/settings.json` AfterAgent; W-oc2 loses only the `file:` object entry and W-oc1 only the tuple and the absolute-path entries
And each mixed entry keeps its matcher and only `./scripts/notify.sh`, and each separate user entry, `{"package": "my-plugin"}`, and `./plugins/mine.ts` stay byte-identical in place
And every managed handler outside group S parses to the same tree as O's update output for the same workspace

### S12: Retraction is idempotent, bounded, atomic, and revertible
Priority: Must
Given the S11 workspaces after the first update, a fresh W-claude copy F, W-mix, W-oc-bad, and a scratch `HOME` whose `.claude/settings.json` holds a Stop handler for `hook-claude-stop.sh`
When `auto update` runs again in the S11 workspaces, F's first update runs with `adapter.transactionStepHook` failing the first write after the removes, W-mix and W-oc-bad are updated, and binary O runs `auto update` on the S11 results
Then H(w) after the second run equals H(w) after the first for every S11 workspace, so only the excluded transaction records differ
And the second run removes no file and leaves every file outside `.autopus/txns/`, `.autopus/backup/`, and the manifest `generated_at` fields byte-identical, and the `HOME` file is byte-identical and named by `auto doctor` as user-level
And F's claude-code settings, group S scripts, and manifest are byte-identical to before, stdout holds `  ✗ claude-code: ` with the injected error, stderr starts with `Error: 플랫폼 업데이트 실패: claude-code: `, the exit status is 1, and a clean rerun reaches the S11 end state
And W-mix keeps the `.ts` and its entry; W-oc-bad's update fails with `invalid native plugins array` and keeps `opencode.json` and the `.ts` byte-identical; `auto doctor` reports both scripts
And O's update restores its completion handlers and scripts, keeps every user handler byte-identical, and loads `autopus.yaml` with exit status 0

### S13: Doctor reports exactly what update deletes
Priority: Must
Given the S11 workspaces before the update, W-claude holding C2 as `autopus.yaml`
When `auto doctor --json` and `auto doctor` run, then `auto update`, then both doctor modes again
Then before the update `doctor.legacy_orchestra_config` is `warn` with message `legacy orchestra keys: <P2>` and remedy `run "auto update"`, and `doctor.stale_completion_hooks` is `warn` listing the S11 deletion set
And the set doctor reports equals the set the update deletes, and after the update both checks are `pass` in both modes
And after the update no check has status `warn` or `fail` with a message naming `hook-claude-stop.sh`, `hook-codex-stop.sh`, `hook-gemini-afteragent.sh`, `AUTOPUS_SESSION_ID`, or `monitor_pattern_timeout_ms`

### S14: Default-entry upgrade decisions are unchanged
Priority: Must
Given the claude, codex, and gemini entries of C1, entries equal to `historicalCanonicalCodexPaneArgs` and `v05066AutoPinnedCodexPaneArgs`, and a user codex entry whose only customization is `pane_args`
When the codex and claude default-entry detection runs at B (golden from T2) and after the change
Then every decision matches the golden expected value, except the six entries whose only difference from a default entry is a retired pane key (one claude entry and five codex entries, `panerm_default_entry_decisions_test.go`), which now decide like the default entry they otherwise equal, as the CHANGELOG states
And an entry whose codex model the user chose (user args, with or without `model_policy: pinned`) keeps it exactly as at B

### S15: No pane symbol remains reachable
Priority: Must
Given the merged change
When the extended `TestCLIProductionCodeNeverCallsPaneEntryPoints`, rebuilt deadcode, and `go run ./cmd/source-lines -max 0 -ext .go pkg/orchestra` run
Then the test passes, 0 of the 60 group P files exist, and deadcode prints 0 lines starting with `pkg/orchestra/` or `internal/cli/orchestra`
And the summed physical lines of non-test files is at most 9,945 (17,945 at B minus 8,000)
And `go list -f '{{join .Imports "\n"}}' ./pkg/orchestra` has no `pkg/terminal` import, and non-test `pkg/orchestra` files call no `.SendCommand(`, `.SendLongText(`, or `.ReadScreen(`
And the guard self-test, fed a Go file that declares `RunPaneOrchestra` and references `PaneArgs`, fails with one finding per identifier

### S16: Output and receipt parity
Priority: Must
Given the T2 goldens for brainstorm, plan, review, secure, run, and recheck `--format json` stdout, and for `auto spec review` (`review-receipt.json` and review.md Provider Health rows), all with fake providers
When the same invocations run after the change
Then stdout, `review-receipt.json`, and the Provider Health rows are byte-identical after normalizing only the volatile fields T2 lists (timestamps, durations, run ids, temp paths)
And every `receipt.provider_receipts[].backend` and `providers[].executed_backend` equals its golden expected value

### S17: Subprocess engine tests stay green with bounded diffs
Priority: Must
Given `pkg/orchestra/provider_argv_test.go`, `subprocess_schema_test.go`, `subprocess_judge_session_evidence_test.go`, and `internal/cli/provider_argv_test.go`
When `git diff` against B runs on them and `go test -race ./...` runs
Then the first two show no diff, and `internal/cli/provider_argv_test.go` loses exactly `TestProviderConfig_GeminiPaneNoPrint` and `TestProviderConfig_CodexPaneNotExec` (ORCH-021 S17, S19)
And `subprocess_judge_session_evidence_test.go` loses only table fields, rows, and imports that set `Terminal`, `SubprocessMode`, or `pkg/terminal` values, and every remaining row keeps its expected value
And `TestBuildSubprocessArgs_GeminiPrintValueSlot`, `TestBuildSubprocessArgs_CodexExecSchema`, `TestProviderConfig_CodexStructuredSchema`, and `TestResolveSpecReviewProviders_DefaultIncludesCodex` (ORCH-021 S15, S16, S18, S20) pass unchanged
And the race-suite failure set is a subset of the T2 baseline, and `go doc -all ./pkg/terminal` equals its output at B

### S18: Instructions, docs, and the instruction guard
Priority: Must
Given a guard self-test fixture holding `--yield-rounds`, `auto orchestra wait`, `pane_args`, `--no-detach`, and `--subprocess` on separate lines of a template file
When the guard runs over the fixture and over `content/`, `templates/`, and `configs/`
Then the fixture run fails with one finding per line in the form `<file>:<line>: retired token "<token>"`, and the tree run passes
And `README.md` lines 497-509 equal B, and the CHANGELOG entry names the 5 keys, 4 flags, 6 subcommands, 9 group H assets, B, `7c781509`, and `16b50216`
And every regenerated file in the diff maps to a changed canonical source

### S19: Headless subscription runs succeed before deletion
Priority: Must
Given RFP-1's setup with the API key variables unset
When the operator runs the RFP-1 brainstorm in a plain shell, in cmux or tmux, and from a Claude Code Bash tool
Then each stdout has `schema` = `orchestration_cli_result.v1` and, for claude, codex, and gemini, `receipt.provider_receipts[]` rows with `backend` = `subprocess`, `exit_code` 0, `timed_out` false, `usable` true, and no `failure_class`
And the operator confirms subscription billing for each provider

### S20: Superseded notes on pane-centric SPECs
Priority: Nice
Given the SPEC list of PRD FR-30
When T18 lands
Then each listed `spec.md` gains exactly one header line naming SPEC-PANERM-001, and no other line changes

## Oracle Acceptance Notes

- Every Must scenario names a concrete expected output: exact stderr lines, path lists, receipt field values, argv
  slices, byte-, hash-, or tree-identical files, set equality, or a numeric bound (S2 process count, S15 LOC bound).
  An exit code, file existence, a heading, or non-empty output never closes a scenario on its own.
- Heterogeneous entities: three provider CLIs plus OMP; seven workspace kinds; A34-generated and hand fixtures;
  literal, wildcard, reserved, typo, misplaced, and non-mapping keys; TTY, `--quiet`, and pipe stderr; separate, mixed,
  user-level, legacy, object, tuple, `file:`, absolute, invalid, and package plugin entries.
- Paired oracles compare with B or O on the same input (S3, S5, S7, S11, S12, S14, S16, S17). Discriminators: S5 C5,
  S6 pipe run, S10 root flags, S11 entry forms, S12 fault and W-oc-bad, S13 set equality. RFP-1 (S19) is operator-run.
