# Harness golden set (SPEC-HARNEVAL-001)

This directory is the committed golden set that `auto eval harness` evaluates.
The deterministic PR lane generates the five platform surfaces in-process with
every input pinned, evaluates the surface tasks, and compares them with
`baseline.json`. Agent tasks are never run by the PR lane; it checks their
schema and corpus digest only. The maintainer live lane (T10-T13) runs them.

| Path | Content |
|------|---------|
| `manifest.json` | `harness_golden_set.v1`: active paths, floors, live policy, pins |
| `baseline.json` | `harness_eval_baseline.v1`, written by `auto eval harness baseline --init` on this tree; 48 rows (35 active and 1 retired surface, 12 agent) |
| `fixtures/codex-models.json` | the pinned Codex model catalog (`pins.codex_model_catalog`) |
| `tasks/surface/*.json` | 35 active surface tasks and 1 retired tombstone |
| `tasks/agent/*.json` | 12 active agent tasks, one per benchmark corpus task; 5 of them also carry a black-box oracle |
| `oracles/<task id>/` | the input fixtures and expected outputs the black-box oracles pin (see Black-box oracles) |
| `candidates/` | the SPEC-HARNEVAL-002 intake area: open candidates, `promoted/` link records, `rejected/` records; never loaded (see Incident intake) |

## Commands

```sh
go run ./cmd/auto eval harness run --format json            # result document on stdout
go run ./cmd/auto eval harness digest --format json         # set, agent set, surface digests
go run ./cmd/auto eval harness baseline --update [--accept-regression GT-ID --reason "..."]
go run ./cmd/auto eval harness report --input <session-dir> --format json  # unsigned live advisory report
go test ./pkg/harneval -run GoldenSet                       # REQ-HE-13 coverage test (S14)
go test ./pkg/harneval -run SeededMutations                 # REQ-HE-12 mutation table (S13)
```

Run with `CODEX_HOME` pointing at an empty directory for a hermetic shell. The
generation itself never reads the host: `PATH` and `HOME` are replaced by
sentinels while it runs.

## CI gate

The `harness-eval` job in `.github/workflows/ci.yaml` always runs and always
reports its check; there is no paths filter and no job-level `if`. For a pull
request it lists `git diff --name-only --no-renames <base>...<head>` and asks
`auto eval harness applicable`; when no changed path meets the harness input
set the job ends `not_applicable` and passes. Every other event (push to main,
the release `workflow_call`) is evaluated. The evaluation is
`auto eval harness run --format json --output <file> --summary "$GITHUB_STEP_SUMMARY"`:
any failure reason fails the job, the job summary gets the category table and
the transitions (never task text), and the result document is uploaded as the
`harness-eval` artifact. Blocking merges also needs `harness-eval` registered
as a required status check on main (T14, an OPS step).

## Pins and live policy

| Field | Value | Why |
|-------|-------|-----|
| `pins.generator_version` | `v0.50.123` | the release coordinate of this tree; any build gives the same surface |
| `pins.project_name` | `harness-golden` | fixed name for every generated document (asserted by GT-PROMPT-PROJECT-IDENTITY) |
| `pins.codex_model_catalog` | `fixtures/codex-models.json` | advertises every model and effort the default config places, so no profile is substituted and no fallback diagnostic is printed |
| `pins.codex_cli_version` | `codex-cli 0.160.0` | the maintainer host's `codex --version`; the live lane refuses a different CLI |
| `pins.opencode_cli_version` | `1.18.7` | the OpenCode V1 contract, the pin of the adapter contract tests and probe A2 |
| `live.workspace_revision` | `a05ce69df9dc03493b8c5de0ab9299459195e8d8` | the revision the corpus mutations were written against (pilot `--revision`); all 12 match exactly once there |
| `live.baseline_ref` | `v0.50.123` | the latest release tag, an ancestor of this tree |
| `live.model` | `gpt-6-astra` | the pilot default model |
| `live.k`, `threshold_bp`, `completeness_floor` | 2, -1000, 0.9 | SPEC initial values |
| `live.max_agent_runs` | 48 | exactly 12 tasks x K 2 x 2 arms; growing the set or K must raise the cap on purpose |
| `live.trial_timeout_seconds` | 180 | the pilot deadline |

## Coverage (REQ-HE-13)

`floors` are the SPEC floors, so a run below them is `vacuous`: 20 surface and
12 agent tasks. The coverage test checks the remaining breadth floors.

| Measure | Floor | Committed |
|---------|-------|-----------|
| active surface tasks | 20 | 35 |
| assertion platforms | 5 | 5 |
| surface categories | 4 | 5 |
| multi-platform or multi-path ratio | 0.60 | 34/35 = 0.97 |
| active agent tasks | 12 | 12 |

Surface tasks by category:

- `routing`: GT-ROUTE-CLAUDE-DETAILS, GT-ROUTE-ANTIGRAVITY-DETAILS, GT-ROUTE-CODEX-SKILL-NAMES,
  GT-ROUTE-OMP-EXACT-MAP, GT-ROUTE-OPENCODE-DETAILS, GT-ROUTE-ALIASES, GT-ROUTE-ANTIGRAVITY-COMMANDS
- `hooks_settings`: GT-HOOK-ARCH-GATE-ON, GT-HOOK-ARCH-GATE-OPT-OUT (variant `hooks.pre_commit_arch=false`),
  GT-HOOK-COMPLETION-RETIRED, GT-HOOK-EDIT-GUARD-ON, GT-HOOK-CLAUDE-RULE-DISPATCH, GT-SETTINGS-MCP-SERVERS,
  GT-SETTINGS-PERMISSION-SCOPE, GT-HOOK-OPENCODE-PLUGIN, GT-HOOK-LORE-COMMIT-MSG
- `prompt_contract`: GT-PROMPT-TRIAGE-PARITY, GT-PROMPT-THIN-ROUTER, GT-PROMPT-WORKER-RECEIPT,
  GT-PROMPT-DELEGATION-RULE, GT-PROMPT-SHELL-PORTABILITY, GT-PROMPT-REVIEW-CONVERGENCE,
  GT-PROMPT-PROJECT-IDENTITY, GT-PROMPT-WORKER-HYGIENE
- `agent_skill_exposure`: GT-AGENT-READONLY-REVIEW, GT-AGENT-PIPELINE-ROLES, GT-SKILL-CORE-CATALOG,
  GT-SKILL-CLAUDE-NATIVE-ORCHESTRATION, GT-SKILL-CODEX-PLUGIN-ENTRY, GT-SKILL-UI-CRITIQUE-LOOP,
  GT-SKILL-UI-VISUAL-DETERMINISM
- `generated_root_hygiene`: GT-HYGIENE-MANAGED-BLOCKS, GT-HYGIENE-SHARED-AGENTS-ROOT,
  GT-HYGIENE-REFERENCE-INTEGRITY, GT-HYGIENE-GENERATED-SURFACE-SAFETY

Retired (2026-10-08): GT-HOOK-SESSION-LIFECYCLE, whose session lifecycle hooks
SPEC-PANERM-001 REQ-12 removed with the orchestra pane backend.
GT-HOOK-COMPLETION-RETIRED now asserts that none of those scripts or handlers
is generated, and GT-HOOK-EDIT-GUARD-ON took over its Codex `hooks = true`
check, since the Codex edit guard (SPEC-EDITGUARD-001) is a hook too. On the
pinned surface of the integration tree before SPEC-PANERM-001 and
SPEC-EDITGUARD-001 (`662eee2e`), every assertion of GT-HOOK-COMPLETION-RETIRED
and every guard assertion of GT-HOOK-EDIT-GUARD-ON failed; the two that hold
either way are the Codex hooks feature and the absent guard on the
advisory-only `.agents/hooks.json` lane.

Each task's `provenance.ref` names the canonical source the behavior comes
from. A task must assert a cross-cutting behavior of the generated surface; a
task that restates one existing contract test assertion is rejected in review.
`file_absent` reads the generated tree on disk as well as ownership: a path
counts as absent on a platform when nothing is there, or when what is there
another platform's adapter reported (the five platforms share one root). An
entry that no adapter reported fails on every platform (`present_unreported`),
so an unreported write cannot pass as absent.

## Agent tasks and calibration

Each agent task pins `corpus_ref.file_sha256` to the raw bytes of
`scripts/benchmarks/harness/corpus_a.json` or `corpus_b.json` and fixes the
top-level oracle tests in `expected_tests` (a04 uses the prefix pattern
`^TestEvaluateGate_`, so its seven names cannot be read from the pattern).

The names come from a two-direction calibration run on 2026-10-07 against a
`git archive` snapshot of `live.workspace_revision`, using `go test -json` with
the corpus oracle command. No agent and no model was called.

- clean snapshot: 12/12 oracles exit 0 and every expected test has a pass event;
- mutated snapshot (corpus mutation applied): 12/12 oracles exit 1 and are not accepted.

| Task | Expected tests | Failing after the mutation |
|------|----------------|----------------------------|
| GT-AGENT-A01 | 3 | TestVersionMismatchWinsOverUnknown |
| GT-AGENT-A02 | 1 | TestAggregateUsage_DuplicateIdentity_DeduplicatesOrBlocksOnConflict |
| GT-AGENT-A03 | 3 | TestCompareManifestsReportsMetadataChanges |
| GT-AGENT-A04 | 7 | TestEvaluateGate_ConflictingOrUnknownVerdict_FailsClosed |
| GT-AGENT-A05 | 3 | TestStrictDecode, TestStrictPolicyAndReplayDocuments |
| GT-AGENT-A06 | 3 | TestHarnessIncompatibleIdentityHasNoDelta, TestHarnessOverallCompletenessRequiresComparablePairsAndCorrections |
| GT-AGENT-B01 | 1 | TestParallelDependencyOutputAndInputOrder |
| GT-AGENT-B02 | 1 | TestValidateOwnershipSupervisorBoundary |
| GT-AGENT-B03 | 1 | TestReuse_MaxAgeBoundaryIsInclusive |
| GT-AGENT-B04 | 1 | TestClassifyChange_Table |
| GT-AGENT-B05 | 1 | TestConfigOperations_RejectSymlinkedConfig |
| GT-AGENT-B06 | 1 | TestPluginConfigNewVersionShapeAndPathAliases |

The same calibration inside the `grader.sb` profile with the read-only module
cache (REQ-HE-09) ran on 2026-10-07 with
`scripts/benchmarks/harness/prepare_grader.py`: 12/12 clean accepted, 12/12
mutated rejected, with the failing tests above. With an empty module cache
the ten tasks whose packages import an external module fail to build with no
pass event; GT-AGENT-A01 and GT-AGENT-A05 (`pkg/skillpolicy`, standard library
only) still pass, so the calibration is refused through the other ten
(`.autopus/specs/SPEC-HARNEVAL-001/evidence/t11-grader.txt`). The live lane
runs this calibration before any trial and refuses the session with
`oracle_calibration_failed` if a name here is wrong.

## Black-box oracles (SPEC-HARNEVAL-003 T15)

The signed live lane judges only agent tasks whose document carries
`oracle_mode: black_box` and a `black_box_oracle` (format: the signed-lane
section of `scripts/benchmarks/harness/README.md`). The trusted runner builds
`./cmd/auto` from the agent-modified `live.workspace_revision` snapshot, runs
the command below under `artifact.sb` with the inputs copied into `{input}`,
and the trusted oracle harness compares the exit status and the whole stdout
with the pinned expectations. The Go loader applies the runner's definition
rules, requires every fixture below `evals/harness/oracles/` (removed from
every agent, build, and artifact snapshot), and reads each one like a corpus
file: a byte or size drift (inputs up to 16 MiB, expected outputs up to 1 MiB)
is `invalid` with detail `oracle_digest_mismatch`. `floors.signed_agent_tasks`
is 5: a signed session with fewer black-box tasks is `vacuous`.

| Task | Command after `{artifact}` | Expected |
|------|----------------------------|----------|
| GT-AGENT-A01 | `skill select --policy-json {input}/policy.json --task-json {input}/task.json --dir {input}` | exit 0; `version-mismatch` stays `excluded` (mutation: `unknown`) |
| GT-AGENT-A02 | `telemetry team --evidence-json {input}/team.json --format json` | exit 0; a call and its retry in one run count twice (mutation: once) |
| GT-AGENT-A05 | `skill select --policy-json {input}/policy.json --task-json {input}/task.json --dir {input}` | exit 1, empty stdout: a nested duplicate key is refused (mutation: accepted); positive control: the same document without the repeat is selected |
| GT-AGENT-A06 | `telemetry harness --evidence-json {input}/evidence.json --format json` | exit 0; the incompatible-identity task leaves the two pairs it would join (mutation: paired) |
| GT-AGENT-B04 | `spec gates {input} --changed pkg/a/x.go,pkg/b/schema.go --read-only` | exit 0; one schema path across two modules is `security_or_data` (mutation: `multi_domain`) |

Each expected stdout is what the clean reference artifact of the workspace
revision prints. Calibration on 2026-10-09 (macOS 26.5.2, go1.27.0, no agent
and no model, `golden_blackbox_trial.calibrate` under both profiles): 5/5
clean artifacts `accepted`, 5/5 mutated ones `expectation_mismatch`
(`.autopus/specs/SPEC-HARNEVAL-003/evidence/t15-calibration.txt`). GT-AGENT-A05
also pins a positive control (`control/task.json` without the duplicate key,
with the same `policy.json`, must exit 0 and print `control/stdout.json`), so
an `auto` that refuses every document fails it: recalibrated the same day,
clean `accepted`, mutated and a refuse-everything decoder both
`expectation_mismatch`.

The SPEC's first five (a06, b03, b04, b05, b06) were not all observable with
this format. b03 needs prior gate evidence at `{SPEC_DIR}/gates/` while inputs
are copied flat into `{input}`; b05 needs a symlinked `autopus.yaml` and b06
an existing `opencode.json` naming the project's absolute path, while inputs
are read-only regular files and the output root starts empty. a01, a02 and
a05 took their places. The other 7 tasks stay white-box in the advisory lane.

## Live advisory report (REQ-HE-10, REQ-HE-11)

`auto eval harness report` judges one live session directory written by the
trusted runner (`python3 scripts/benchmarks/harness/run.py --mode golden`, see
`scripts/benchmarks/harness/README.md`) and prints the unsigned
`harness_live_advisory.v1` report on stdout. The report carries the
protocol's `runner_sha256`, the tree digest of the whole runner file set
(golden runner, grader, `grader.sb`, trusted preparation and the pilot modules
they load), and `grader_profile_sha256`, the SHA-256 of `grader.sb`. It exits 0 whatever the verdict and 1 only when the session cannot be
judged. The report gates nothing: `auto check --eval-regression` rejects it as
`artifact_unsigned` before decoding it. `grader.jsonl` is diagnostic and never
read. `pkg/harneval/testdata/live-session/` is a complete reference session.

| File | Document | When absent |
|------|----------|-------------|
| `protocol.json` | `harness_golden_live_protocol.v1`; `policy` is the manifest `live` block | invalid input, exit 1 |
| `calibration.json` | `harness_golden_calibration.v1`; `after` only once a trial ran | verdict `vacuous` |
| `records.jsonl` | one `harness_golden_live_record.v1` per line | no record |

Every document is decoded strictly (unknown fields and trailing data are
invalid). `schema_version` is optional on these three; when present it must be
the document's own identifier. A record's `signal` fixes its `outcome` by the
REQ-HE-08 table, and an `error` record never has `oracle.ran` true. Each
document is read up to 64 MiB; a larger one is `read_failed`. Calibration is
judged first: without a passed `before`, any record is
`records_protocol_mismatch`. With one, every record must be an `order` attempt
of the session, at most once. `after` is recorded only once every trial has
ended, so with `after` every attempt must have its record; without it the
session stopped early, may hold any part of the order, and reports `vacuous`
with calibration `missing`. A mismatch writes no report.

| Verdict (precedence) | Reason | When |
|----------------------|--------|------|
| `vacuous` | `oracle_calibration_failed` | `calibration.json` absent, or `before` or `after` not `passed` (an absent `after` is calibration `missing`) |
| `vacuous` | `oracle_not_run` | an arm has no record with `oracle.ran` true (build failures only). Known limitation: an arm whose every trial is `scope_violation` or `forbidden_construct` skips grading, so it also lands here rather than in `regression` |
| `vacuous` | `agent_all_failed` | no trial of either arm got past the agent step (`agent_launch_failed`, `agent_exit_nonzero`, `agent_timeout`, `observation_failed` or error only), e.g. missing credentials; one arm alone failing at the agent step is still judged |
| `incomplete` | `completeness_below_floor` | completeness < `policy.completeness_floor` |
| `incomplete` | `no_valid_trial` | an arm has no non-error trial (defensive: such an arm has no run oracle, so a decoded session meets `oracle_not_run` first) |
| `regression` | `hard_flip` | a task passed K/K in the baseline and 0/K in the candidate |
| `regression` | `pass_rate_regression` | `10000·(cp·bv − bp·cv) < threshold_bp·bv·cv` |
| `ok` | `within_threshold` | otherwise; a delta exactly at the threshold is no regression |

## Seeded mutations (REQ-HE-12)

`go test ./pkg/harneval -run SeededMutations` runs the committed mutation
table in `pkg/harneval/mutation_test.go`. It generates the pinned surfaces
once and, inside the run's mutation seam (after generation, before any
assertion), applies one row at a time, evaluates and compares the surfaces
with this baseline exactly as a run does, and undoes the edit. Every row must
fail with reason `regression` and exactly the tasks below; the unmutated
surfaces pass every active surface task before the first row and after the
last. A row whose target text is gone fails the test instead of passing
without having applied.

| Row | Defect (the source change it stands for) | Regressed tasks |
|-----|------------------------------------------|-----------------|
| M1 | managed PreToolUse rule-dispatch hook removed from `.claude/settings.json` | GT-HOOK-ARCH-GATE-OPT-OUT, GT-HOOK-CLAUDE-RULE-DISPATCH |
| M2 | Claude router maps `plan` to `auto-go` (`templates/claude/commands/auto-router.md.tmpl`) | GT-ROUTE-CLAUDE-DETAILS |
| M3 | one Task Triage bullet dropped on Antigravity (`templates/gemini/commands/auto-router.md.tmpl`) | GT-PROMPT-TRIAGE-PARITY |
| M4 | `hooks.pre_commit_arch` ignored: the `false` variant gets the flag-on surface (`pkg/content/hooks.go`) | GT-HOOK-ARCH-GATE-OPT-OUT |
| M5 | the `tdd` skill no longer exposed on OMP | GT-SKILL-CORE-CATALOG |
| M6 | reviewer gains Write, Edit (`content/agents/reviewer.md`) | GT-AGENT-READONLY-REVIEW |
| M7 | harness-workflow loses its claude-only gating (`content/skills/harness-workflow.md`) | GT-SKILL-CLAUDE-NATIVE-ORCHESTRATION |
| M8 | OpenCode after-hook runs for every tool (`pkg/adapter/opencode/opencode_plugin.go`) | GT-HOOK-OPENCODE-PLUGIN |
| M9 | Codex marketplace points at `./.autopus/plugins/autopus` (`pkg/adapter/codex/codex_plugin_manifest.go`) | GT-SKILL-CODEX-PLUGIN-ENTRY |
| M10 | Claude managed-block markers renamed (`pkg/adapter/claude/claude.go`) | GT-HYGIENE-MANAGED-BLOCKS |
| M11 | OMP `/auto-plan` loads the router instead of the detail (`pkg/adapter/omp/omp_commands.go`) | GT-ROUTE-OMP-EXACT-MAP |
| M12 | lore-commit loses its hook condition (`content/rules/lore-commit.md`) | GT-HOOK-CLAUDE-RULE-DISPATCH |
| M13 | Claude Code lane no longer `enforced`, so its edit guard is not registered in `.claude/settings.json` (`pkg/editguard/matrix.go`) | GT-HOOK-EDIT-GUARD-ON |
| M14 | legacy `/auto:plan` loads `auto-go` (`templates/gemini/commands/auto/plan.toml.tmpl`) | GT-ROUTE-ANTIGRAVITY-COMMANDS |

M1-M5 are the five REQ-HE-12 classes. An edit lands on every generated
variant that holds its target, as a source change would, so M1 also regresses
the opt-out variant task. M2-M4 and M6-M14 began as a manual source
spot-check on 2026-10-07: each edited one canonical source or template in a
scratch copy of this tree, `auto` was rebuilt from it, and `auto eval harness
run` regressed exactly the task above. The generators read `content/` and
`templates/` through `go:embed`, so a source edit needs a binary of its own;
each row is instead the surface edit its source change made, and it regresses
the same task.

M13 was replaced on 2026-10-08. Its first defect, a Claude completion hook
directory made absolute, regressed GT-HOOK-SESSION-LIFECYCLE, and SPEC-PANERM-001
deleted that hook and its source. The new row was spot-checked the same way:
the Claude Code lane of `pkg/editguard/matrix.go` set to `none` in a scratch
copy regressed exactly GT-HOOK-EDIT-GUARD-ON. So did the Antigravity lane set
to `enforced`, which puts the guard on the advisory-only `.agents/hooks.json`.

M9 first survived that spot-check: `value_contains` is a substring test, and
`./.autopus/plugins/autopus` contains `./.autopus/plugins/auto`. Values that
a longer path could contain are therefore also asserted as quoted strings with
`contains`.

## Changing the set

- A changed assertion, variant, corpus digest, or expected test changes the
  task's `expectation_digest`; the run fails with `expectation_changed` until
  `auto eval harness baseline --update` records it. Wording in `intent` and
  `outcome` is outside the digest.
- The digest also covers `oracle_mode` and `black_box_oracle` of a black-box
  task, so an edited fixture needs its new `sha256` in the definition and
  then a baseline update. A white-box task encodes neither field, so its
  digest is the SPEC-HARNEVAL-001 one; adding the five oracles changed exactly
  those five baseline rows.
- Retire a task with `status.state: retired` and a reason instead of deleting
  it; the baseline keeps the retired row.
- Editing a corpus file changes its digest: update `file_sha256` in every agent
  task that pins it, and recalibrate `expected_tests` if the oracle changed.

## Incident intake (SPEC-HARNEVAL-002)

An incident recorded in the learn store (`.autopus/learnings/pipeline.jsonl`)
becomes a golden task only through a person. Intake turns the entry into a
quarantined candidate under `candidates/`, a person writes the task's category
and assertions, and an explicit `promote` publishes it to `tasks/surface/`, or
`reject` retires it. Nothing is promoted in bulk or automatically, no
assertion is generated, and no `repro` value is ever executed. Run every
command from the repository root: `auto learn` uses the working directory, and
`auto eval harness` reads `--dir`, which defaults to `.`.

```sh
auto learn record --type fix_pattern --pattern "hook missing in codex" --files pkg/content/a.go,pkg/content/hooks.go,pkg/content/z.go --packages pkg/adapter,pkg/content --expected "codex gets .codex/hooks.json" --actual "the codex surface has no hooks file" --repro "auto init"
auto eval harness intake --all-eligible --format json
# write the task category and assertions into candidates/GTC-023e9302ff0b.json
auto eval harness promote GTC-023e9302ff0b --format json
auto eval harness baseline --update
```

1. **Record.** `--expected` and `--actual` make an entry eligible; `--repro`
   is stored as data. The store writer masks secrets in every free-text field
   with `pkg/secretscan` before any byte is written. It refuses an `expected`,
   `actual`, or `repro` with a control character, and caps `expected` and
   `actual` at 1024 bytes and `repro` at 512 bytes after masking.
2. **Intake.** `--all-eligible` takes every entry that has `expected` and
   `actual` and whose fingerprint is not already an open candidate, a promoted
   link, a rejection, or an active incident task. `--learning L-NNN[,L-MMM]`
   names entries and reports each one it skips. Entries with the same
   fingerprint (type, pattern, files, and packages, normalized) form one
   candidate, `candidates/GTC-<12 hex>.json`, whose evidence comes from the
   lowest id alone. Intake checks and masks the stored text again and skips a
   pattern that has a control character other than newline or tab or is
   longer than 4096 bytes. stdout holds one `harness_intake_result.v1`
   document. The exit code is 0, 2 when a row was skipped, and 1 when the
   invocation is refused, which writes nothing.
3. **Write the draft.** The candidate's `task` is a complete
   `harness_golden_task.v1` draft whose `category` is `""` and whose
   `assertions` is `[]`. Set `category` and write at least one assertion;
   `variants` is optional. For the example above, the `task` gets:

   ```json
   {
     "category": "hooks_settings",
     "assertions": [
       {"kind": "file_exists", "platform": "codex", "path": ".codex/hooks.json"}
     ]
   }
   ```

4. **Promote or reject.** `promote` checks the candidate in a fixed order and
   stops at the first failure without writing anything. It publishes
   `tasks/surface/GT-INC-<8 HEX>.json`, loads the whole set again and removes
   the task alone when the set no longer loads, writes the permanent link
   record `candidates/promoted/GT-INC-<8 HEX>.json`, and removes the candidate
   last, so rerunning an interrupted promotion finishes it. `current_outcome`
   is the new task's `pass` or `fail` on the pinned surface, or
   `not_evaluated` with the precondition that stopped it; the promotion holds
   either way. `auto eval harness reject GTC-<12 hex> --reason "<text>"` moves
   the candidate into `candidates/rejected/` instead, so intake never offers
   that fingerprint again. Deleting a candidate file by hand is not a
   rejection: the next `--all-eligible` creates it again.
5. **Pin.** Until `auto eval harness baseline --update` records it, the run
   reports the promoted task as `new` with `set_digest_mismatch`.

`auto learn prune --days N` keeps every entry that an open candidate, a
promoted link record, or the provenance of an incident task names, whatever
its age, and then prints `Kept K entries linked to golden-task evals.`. A
rejection protects nothing. When any of those files cannot be read, prune
stops with `eval_links_unreadable` and leaves the store unchanged. Intake,
promote, and reject refuse with `platform_unsupported` on Windows, so
golden-set upkeep is a macOS or Linux task.

Caveats:

- **Mixed-version prune.** An `auto learn prune` from a binary older than this
  flow deletes linked entries by age and drops `expected`, `actual`, and
  `repro` from the entries it keeps. Candidates and link records hold their
  own copies, so the eval evidence survives.
- **Duplicate entries.** Intake never edits an existing candidate. An entry
  recorded with the same fingerprint after its candidate exists does not join
  it: `--all-eligible` skips it, `--learning` reports `duplicate_candidate`,
  and prune does not keep it. After a secret detector change, recording the
  same incident again can mask its text differently and yield a second
  candidate; reject the duplicate.
- **Shell history.** Masking covers what these commands write and print. The
  values of `--pattern`, `--expected`, `--actual`, `--repro`, and `--reason`
  stay in your shell history as typed, so do not paste secrets into them.
