# Harness golden set (SPEC-HARNEVAL-001)

This directory is the committed golden set that `auto eval harness` evaluates.
The deterministic PR lane generates the five platform surfaces in-process with
every input pinned, evaluates the surface tasks, and compares them with
`baseline.json`. Agent tasks are never run by the PR lane; it checks their
schema and corpus digest only. The maintainer live lane (T10-T13) runs them.

| Path | Content |
|------|---------|
| `manifest.json` | `harness_golden_set.v1`: active paths, floors, live policy, pins |
| `baseline.json` | `harness_eval_baseline.v1`, written by `auto eval harness baseline --init` on this tree |
| `fixtures/codex-models.json` | the pinned Codex model catalog (`pins.codex_model_catalog`) |
| `tasks/surface/*.json` | 31 active surface tasks |
| `tasks/agent/*.json` | 12 active agent tasks, one per benchmark corpus task |
| `candidates/` | reserved for SPEC-HARNEVAL-002 quarantine; never loaded |

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
| active surface tasks | 20 | 31 |
| assertion platforms | 5 | 5 |
| surface categories | 4 | 5 |
| multi-platform or multi-path ratio | 0.60 | 30/31 = 0.97 |
| active agent tasks | 12 | 12 |

Surface tasks by category:

- `routing`: GT-ROUTE-CLAUDE-DETAILS, GT-ROUTE-ANTIGRAVITY-DETAILS, GT-ROUTE-CODEX-SKILL-NAMES,
  GT-ROUTE-OMP-EXACT-MAP, GT-ROUTE-OPENCODE-DETAILS, GT-ROUTE-ALIASES, GT-ROUTE-ANTIGRAVITY-COMMANDS
- `hooks_settings`: GT-HOOK-ARCH-GATE-ON, GT-HOOK-ARCH-GATE-OPT-OUT (variant `hooks.pre_commit_arch=false`),
  GT-HOOK-SESSION-LIFECYCLE, GT-HOOK-CLAUDE-RULE-DISPATCH, GT-SETTINGS-MCP-SERVERS,
  GT-SETTINGS-PERMISSION-SCOPE, GT-HOOK-OPENCODE-PLUGIN, GT-HOOK-LORE-COMMIT-MSG
- `prompt_contract`: GT-PROMPT-TRIAGE-PARITY, GT-PROMPT-THIN-ROUTER, GT-PROMPT-WORKER-RECEIPT,
  GT-PROMPT-DELEGATION-RULE, GT-PROMPT-SHELL-PORTABILITY, GT-PROMPT-REVIEW-CONVERGENCE,
  GT-PROMPT-PROJECT-IDENTITY
- `agent_skill_exposure`: GT-AGENT-READONLY-REVIEW, GT-AGENT-PIPELINE-ROLES, GT-SKILL-CORE-CATALOG,
  GT-SKILL-CLAUDE-NATIVE-ORCHESTRATION, GT-SKILL-CODEX-PLUGIN-ENTRY
- `generated_root_hygiene`: GT-HYGIENE-MANAGED-BLOCKS, GT-HYGIENE-SHARED-AGENTS-ROOT,
  GT-HYGIENE-REFERENCE-INTEGRITY, GT-HYGIENE-GENERATED-SURFACE-SAFETY

Each task's `provenance.ref` names the canonical source the behavior comes
from. A task must assert a cross-cutting behavior of the generated surface; a
task that restates one existing contract test assertion is rejected in review.
`file_absent` reads ownership: a path counts as absent on a platform when that
platform's adapter did not generate it, even if another platform wrote it.

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
REQ-HE-08 table. Calibration is judged first: without a passed `before`, any
record is `records_protocol_mismatch`; with one, the records must hold every
`order` attempt of the session exactly once. A mismatch writes no report.

| Verdict (precedence) | Reason | When |
|----------------------|--------|------|
| `vacuous` | `oracle_calibration_failed` | `calibration.json` absent, or `before` or `after` not `passed` |
| `vacuous` | `oracle_not_run` | an arm has no record with `oracle.ran` true (build failures only) |
| `vacuous` | `agent_all_failed` | no trial of either arm got past the agent step (`agent_launch_failed`, `agent_exit_nonzero`, `agent_timeout`, `observation_failed` or error only), e.g. missing credentials; one arm alone failing at the agent step is still judged |
| `incomplete` | `completeness_below_floor` | completeness < `policy.completeness_floor` |
| `incomplete` | `no_valid_trial` | an arm has no non-error trial |
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
| M13 | Claude hook directory made absolute (`pkg/content/hooks_completion.go`) | GT-HOOK-SESSION-LIFECYCLE |
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

M9 first survived that spot-check: `value_contains` is a substring test, and
`./.autopus/plugins/autopus` contains `./.autopus/plugins/auto`. Values that
a longer path could contain are therefore also asserted as quoted strings with
`contains`.

## Changing the set

- A changed assertion, variant, corpus digest, or expected test changes the
  task's `expectation_digest`; the run fails with `expectation_changed` until
  `auto eval harness baseline --update` records it. Wording in `intent` and
  `outcome` is outside the digest.
- Retire a task with `status.state: retired` and a reason instead of deleting
  it; the baseline keeps the retired row.
- Editing a corpus file changes its digest: update `file_sha256` in every agent
  task that pins it, and recalibrate `expected_tests` if the oracle changed.
