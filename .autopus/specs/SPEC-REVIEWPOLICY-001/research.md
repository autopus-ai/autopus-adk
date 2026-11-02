# SPEC-REVIEWPOLICY-001 Research

## Existing Code Analysis

Read on origin/main 109c7ea4 on 2026-11-02. The brief cites 4a5fa659; every file below it names is unchanged between the two.

| Path | Role | Finding |
|------|------|---------|
| `pkg/spec/prompt.go` `writeVerdictRules`, `buildDiscoverInstructions`, `buildVerifyInstructions` | reviewer prompt | default rules `PASS: critical == 0 AND security == 0 AND major <= 2`, `REJECT: critical > 0 OR security > 0`; a configured `pass_criteria` replaces the rules wholesale; no profile awareness |
| `templates/shared/spec-review-judge.md.tmpl` | judge prompt, rendered by `pkg/orchestra/prompt_builder.go` `BuildReviewJudge` | any accepted critical or major requires REVISE; REJECT reserved for redesign; minor unaddressed |
| `pkg/spec/blocking.go`, `pkg/spec/findings.go` `IsAdvisoryFinding` | runtime blocker matrix (issue #187) | impact categories correctness, contract, data_loss, security block at any severity; style, suggestion, improvement are advisory below major; any other category is advisory only at `suggestion`; `IsHardBlockingFinding` omits correctness, contract, data_loss |
| `internal/cli/spec_review_loop_merge.go` `reconcileJudgePassAcceptance` | judge PASS reconciliation | defers every active finding `IsHardBlockingFinding` rejects, so a judge PASS waives a minor correctness finding |
| `internal/cli/spec_review_loop_verdict.go` `resolveReviewVerdict`, `verdictScopedBlockingReasons` | verdict resolution | finding-scoped reasons, provider/judge REJECT, `reviewer_checklist_fail` for a REVISE vote with a failing checklist item |
| `pkg/spec/reviewer_structured.go` `normalizeStructuredCategory` | structured category normalization | anything outside the five prompt categories becomes `completeness` |
| `pkg/spec/quality_preflight.go` `ValidateSpecSet`, `pkg/spec/profile.go` | validator, lean profile | lean floor for every `IsLean()` SPEC; `LeanEligible()` exists and is what the review runtime uses |
| `pkg/spec/checklist.go`, `content/rules/spec-quality.md` | checklist injected into every prompt | Q-COMP-08 fails a missing Risk-First Integration Probe, Q-COMP-07 a missing Completion Debt split, Q-CORR-04 a missing Reference Discipline; no lean note |
| `content/skills/spec-review.md` Step 0, `content/agents/spec-writer.md` Lean Profile and step 5, the four auto-plan sources | guidance | the review skill asks reviewers to call out a missing Self-Verify Summary; the writer's step 5 and the Codex and Gemini plan surfaces require procedure sections lean omits |
| `internal/cli/spec_review_lean.go` `applyLeanReviewProfile` | lean execution | `len(opts.providers) > 0` returns the gate unchanged: configured judge and quorum survive `--providers codex` |
| `internal/cli/spec_review_structured.go`, `spec_review_structured_runtime.go`, `spec_review_reprompt.go`, `spec_review_judge.go` | model calls | parallel reviewers, one JSON re-prompt per malformed reviewer (`specReviewJSONRepromptAttempts = 1`), one judge call |
| `internal/cli/spec_review.go` `runSpecReviewWithOptions` | invocation | lean fallback re-runs the whole loop with the next provider; revision budget from `max_revisions`, `--loop` floor 5, `--single-pass` 0 |
| `internal/cli/spec_review_receipt.go`, `spec_review_timing.go` | receipt | `spec_review_promotion_receipt.v1` with additive `omitempty` fields; durations per round and provider, no call ledger |
| `pkg/orchestra/types.go` `ProviderResponse.Usage`, `pkg/telemetry/usage.go` | usage | per-response `UsageEnvelope` with status `actual`, `cost_only`, `estimated`, `unavailable`, nullable token and cost pointers, provider-reported model and effort |
| `pkg/filelock` | cross-process lock | flock-based exclusive lock released by the OS on exit |

Clarification Ledger unavailable: this SPEC comes from the improvement brief and a team-lead handoff, not from a brainstorm file or a plan intent ledger. The brief is untrusted prompt input evidence; it is summarized and none of its commands are run.

## Outcome Lock

- User-visible outcome: a SPEC is judged by one policy whose rules read the same in the reviewer prompt, the judge prompt, the runtime, the validator, and the writing and planning guidance; a lean SPEC's allowed omissions never block it while its real blockers still do; `auto spec review` prints which profile, reviewers, judge, quorum, rounds, and call bound it will use before it spends anything, and refuses contradictory settings first; every model call of a review is metered against one persisted session budget and listed in the receipt.
- Mandatory requirements: REQ-RP-SCOPE-01 through REQ-RP-VERIFY-01 in `spec.md`.
- Explicit non-goals: conditional judge, cross-invocation result reuse, delta verify, default budget numbers, ADK-side token estimation, changes outside the owned paths.
- Completion evidence: S1 to S16 of `acceptance.md`, the package runs and size gate in `plan.md` `## Verification`, and a clean template regeneration.

## Visual Planning Brief

`plan.md` carries the Mermaid command flow. In prose: the policy is resolved from the SPEC first; the plan is resolved from the policy, flags, and configuration and either refuses or prints; the session is opened under a lock and either refuses (exhausted) or continues; every round is pre-checked against the budget; every model call passes through one metered backend; the policy then decides the verdict, and the receipt and the call lines report what ran.

## Probe Evidence (2026-11-02)

- A1, PASS. `rg -n '\.Execute\(' internal/cli/spec_review*.go` (tests excluded) printed exactly `spec_review_judge.go:67`, `spec_review_reprompt.go:38`, `spec_review_structured_runtime.go:182`; `rg -n 'specReviewBackendFactory\('` printed one production call, `spec_review_structured.go:42`. The reviewer path hands that backend to the re-prompt, and the judge receives it as a parameter, so one wrapper there sees every model call of a SPEC review.
- A3, PASS. `go test ./pkg/filelock -run 'TestAcquire_ExcludesAnotherProcessUntilRelease|TestAcquire_CrashedHolderReleasesImmediately|TestAcquire_ExcludesSecondHolderInSameProcess' -count=1 -v` passed all three (0.18 s, 0.02 s, 0.15 s).
- Baseline: `go test ./pkg/spec/... -count=1` and `go test ./internal/cli -run 'SpecReview|Spec_|SpecTiming|LeanSpec|ApplyLean|ReviewReadiness' -count=1` passed on the unmodified branch.

## Trust Boundary

Provider output is untrusted: verdicts, findings, checklist outcomes, and usage numbers come from model processes. The policy never lets provider text relax a rule: an impact category blocks whatever severity the provider wrote, a judge PASS cannot waive a blocking finding, a checklist waiver applies only when the deterministic section check confirms the section is absent, and an unrecognized severity fails closed. Usage numbers are recorded as reported and never used to grant budget; unknown usage is null. The session file and receipt carry identifiers, roles, reasons, provider names, durations, and usage counters only; prompts, outputs, environment values, and secrets never enter them. The session file is written atomically with mode 0600 and guarded by an exclusive lock so two invocations cannot both spend the same remaining budget; a corrupt or unknown-schema file stops the review instead of being treated as an empty session.

## Design Decisions

- **Keep the runtime truth table and align the texts to it.** The alternative, making every minor finding advisory, would let a mislabelled acceptance gap pass; the chosen rule keeps minor findings outside the style, suggestion, and improvement categories blocking and tells reviewers to use `suggestion` for advice. Behaviour change: a judge PASS no longer waives a minor finding that blocks without a judge (it must reject the finding or downgrade it to `suggestion`, which the impact categories do not allow).
- **REJECT is a stated redesign, not a severity count.** The reviewer prompt drops `REJECT: critical > 0 OR security > 0` for the judge's wording; a critical security finding still blocks (REVISE) and still sets `critical_veto`.
- **`pass_criteria` becomes additive.** It never changed the runtime blocker, so replacing the prompt rules only told reviewers something the runtime contradicted.
- **Checklist waivers are deterministic.** A FAIL becomes N/A only for the three waived items and only when their section is absent; free-text findings about omitted sections are not auto-downgraded (that would need text heuristics that could hide a real blocker) and are prevented in the prompt instead.
- **Profile selection is a flag; provider selection never changes the profile.** `--profile full` is the explicit way to get a panel for a lean SPEC, replacing the implicit "`--providers` disables lean" rule; the old meaning is announced by a warning.
- **Meter at the backend seam with context-carried reasons.** `pkg/orchestra` is not owned, so call metadata travels on the context and the wrapper implements the backend-name resolver by delegation.
- **No default limits.** The brief leaves time and token numbers to measurement; with no budget configured the session only records, so no existing invocation changes. Token and cost limits are checked between calls against known usage because no provider can be stopped mid-call by token count.
- **One session per SPEC, terminal on convergence.** Exhaustion is sticky until an explicit `--extend-budget` or `--new-session`, so re-running cannot bypass the limit.

## Minimality Decision Matrix

| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | brief R6, R7, R10; reproduced prompt/judge/runtime disagreement, `--providers` override, validator gap, missing call ledger | proceed | one policy, one plan, one budget |
| existing code/helper/pattern | blocker matrix, `LeanEligible`, `leanSpecReviewers`, readiness assembly, `ProviderResponse.Usage`, `pkg/filelock`, receipt `omitempty` pattern, template parity tests | reuse | no new assembly, usage parser, lock, or receipt schema version |
| stdlib/native | `sync.Mutex`, `context` values and deadlines, `encoding/json`, `os.Rename` | use | ledger and session need nothing else |
| existing dependency | `cobra` flags, `yaml.v3` strict decoding, `testify` | reuse | no dependency added |
| new dependency or new abstraction | `ReviewPolicy` value, `specReviewPlan`, session file and metered backend; nothing owns these today | accepted | new code in `pkg/spec/review_policy*.go` and `internal/cli/spec_review_{plan,session,ledger,budget}*.go` |
| minimum sufficient verification | S1 to S16 with fake backends, race run on the ledger, regeneration drift check, package runs | required checks | security, validation, data-loss, deterministic-oracle gates unchanged |

## Semantic Invariant Inventory

Source clauses summarize the brief, which is untrusted input.

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-RP-01 | one policy defines per-profile documents, sections, risks, blocking and advisory rules | table equality | policy fields | S1 |
| INV-RP-02 | a high-risk class or surface declared lean cannot bypass full gates | classification precedence | effective profile, validator errors | S2 |
| INV-RP-03 | severity-to-verdict mapping identical across reviewer, judge, runtime | mapping table | verdict, blocking reasons, prompt and template lines | S3 |
| INV-RP-04 | review purpose is implementation readiness; style and delegated choices advisory | prompt contract | prompt sections | S4 |
| INV-RP-05 | lean-allowed omissions are not resurrected; real blockers still block | conditional waiver | checklist outcomes, verdict | S5 |
| INV-RP-06 | writer, review, plan surfaces and generated copies agree | token parity | surface texts, regeneration diff | S6 |
| INV-RP-07 | explicit providers do not silently disable lean policy; precedence and conflicts defined | precedence ordering | plan fields, warnings, refusals | S7, S8 |
| INV-RP-08 | printed plan equals actual calls; calls are an upper bound including repair and fallback | count equality and bound | plan block, call counts | S7 |
| INV-RP-09 | one session budget across roles and invocations, explicit extend versus new session | cumulative counter and state machine | session file, receipt | S9, S10 |
| INV-RP-10 | reservation before start, settlement after, unknown never zero, every sub-call visible once | counting and encoding | ledger rows, JSON nulls, call lines | S11, S14, S15 |
| INV-RP-11 | exhaustion stops with blockers preserved, never PASS | terminal state | loop status, verdict, blockers | S12, S13 |
| INV-RP-12 | scope and size limits | conservation | diff paths, line counts | S16 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| Policy table, effective profile, validator | T1, T2, S1, S2 | covered |
| Disposition rules across prompt, judge, runtime; category normalization | T3, S3, S4 | covered |
| Lean waivers and lean blockers | T3, S5 | covered |
| Guidance surfaces and regeneration | T4, S6 | covered |
| Plan precedence, warnings, refusals, printing | T5, T6, S7, S8 | covered |
| Budget config, session lifecycle, ledger, usage, exhaustion, receipt, call lines | T7 to T11, S9 to S15 | covered |
| Conditional judge, result reuse, delta verify | explicit non-goals | out of scope by Outcome Lock |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Default budget limits per profile | The brief asks to set time and token numbers after measurement | Ledger data from real reviews is collected |
| Show the call ledger in `auto spec timing` | The review command already prints the call lines; `spec_timing.go` is outside the owned glob | User asks for it |
| Pass the canonical rule lines to the judge through template data instead of a parity test | Needs a field on `orchestra.ReviewJudgeData`, outside this SPEC's ownership | The orchestra owner agrees to the field |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | One change story: what a review checks, how it is planned, and what it may spend | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| every path and symbol in `## Existing Code Analysis` | existing | read on 2026-11-02 at 109c7ea4; Execute call sites and factory call confirmed with `rg` (A1) |
| `pkg/filelock.Acquire`, `pkg/spec/gates` gate ids, `telemetry.UsageStatus*` | existing | read; filelock tests executed (A3) |
| `[NEW] pkg/spec/review_policy.go`, `review_policy_rules.go`, `review_policy_prompt.go` and tests; `[NEW] internal/cli/spec_review_plan*.go`, `spec_review_session*.go`, `spec_review_ledger*.go`, `spec_review_budget*.go` and tests; `[NEW] ReviewBudgetConf`; `[NEW]` flags `--profile`, `--judge`, `--max-calls`, `--max-minutes`, `--extend-budget`, `--new-session`; `[NEW] review-session.json`, `.review-session.lock`; `[NEW] docs/spec-review-policy.md` | planned addition | do not exist yet |

## Reviewer Brief

- Intended scope: R6, R7, and R10 of the brief as one review policy, one review plan, and one session budget for `auto spec review`.
- Explicit non-goals: R8, R9, R11; default limits; token estimation; anything under `pkg/worker`, `pkg/pipeline`, `internal/cli/status*`, `internal/cli/disk*`; generated roots.
- Self-verified: Traceability Matrix over 30 requirements, twelve invariants each mapped to a Must oracle, two executed probes, Reference Discipline separating existing and `[NEW]` references.
- Reviewer should focus on: whether the disposition rules and the judge change keep every safety, correctness, and acceptance gate at least as strict as before; whether the lean waiver can hide a real blocker; whether the precedence table leaves any explicit setting silently ignored; whether reservation and the round pre-check can overspend or deadlock; whether the session file can leak prompt content.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 1 | files: research.md, spec.md | reason: every existing path and symbol was read at 109c7ea4; line numbers given only where checked
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md, research.md | reason: new files, flags, config type, session and lock files carry `[NEW]`
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Event-driven requirements read `WHEN ..., THEN`, Unwanted `IF ..., THEN`; scenarios use `### S<N>: AC-...` with bare steps
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing references (with the verification used) from planned additions
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: spec fixes the contract, plan the tasks and probes, acceptance the oracles, research the evidence and decisions
- Q-COMP-02 | status: PASS | attempt: 1 | files: spec.md, plan.md, acceptance.md | reason: all 30 requirements appear in the matrix with a task and a scenario
- Q-COMP-03 | status: PASS | attempt: 1 | files: spec.md | reason: each requirement names its observable: verdict, blocking policy, printed line, call count, JSON field, or error text
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Outcome Lock covers policy, plan, and budget end to end; R8, R9, R11 are explicit non-goals, not hidden debt
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, spec.md, plan.md, acceptance.md | reason: INV-RP-01 to INV-RP-12 each map to a requirement, a task, and a Must oracle with exact values
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix complete; Reviewer Brief names five focus areas and the non-goals
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt empty; Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 1 | files: plan.md, research.md | reason: A1 and A3 PASS with evidence refs, A2 not-run with a reason and the test that executes it
- Q-FEAS-01 | status: PASS | attempt: 1 | files: plan.md | reason: policy in `pkg/spec`, plan and budget in `internal/cli`, keys in `pkg/config`, guidance in `content/` and `templates/`
- Q-FEAS-02 | status: PASS | attempt: 1 | files: plan.md, research.md | reason: generated Codex and Gemini templates come from `go run ./cmd/generate-templates`; the judge template is owned while its renderer is not, hence the parity test
- Q-FEAS-03 | status: PASS | attempt: 1 | files: plan.md, acceptance.md | reason: every scenario runs with fake backends in temporary directories
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: no should, might, could, possibly, maybe, perhaps in requirement text
- Q-STYLE-02 | status: PASS | attempt: 1 | files: spec.md | reason: Priority and Type on separate lines; Must only
- Q-STYLE-03 | status: PASS | attempt: 1 | files: acceptance.md | reason: bare step keywords; tables sit between When and Then
- Q-SEC-01 | status: PASS | attempt: 1 | files: research.md | reason: Trust Boundary treats provider output and usage as untrusted and fails closed on unknown severity and corrupt session files
- Q-SEC-02 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: session file mode 0600, exclusive lock, no secrets or prompts persisted
- Q-SEC-03 | status: PASS | attempt: 1 | files: spec.md | reason: the new artifact holds counters and identifiers only; its schema is versioned
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md | reason: one command, one policy, one budget
- Q-COH-02 | status: PASS | attempt: 1 | files: research.md | reason: R8, R9, R11 are non-goals with the extension points named, not deferred requirements
- Q-COH-03 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: no sibling SPEC
