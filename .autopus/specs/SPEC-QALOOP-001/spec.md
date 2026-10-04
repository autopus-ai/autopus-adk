# SPEC-QALOOP-001: Intent-anchored autonomous QA loop

**Status**: completed
**Created**: 2026-10-03
**Domain**: QALOOP
**Module**: autopus-adk
Builds on: SPEC-QAMESH-001 (feedback bundle), the `qamesh.scenario.v1` compiler.

## Problem

QAMESH executes and gates deterministically, but every scenario that carries
meaning is hand-written and every failure ends in a prompt file that nothing
reads (see `research.md`). The ask is an unattended chain:

    intent → test scenarios + user scenarios → execute → triage → fix → re-verify

Toss's FE platform team tried the naive version — derive scenarios and E2E from
code — and abandoned it: code holds too much, and the intent of the people who
specified the product is not in it. What worked was (a) authoring from human
intent, including conversational recording on a real device, and (b) a replay
bundle that lets an agent repair a broken test on its own.

This SPEC automates everything except the *oracle*. Expected values come only
from intent: SPEC acceptance criteria, a human-made recording, or an explicitly
labelled baseline. The harness may generate, heal, and repair freely around
that, and it refuses any change that would quietly move an oracle.

## Principles

- P-1 Intent owns the oracle. A generated or healed test may change *how* it
  reaches an element; it may never change *what it expects* unless the intent
  source changed.
- P-2 Agents propose, the harness disposes. Every agent output is parsed,
  validated, and diff-checked by deterministic Go before it takes effect.
- P-3 Fail closed and say why. Every stop carries a reason code and next step.
- P-4 Never touch the user's working state. The loop works on its own branch
  and removes only files it created.

## Non-goals

- AI pass/fail authority. `pass_fail_authority: ai` stays forbidden.
- Native mobile or desktop recording. Recording and discovery target web
  surfaces driven by Playwright.
- Hosted device farms.
- Generating unit-test *code* from acceptance criteria; the `tester` agent in
  `/auto go` keeps that role. This SPEC generates scenario and check *specs*.

## Requirement Index

| ID | Requirement |
|----|-------------|
| REQ-1 | WHEN an acceptance.md is parsed, THE SYSTEM SHALL yield criteria with id, title, given, when, then, and line, and SHALL report a criterion without THEN as missing_then |
| REQ-2 | WHEN a scenario declares qamesh.scenario.v2, THE SYSTEM SHALL validate intent_source, spec, recording_ref, action steps, and per-step ac, and SHALL keep v1 behaviour unchanged |
| REQ-3 | WHEN a v2 scenario contains an action step, THE SYSTEM SHALL compile it with the @journey tag and never @explore |
| REQ-4 | WHEN a spec is compiled, THE SYSTEM SHALL write a step map sidecar mapping each emitted step line to screen, index, kind, and ac |
| REQ-5 | WHEN test-scenarios files exist, THE SYSTEM SHALL compile allowlisted command cases into candidates and defer the rest with a reason code |
| REQ-6 | WHEN scenario generation runs, THE SYSTEM SHALL validate agent output against the parsed criteria before writing candidates and SHALL report per-criterion coverage |
| REQ-7 | WHEN a candidate is promoted, THE SYSTEM SHALL require validation, existing acceptance refs, and confirmed agent assertions, and SHALL never overwrite a differing active file |
| REQ-8 | WHEN an agent is invoked headless, THE SYSTEM SHALL use the per-target argv table or the AUTOPUS_QA_AGENT_ARGV override, and SHALL report a missing binary as a setup gap |
| REQ-9 | WHEN a journey fails, THE SYSTEM SHALL assign exactly one triage class from deterministic evidence |
| REQ-10 | WHEN auto qa loop runs, THE SYSTEM SHALL iterate run, re-run, triage, fix, guard, and commit on a dedicated branch within the iteration budget |
| REQ-11 | WHEN an agent iteration changes a path outside its class allowlist or moves an oracle, THE SYSTEM SHALL reject and revert only that iteration's changes |
| REQ-12 | WHEN the loop stops, THE SYSTEM SHALL record the stop reason and every iteration in report.json and report.md and SHALL restore the original branch |
| REQ-13 | WHEN a repair prompt is written for a failed GUI journey, THE SYSTEM SHALL include a replay section citing the failing step and capture artifact paths |
| REQ-14 | WHEN a codegen file is imported, THE SYSTEM SHALL convert supported lines into a v2 recording scenario and SHALL fail on unsupported lines unless partial import is allowed |
| REQ-15 | WHEN auto qa record runs, THE SYSTEM SHALL launch playwright codegen against an allowed origin and import the result |
| REQ-16 | WHEN a recording JSONL contains an agent assertion without ac, THE SYSTEM SHALL mark it confirm required |
| REQ-17 | WHEN discovery runs, THE SYSTEM SHALL crawl same-origin pages read-only and emit a baseline candidate scenario |
| REQ-18 | WHEN auto qa init runs on a Playwright project, THE SYSTEM SHALL emit a gui-journey Journey Pack example |
| REQ-20 | WHEN auto qa go runs, THE SYSTEM SHALL chain generation, a confirmation unless --auto, promotion, compilation, and the loop, committing the generated QA files as the loop branch's first commit |
| REQ-19 | THE SYSTEM SHALL ship a qa-autopilot skill documenting the unattended chain and the agent recording protocol |

## Requirements (EARS)

### P2 — Intent to scenarios

**REQ-1 Acceptance parsing.** The harness SHALL parse a SPEC's
`acceptance.md` into criteria `{id, title, given, when, then, line}`. A
criterion is recognised from:
- a `##`–`####` heading or list item carrying an id token matching
  `AC-[A-Za-z0-9][A-Za-z0-9_-]*`, whose id is that token, or
- the Autopus scenario heading `### S<n>: <title>`, whose id is `S<n>` unless
  the title carries an `AC-` token, in which case that token is the id. GIVEN/WHEN/THEN/AND lines are matched
case-insensitively, with or without bold markers. The parser SHALL report
criteria without a THEN clause as `missing_then` instead of dropping them.

**REQ-2 Scenario schema v2.** The harness SHALL accept `qamesh.scenario.v2`
next to v1, with v1 behaviour unchanged. v2 adds:
- `intent_source`: one of `acceptance | recording | baseline` (required).
- `spec`: SPEC id. Required when `intent_source` is `acceptance`.
- `recording_ref`: required when `intent_source` is `recording`.
- Action steps: `click`, `fill`, `press`, `check`, `select`, `wait_url`.
  An action target is exactly one of `{role,name,exact}`, `label`,
  `placeholder`, `text`, or `test_id`. `fill` takes `value` or `value_env`,
  never both. `value_env` compiles to `process.env[NAME]` so credentials never
  live in YAML. `press` takes `key` plus an optional target.
- Per-step `ac`: the acceptance id the step proves. When `intent_source` is
  `acceptance`, every expect step SHALL carry an `ac` listed in
  `acceptance_refs`.
- A `baseline` scenario SHALL NOT contain action steps.

**REQ-3 Lane routing by mutation.** A v2 scenario with any action step SHALL
compile with tag `@journey` and never `@explore`. A v2 scenario without action
steps SHALL compile with `@explore`, so the read-only guard keeps its
guarantee. Baseline scenarios additionally carry `@baseline`.

**REQ-4 Step map.** For every compiled spec, the compiler SHALL write a sidecar
`<id>.spec.map.json` (`qamesh.stepmap.v1`) that maps each emitted step line to
`{screen, index, kind: action|expect|goto, ac}`. Triage depends on it.

**REQ-5 Test scenarios.** The harness SHALL accept
`.autopus/qa/test-scenarios/<SPEC-ID>.yaml` (`qamesh.test-scenarios.v1`). Each
case has `{id, ac, kind: happy|negative|edge, title, given, when, then,
automation}`, where `automation.type` is `gui`, `command`, or `manual`:
- `gui` names a user scenario.
- `command` carries a check `{adapter, argv, cwd, timeout}`.
- `manual` carries a `reason`.

The candidate compiler SHALL read command checks as candidates with source
`test-scenarios`. `argv[0]` SHALL be one of `go`, `npm`, `npx`, `pnpm`, `yarn`,
`bun`, `pytest`, `python`, `python3`, `uv`, `cargo`, `make`, `deno`, or `node`;
any other value becomes a deferred candidate with
`qa_test_scenario_command_not_allowlisted`.

**REQ-6 Generation.** `auto qa scenario generate --spec <ID> --agent <target>`
SHALL:
1. Build a bounded prompt from the parsed criteria, the v2 and
   test-scenario schemas, and the ids of existing scenarios.
2. Run the agent headless in generate mode, which must not write files.
3. Extract fenced YAML documents from the agent's stdout.
4. Validate them.
5. Write the valid ones to `.autopus/qa/scenarios/candidates/` and
   `.autopus/qa/test-scenarios/candidates/`.

Before writing, every step `ac` and every case `ac` SHALL exist in the parsed
criteria. The command SHALL print and return per-criterion coverage:
`covered_by_case`, `covered_by_user_scenario`, or `uncovered`.

**REQ-7 Promotion.** `auto qa scenario promote <id>|--all` SHALL move a
candidate into the active directory only if:
- it validates,
- its acceptance refs still exist, and
- it is not a recording whose agent-authored assertions are unconfirmed (see
  REQ-12).

Promotion SHALL never overwrite an existing active file with different content.

### P1 — Self-healing loop

**REQ-8 Headless agent runner.** A runner SHALL invoke the target CLI with the
prompt on stdin or as the final argument, a working directory, and a timeout:

| Target | Generate mode | Edit mode |
|---|---|---|
| claude | `claude -p --output-format text` | `claude -p --permission-mode acceptEdits` |
| codex | `codex exec --skip-git-repo-check --sandbox read-only -o <file> -` | `--sandbox workspace-write` |
| gemini | `agy -p=<prompt>` | `agy --mode accept-edits -p=<prompt>` |
| opencode | `opencode run -- <prompt>` | `opencode run -- <prompt>` |

agy's `-p` takes the prompt as its value, so it is attached with `=` and
`--mode` goes first; `agy -p --mode accept-edits` reads `--mode` as the
prompt. `--` keeps a prompt that starts with a dash from being read as an
opencode flag. A prompt passed as an argument is capped at 120 KiB
(`qa_agent_prompt_too_large`), below Linux's single-argument limit.

`AUTOPUS_QA_AGENT_ARGV` (a JSON array) SHALL override the table; the prompt
then goes on stdin. A missing binary is a setup gap (`qa_agent_cli_missing`),
not a failure.

**REQ-9 Triage.** For each failed journey the harness SHALL assign exactly one
class from deterministic evidence:
- `environment`: the adapter is blocked or skipped with a setup gap, or the
  output matches connection refused, a missing browser executable, command
  not found, or a webServer timeout.
- `test_defect`: syntax, type, or module errors located in a test file, or a
  Playwright strict-mode violation.
- `test_drift`: the failing line maps (REQ-4) to an `action` step.
- `product_defect`: the failing line maps to an `expect` step, or a non-GUI
  check fails its oracle.
- `flaky`: a failed journey passes on one immediate re-run.
- `unknown`: anything else.

Each verdict carries the matched signal.

**REQ-10 Loop.** `auto qa loop --lane <lane> --agent <target>` SHALL:
1. Require a git repo with a clean tracked tree.
2. Create branch `autopus/qa-loop/<run-id>` from HEAD.
3. Iterate at most `--max-iterations` times (default 3) through run, re-run
   failures once, triage, fix, guard, then commit or revert.

Each class has a fixed action:
- `environment` and `unknown`: stop and report; do not fix.
- `flaky`: report as quarantined; it does not fail the loop.
- `test_drift`: run the healer, which may edit only `.autopus/qa/scenarios/**`
  action steps, then recompile.
- `test_defect`: run the test fixer, which may edit only test paths.
- `product_defect`: run the product fixer, which may not edit tests,
  scenarios, test-scenarios, `.autopus/specs/**`, `.autopus/qa/**`, or
  Playwright config.

**REQ-11 Diff guard.** After each agent run, the harness SHALL compute the
changed paths (tracked and newly created). It SHALL reject the iteration when:
- a path is outside the class allowlist, or
- for a heal, any expect step, `ac`, `acceptance_refs`, `intent_source`, or
  `spec` differs between the before and after scenario.

A rejected iteration SHALL be reverted with `git checkout` on the tracked
paths it changed plus removal of the untracked paths it created. Nothing else
is touched, and the loop stops with `qa_loop_guard_rejected`. An accepted
iteration is committed as `fix(qa): <class> <journeys>`. A rejected commit
hook stops the loop with `qa_loop_commit_rejected`.

**REQ-12 Stop conditions and report.** The loop SHALL stop on any of:
- `passed` or `passed_with_flaky`
- `max_iterations`
- `no_progress`: the failure fingerprint (journey, class, step) set is
  unchanged after a fix
- `blocked_environment`
- `guard_rejected`
- `agent_failed`

It SHALL write `.autopus/qa/loop/<run-id>/report.json` and `report.md` with
every iteration's triage, agent, changed paths, guard verdict, and commit. It
SHALL finish by checking out the original branch and naming the loop branch.

**REQ-13 Replay bundle.** The repair prompt for a failed GUI journey SHALL
include a replay section:
- the failing step from the step map,
- the bounded failure excerpt,
- references to the capture trace, screenshot, console, and network evidence
  that already exist under the run directory.

Raw artifacts stay local; the prompt cites paths only.

### P3 — Recording and discovery

**REQ-14 Codegen import.** `auto qa record import --from <file.js>` SHALL
convert Playwright codegen JavaScript into a v2 `recording` scenario:
- `goto` lines become screens.
- `getByRole`, `getByLabel`, `getByPlaceholder`, `getByText`, and
  `getByTestId` with `.click/.fill/.press/.check/.selectOption` become action
  steps.
- `expect(...).toBeVisible()`, `toHaveText`, `toHaveURL`, and `toHaveTitle`
  become expect steps.

Lines it cannot convert SHALL be listed with line numbers. Import SHALL fail
unless `--allow-partial` is passed.

**REQ-15 Live recording.** `auto qa record --origin <url>` SHALL run
`playwright codegen` against an origin allowed by a Journey Pack (or
`--origin`). When the window closes, it SHALL import the output as in REQ-14.

**REQ-16 Agent recording log.** `auto qa record import --from <file.jsonl>`
SHALL accept `qamesh.recording.v1` lines
`{action|expect, target, value, key, url, by: human|agent, ac?}`, so that an
agent driving a browser can log what it did. An expect step with `by: agent`
and no `ac` SHALL be marked `confirm: required`. Promotion refuses such a
scenario until `auto qa scenario promote --accept-agent-assertions` is given
or the step gains an `ac`.

**REQ-17 Discovery.** `auto qa discover --origin <url> [--max-pages N]` SHALL:
1. Run a harness-generated, read-only Playwright crawler that only calls
   `goto` on same-origin links, up to N pages (default 20).
2. Record each page's title, `h1`/`h2` headings, and landmark roles.
3. Emit one `baseline` v2 candidate scenario with one screen per page.

The origin SHALL be allowed by a Journey Pack unless `--origin` is explicit.
Baseline scenarios are labelled as regression baselines, not correctness
proofs, in their header comment and in the command output.

### Integration

**REQ-18** `auto qa init` SHALL emit a `gui-journey` Journey Pack example
running `--grep @journey` in lane `browser-staging`.

**REQ-19** A `qa-autopilot` skill SHALL document the chain
`generate → promote → compile → loop` for unattended use and the agent
recording protocol for P3. The agent-pipeline verification reference SHALL
point `/auto go` at `auto qa loop` for QA failures.

**REQ-20 One command.** `auto qa go [SPEC-ID]` SHALL chain REQ-6, REQ-7,
compilation, and REQ-10.
- After generation it SHALL print per-criterion coverage. Unless `--auto` (or
  JSON output) is set, it SHALL ask before promoting. No answer stops the run
  with `qa_go_declined` and leaves the candidates in place.
- `--agent` SHALL default to the first installed CLI in the order claude,
  codex, agy, opencode. `--lane` SHALL default to `browser-staging` when a
  Journey Pack declares it, else the first declared lane, else `fast`.
- The active scenarios, test scenarios, and compiled specs SHALL be committed
  as the loop branch's first commit. Changes under them SHALL NOT count as a
  dirty tree, and that branch SHALL be kept even when no fix was needed.
- Without a SPEC-ID, generation and promotion SHALL be skipped.
