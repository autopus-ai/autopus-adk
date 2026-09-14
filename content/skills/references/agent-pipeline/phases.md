# Pipeline Phases

Read this file when you reach a phase and need its exact entry condition, work,
and exit condition. Read one phase at a time; do not preload the whole file into
a worker prompt.

Every phase below runs in the supervising session unless it names a worker role.
A worker is dispatched only when the slice is genuinely independent — separate
owned paths, no dependency edge, and enough context to finish alone — or when
the step needs an isolated context or a specialist the supervisor cannot be.
Otherwise the supervisor performs the step inline. Task size, file count, and
line count never decide this.

## Phase 0 — Preflight

1. Resolve the SPEC directory and read the SPEC requirement set.
2. Resolve the declared change class and gate applicability (`references/gates.md`).
3. Resolve quality, permission, and topology settings (`references/quality-modes.md`).
4. If the run will dispatch workers, confirm the surface-native subagent tool
   exists and initialize `subagent_dispatch_count = 0`,
   `subagent_roles_dispatched = []`, `degraded_mode = none`.

An explicit `--solo` run reports `subagent_dispatch_count: 0` and is labelled
solo, never as a degraded worker pipeline. An explicit `--team` or `--multi`
request is honored and reported as requested; if the platform cannot support the
requested topology, stop with an unsupported-mode diagnostic instead of silently
substituting another one.

### Route selection

`auto pipeline run` picks the dispatched phase set from the change contract and
prints `Pipeline route: compact|full — <reason>` on stderr; the checkpoint and
receipt carry only that set as `route_phase_set`.

- Full route dispatches `plan`, `test_scaffold`, `implement`, `validate`,
  `review`. An absent, unreadable, or escalated contract takes this route.
- Compact route dispatches `implement`, `validate`, `review` only when exactly
  one strictly parsed contract binds this SPEC and its acceptance, intended
  surface, and verification plan. Recorded risk must agree with risk recomputed
  from the declared class, surface, and new-contract flag. Actual changed paths
  must remain inside that surface and still reassess as low risk.
  Ambiguous, malformed, escalated, or undeterminable inputs keep the full route.
  Test-first work remains inside implementation; only the extra dispatches vanish.

Those five ids are the whole registry. The Phase 1.9 probe below is in-session
work bounded by the `risk_first_probe` gate decision, not a dispatched phase, so
the route never changes it. Never report a phase the route did not dispatch.

On `--continue`, the saved route must match the newly authorized route before any
dispatch. A checkpoint without a recorded route counts as full. A mismatch is a
blocking resume error, not permission to reinterpret prior completed phases.

## Phase 1 — Planning

Entry: SPEC (or compact `change.md`) resolved.

The planner produces observable work units, not a delegation quota:

- decompose observable acceptance into units that can each be verified alone;
- name exact owned paths and forbidden paths per unit;
- classify dependency edges, migration-numbering lanes, and shared interfaces;
- apply the minimality ladder before assignment: actual need → existing
  code/helper/pattern → stdlib/native → existing dependency → new dependency or
  abstraction → minimum sufficient verification;
- flag any new helper, dependency, or abstraction that has no prior evidence as
  a risk or revise-target;
- mark each unit `inline` (supervisor performs it) or `worker` (independent
  slice). A single-unit plan with one owner is an inline plan; inventing a
  planner-plus-worker scaffold for it is waste, not rigor.

Assignment table shape:

| Unit | Execution | Owned paths | Forbidden paths | Depends on |
|---|---|---|---|---|
| T1 | worker | `pkg/foo/**` | `pkg/bar/**` | — |
| T2 | inline | `docs/foo.md` | — | T1 |

Exit: every acceptance criterion maps to at least one unit.

## Phase 1.5 — Failing Test Scaffold

Entry: plan accepted, on the full route. Skipped when `--skip-scaffold` is
explicit or the declared change class is `docs_only`. The compact route has no
separate scaffold dispatch at all — the same test-first discipline below applies
inside the implementation step.

Write failing tests for each mandatory requirement before implementation. Every
generated test must fail for a plausible defect, assert observable behaviour
rather than source text, and stay deterministic. A generated test that already
passes is reported, not hidden: it means the behaviour exists already.

Implementers treat these tests as read-only specification. A test that looks
wrong is reported as a blocker, not edited by the implementer.

Exit: the scaffold is red for the right reason.

## Gate 1 — Approval

Show the assignment table and wait for approval. Skipped when `--auto` is set.

## Phase 1.8 — External Documentation

Entry: Gate 1 closed. Skip entirely when the change touches no external library
or API.

Runs in the supervising session, because documentation tools are session-scoped.

1. Detect external libraries from the SPEC, the plan, and the imports of the
   affected files. Ignore standard-library modules. Keep at most five.
2. For each, resolve the library id, then query docs for the task-relevant
   topic. On failure or empty response, fall back to a focused web search and
   label the result as a web fallback.
3. Budget the injected payload: one library ~5000 tokens, two ~3000 each, three
   ~2500 each, four or five ~2000 each, hard cap 10000 total. Trim in this
   order: API signatures > config examples > breaking changes > error patterns >
   tutorials.
4. Pass the result to later steps as a `## Reference Documentation` section and
   preserve `version`, `source_ref`, and `checked_at` for the Technology Stack
   Decision evidence a greenfield manifest needs.

Unavailable supplementary documentation may be reported without blocking unrelated
work. Greenfield version/source provenance still requires verification; do not
install an unverified new stack or silently change an existing major version.

## Phase 1.9 — Risk-First Probe Gate

Entry: Phase 1.8 complete or skipped. Applicability comes from
`gate-applicability.json`, never from a worker's judgement.

Consume the `## Risk-First Integration Probe` table in `plan.md` (1–3 rows:
`assumption_id`, `class`, `risk`, `boundary`, `input`, `oracle`, `isolation`,
`status`, `reason`, `evidence`).

1. Execute every `not-run` row that is executable now: a real boundary, an
   isolated fixture, no unapproved external effect. Record `PASS` or `FAIL` plus
   an evidence ref produced by that execution.
2. A row that stays `not-run` keeps its reason and travels into the handoff as
   an explicit limitation. It is never reported as `PASS`.
3. On `FAIL` for a high or critical assumption, revise `plan.md` once. A second
   `FAIL` surfaces to the user instead of a third attempt.
4. An implementer-introduced constraint broader than the requirement — a new
   ACL, compatibility limit, or security limit — is a scope expansion. Probe it
   here, before fan-out multiplies the assumption.

At the first row that returns `PASS` — or at the first real integration
execution when every row is honestly `not-run` — record the
`first_vertical_slice` milestone (`references/gates.md`). Recorded late, lead
time is unmeasurable.

Exit: probe gate closed. Next required step is implementation, never completion.

## Phase 2 — Implementation

Entry: probe gate closed.

Inline units are implemented by the supervisor directly. Worker units are
dispatched per `references/delegation.md`, which owns ownership rules, batching,
isolation, and the migration-numbering lane.

Every implementation prompt (inline or dispatched) carries: the resolved
requirement text, the owned and forbidden paths, the acceptance criteria it
closes, the gate decisions for this run, the `## Reference Documentation` from
Phase 1.8 when it exists, and the minimality ladder.

Stack profile injection: read the unit's assigned profile from
`.autopus/profiles/executor/{name}.md` first, then the built-in
`content/profiles/executor/{name}.md`. Resolve `extends` when the generated
profile declares it and merge the base Instructions first. No profile found is a
graceful skip, not an error.

Workers skip project-wide formatters, linters, builds, and test suites while
fan-out is active; a concurrent sibling's half-finished tree produces phantom
failures. Each verifies only its own surface.

Exit: all units integrated into one working tree.

## Gate 2 — Validation

Entry: Phase 2 integrated. See `references/verification.md` for the check set
and the threshold policy.

A FAIL routes precise correction work back to the unit's original owner —
re-using the same worker id when the worker is still revivable — then
re-validates. Do not spawn a fresh worker merely to ask a follow-up question.

## Annotation — opt-in only

@AX annotation is not a pipeline phase. It runs only when the user asks for it
or the run passes the explicit annotation opt-in, in which case
`auto spec gates ... --annotation` evaluates the `annotation` gate. Without that
opt-in the gate is `not_applicable` and nothing is tagged. When it does run, the
`ax-annotation` skill body is the rule set; a missing reference source is
`blocked` with the fallback "record modified files, defer tagging", not a loop.

## Phase 3 — Tests and Minimum Sufficient Verification

Add only the tests that close mandatory acceptance and real regression risk.
Keep the non-reducible gates intact. Threshold policy lives in
`references/verification.md`.

## Phase 3.5 — UX Verification

Runs when the change set contains UI paths and the run is not `--solo`.
Applicability for `accessibility` and `ux_verification` comes from
`auto spec gates`; this phase never self-declares `not_applicable`.

Full contract, including the `no-capture` oracle set, is in
`references/verification.md`.

## Phase 4 — Review

Correctness, security, data-loss, and acceptance findings are authoritative.
Provider fan-out is advisory evidence. The loop, its terminal states, and the
repeat-discovery rule are in `references/review.md`.

## Completion

`references/completion.md` owns the sync-readiness contract, the telemetry
summary, and the final receipt.
