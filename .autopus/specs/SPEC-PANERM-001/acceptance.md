# SPEC-PANERM-001 수락 기준

## Test Scenarios

Fixtures (T3). C1: the A34 default full config written by an `auto` binary built from `c447badc` (v0.50.123); its
group K paths are exactly `features.cc21.monitor_pattern_timeout_ms` (30000), `orchestra.providers.claude.pane_args`
(`[--print, --model, claude-fable-5-1, --effort, max]`), `orchestra.providers.codex.pane_args`
(`[-m, gpt-6-astra, -c, model_reasoning_effort="max"]`), and `orchestra.providers.gemini.interactive_input` (`stdin`).
C2: hand fixture with all five group K keys under providers `claude`, `codex`, and `my-local`, plus retained values
`my-local.prompt_via_args: true`, `orchestra.subprocess.max_concurrent: 2`, `orchestra.subprocess.rounds: 3`,
`orchestra.subprocess.work_dir: "${AUTOPUS_WORKDIR}"`, and `features.cc21.monitor_enabled: true`; its pruned paths are
P2 = `features.cc21.monitor_pattern_timeout_ms, orchestra.providers.claude.pane_args,
orchestra.providers.claude.working_patterns, orchestra.providers.codex.interactive_input,
orchestra.providers.my-local.interactive_input, orchestra.providers.my-local.pane_args,
orchestra.providers.my-local.working_patterns, orchestra.subprocess.enabled`. C3: `orchestra.providers.codex.pane_argz:
[x]`. C4: `orchestra.providers.codex: "x"` and, separately, `orchestra.providers: [a, b]`. C5: `orchestra.pane_args: [x]`
and `orchestra.providers.codex.subprocess.pane_args: [x]`. C6: C2 plus C3's line. Fake providers record argv; the clock,
terminal, and model backends are fakes; text compares exactly unless a scenario names a normalization.

### S1: Every command dispatches to subprocess or OMP
Priority: Must
Given `TMUX` set so that terminal detection reports tmux, a recording fake terminal, CLI providers claude, codex, gemini, and one provider with `backend: omp`
When `auto orchestra brainstorm`, `plan`, `review`, `secure`, `run`, the `recheck` strategy, and `auto spec review` execute
Then every CLI provider reports `executed_backend` = `subprocess` and the OMP provider reports `omp`
And the fake terminal records 0 calls, and no file appears under the former detach job directory

### S2: Backend selection has no pane branch
Priority: Must
Given orchestra configs for strategies consensus, debate, pipeline, relay, fastest, and recheck
When `SelectBackend`, `recheckTransport`, and `RunOrchestra` run with the `newCommand` seam recorded
Then `SelectBackend` and `recheckTransport` return backend name `subprocess` for every config
And a one-round consensus run with 3 providers and no judge records exactly 3 provider processes, one per provider

### S3: The OMP route and missing-binary error stay as at 7c781509
Priority: Must
Given a provider with `backend: omp` and a provider whose binary is not on `PATH`
When `selectRoutedBackend` resolves both
Then the OMP provider routes to the OMP backend and the missing-binary error text equals the `7c781509` text byte for byte

### S4: Legacy configs load and removed values are ignored
Priority: Must
Given workspaces holding C1 and C2
When `config.Load` runs
Then both load with a nil error and the pruned path lists equal C1's four paths and P2 in that order
And for C2 the loaded values are `prompt_via_args` true, `max_concurrent` 2, `rounds` 3, and `monitor_enabled` true

### S5: Typos and misplaced keys still fail
Priority: Must
Given C3, C4, C5, and C6
When `config.Load` runs
Then C3 fails with `yaml: unmarshal errors:` followed by `line 4: field pane_argz not found in type config.ProviderEntry (unknown keys are rejected: fix the typo or delete the key)`
And C4 fails with the same error text as `7c781509` for the same input, without a panic
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

### S7: Save round trip across binaries
Priority: Must
Given C1 and C2 workspaces
When the new binary runs `auto update`
Then the written `autopus.yaml` contains 0 occurrences of `pane_args`, `interactive_input`, `working_patterns`, `monitor_pattern_timeout_ms`, and no `enabled` key under `orchestra.subprocess`
And it equals the file the `c447badc` binary writes for the same input, after deleting group K keys from both, and keeps the literal `${AUTOPUS_WORKDIR}`
And the `c447badc` binary loads the written file with exit status 0

### S8: Hidden no-op flags change nothing
Priority: Must
Given pairs with and without the flag: brainstorm `--no-detach` and `--subprocess`; plan `--no-detach` and `--subprocess`; review and secure `--no-detach`; run `--subprocess`; spec review `--subprocess` and `--plain`
When each pair runs with the same fake providers and inputs
Then recorded argv slices, `executed_backend`, stdout bytes, stderr bytes, and exit code are equal within each pair
And the `templates/codex/skills/auto-review.md.tmpl:69` invocation at `7c781509`, filled as `auto orchestra review a.go --risk-tier high --strategy debate --providers claude,codex --no-detach --format json`, exits 0 with the same expected stdout as without `--no-detach`
And the `--help` text of each command contains none of `--no-detach`, `--subprocess`, `--plain`, `--yield-rounds`

### S9: --yield-rounds runs every round and warns once
Priority: Must
Given `auto orchestra brainstorm "x" --providers claude,codex --rounds 2 --format json` with and without `--yield-rounds`
When both run
Then stdout bytes and exit codes are equal, and each provider records 2 rounds of calls in both runs
And the flagged run's stderr equals the unflagged stderr plus exactly one line `auto: warning: --yield-rounds was retired with the orchestra pane backend (SPEC-PANERM-001); all rounds run synchronously`

### S10: Retired subcommands fail with the migration message
Priority: Must
Given each name in collect, inject, cleanup, status, wait, result, invoked with no arguments and with `job-123 --timeout 60`
When the command runs
Then stderr is exactly `Error: auto orchestra <name> was retired with the orchestra pane backend (SPEC-PANERM-001); orchestra commands now run synchronously and print their result directly`, stdout is empty, and the exit status is 1
And the fake terminal records 0 calls and a snapshot of the workspace and the temp directory is unchanged
And `auto orchestra --help` lists brainstorm, plan, review, secure, run and none of the six names

### S11: auto update removes exactly the stale hook set
Priority: Must
Given scratch workspaces initialized by the `c447badc` binary for claude-code, codex, antigravity-cli, and opencode; one user hook per settings file placed before the managed entries; for opencode, `opencode.json` plugin entries `.claude/hooks/autopus/hook-opencode-complete.ts` (file present) and `./plugins/mine.ts`
When the new binary runs `auto update`
Then the deleted files equal the group S scripts present before: 7 `.claude/hooks/autopus/hook-*.sh` (claude-code), 2 under `.codex/hooks/autopus/` (codex), 2 under `.gemini/hooks/autopus/` (antigravity-cli), and the planted `.ts` (opencode)
And the removed settings entries are exactly claude Stop and SessionStart, codex Stop and SessionStart, `.agents/hooks.json` Stop, `.gemini/settings.json` AfterAgent, and the managed opencode plugin entry
And each user hook keeps its command, matcher, timeout, and position, `./plugins/mine.ts` stays, and every other managed hook equals the `c447badc` binary's update output for the same workspace

### S12: Retraction is idempotent, bounded, and atomic
Priority: Must
Given the S11 workspaces committed after the first update, and a scratch `HOME` whose `.claude/settings.json` holds a Stop entry for `hook-claude-stop.sh`
When `auto update` runs a second time, and separately a third run is given an injected write failure
Then `git status --porcelain` is empty after the second run, and the `HOME` settings file is byte-identical and named by `auto doctor` as user-level
And after the injected failure every file is byte-identical to its state before that run
And a workspace whose `opencode.json` references the `.ts` while the opencode platform is not updated keeps the `.ts`, and doctor reports it

### S13: Doctor reports exactly what update deletes
Priority: Must
Given the S11 workspaces before the update, the claude-code one holding C2 as `autopus.yaml`
When `auto doctor --json` and `auto doctor` run, then `auto update`, then both doctor modes again
Then before the update `doctor.legacy_orchestra_config` is `warn` with message `legacy orchestra keys: <P2>` and remedy `run "auto update"`, and `doctor.stale_completion_hooks` is `warn` listing the S11 deletion set
And the set doctor reports equals the set the update deletes, and after the update both checks are `pass` in both modes
And after the update no doctor check has status `warn` or `fail` with a message naming `hook-claude-stop.sh`, `hook-codex-stop.sh`, `hook-gemini-afteragent.sh`, `AUTOPUS_SESSION_ID`, or `monitor_pattern_timeout_ms`

### S14: Default-entry upgrade decisions are unchanged
Priority: Must
Given the claude, codex, and gemini entries of C1, entries equal to `historicalCanonicalCodexPaneArgs` and `v05066AutoPinnedCodexPaneArgs`, and a user codex entry whose only customization is `pane_args`
When the codex and claude default-entry detection runs at `7c781509` (golden from T2) and after the change
Then every decision matches the golden expected value, except the `pane_args`-only entry, which becomes a default entry as the CHANGELOG states

### S15: No pane symbol remains reachable
Priority: Must
Given the merged change
When `rg -n -w` over non-test Go files in `pkg/orchestra` and `internal/cli` searches the group I list, `ls pkg/orchestra` is matched against group P, rebuilt deadcode runs, and `go run ./cmd/source-lines -max 0 -ext .go pkg/orchestra` runs
Then rg reports 0 matches, 0 of the 60 group P files exist, and deadcode prints 0 lines starting with `pkg/orchestra/` or `internal/cli/orchestra`
And the summed physical lines of non-test files is at most 9,945 (17,945 minus 8,000)
And `go list -f '{{join .Imports "\n"}}' ./pkg/orchestra` has no `pkg/terminal` import, and rg lists `dangerously-skip-permissions` in non-test `pkg/orchestra` files only on bypass-detection lines such as `provider_execution.go:75`

### S16: Output contract parity
Priority: Must
Given the T2 golden `--format json` stdout and receipts for brainstorm, plan, review, run, and recheck with fake providers
When the same invocations run after the change
Then stdout and receipts are byte-identical after normalizing only the volatile fields T2 lists, and `executed_backend` keeps its golden values

### S17: Subprocess engine tests stay unchanged and green
Priority: Must
Given `pkg/orchestra/provider_argv_test.go`, `subprocess_schema_test.go`, `subprocess_judge_session_evidence_test.go`, and `internal/cli/provider_argv_test.go`
When `git diff 7c781509` runs on them and `go test -race ./...` runs
Then the first two show no diff, the last two show only deleted lines that set group F fields, and SPEC-ORCH-021 argv oracles S15–S20 keep their expected values
And the race-suite failure set is a subset of the T2 baseline, and `go doc -all ./pkg/terminal` equals its `7c781509` output

### S18: Instructions, docs, and guard
Priority: Must
Given the guard self-test fixture holding `--yield-rounds`, `auto orchestra wait`, `pane_args`, `--no-detach`, and `--subprocess` on separate lines
When the guard runs over the fixture and over `content/`, `templates/`, and `configs/`
Then the fixture run fails with one finding per line in the form `<file>:<line>: retired token "<token>"`, and the tree run passes
And `README.md` lines 497-509 equal `7c781509`, the CHANGELOG entry names the 5 keys, 4 flags, 6 subcommands, 9 group H assets, and `7c781509`, and each FR-30 SPEC gains exactly one header line
And every regenerated file in the diff maps to a changed canonical source

### S19: Headless subscription runs succeed before deletion
Priority: Must
Given RFP-1's setup with the API key variables unset
When the operator runs the RFP-1 brainstorm in a plain shell, in cmux or tmux, and from a Claude Code Bash tool
Then each run's JSON shows claude, codex, and gemini with status `success` and `executed_backend` = `subprocess`, and the operator confirms subscription billing

## Oracle Acceptance Notes

- Each Must scenario names a concrete expected output: exact stderr lines, exact path lists, argv slices, byte-identical
  files, set equality, or a numeric bound (S2 process count, S15 LOC bound). Exit code, file existence, a heading, or
  non-empty output never closes a scenario on its own.
- Heterogeneous entities: three provider CLIs plus OMP; four platforms; A34-generated and hand fixtures; literal,
  wildcard, typo, misplaced, and non-mapping keys; TTY, `--quiet`, and pipe stderr; user, managed, user-level, and
  out-of-band hooks.
- Paired oracles compare the new binary with `c447badc` or `7c781509` on the same input (S3, S5, S7, S11, S14, S16).
- Wrong-implementation discriminators: S5 C5 (wildcard over-match), S6 pipe run (unconditional notice), S11 user hook
  order (retraction by prefix only), S12 opencode `.ts` (dangling plugin), S13 set equality (doctor drift).
- Live evidence: RFP-1 (S19) is operator-run and paid; RFP-2 and RFP-3 run on scratch fixtures with no network.
