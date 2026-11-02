# SPEC-REVIEWPOLICY-001: One SPEC Review Policy, an Explicit Review Plan, and a Shared Review Budget

**Status**: draft
**Created**: 2026-11-02
**Domain**: REVIEWPOLICY
**Priority**: HIGH
**Change Class**: feature (new review flags, a new persisted session file, new optional config keys)
**Source**: requirements R6, R7, and R10 of the ADK improvement brief dated 2026-11-01 (baseline 4a5fa659), re-verified on origin/main 109c7ea4
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `.autopus/specs/SPEC-REVIEWPOLICY-001/**`

## Purpose

`auto spec review` decides whether a SPEC may enter implementation, but the rules it applies live in five places that disagree. The reviewer prompt tells a reviewer that `major <= 2` is a PASS while the judge template and the runtime blocker treat any open major as blocking. A judge PASS waives an accepted minor correctness finding that blocks the same review when no judge runs. The lean authoring profile lets a writer omit the Self-Verify Summary, the Risk-First Integration Probe, and other procedure sections, yet the injected quality checklist, the spec-review skill, and the auto-plan surfaces still ask for them, so a lean SPEC is either inflated or blocked by its own allowed omissions. The validator lets a high-risk class declared `profile: lean` skip the full section checks it should face.

Execution has the same problem. An explicit `--providers` silently switches the whole lean execution policy off, so `--providers codex` still runs the configured judge. Nothing prints, before the first model call, which profile, reviewers, judge, quorum, rounds, and call bound the run will use. Limits are scattered (revision count, single-pass, provider timeouts) and none of them is a budget: reviewer calls, the judge, JSON repair re-prompts, lean provider fallback, verify rounds, and a second CLI invocation each spend independently, and the receipt records wall-clock time but not which calls ran, why, on which model, or with what usage.

This SPEC puts the profile, section, gate, and finding-disposition rules in one Go policy that the validator, the reviewer prompt, the runtime blocker, the judge template, and the writer and plan guidance consume; separates review-profile selection from provider selection behind a printed review plan; and meters every model call of a review against one persisted session budget whose ledger lands in the receipt.

## Background

All behaviour below was re-read on origin/main 109c7ea4 on 2026-11-02 (`research.md` `## Existing Code Analysis`).

- `pkg/spec/prompt.go` `writeVerdictRules` writes `PASS: critical == 0 AND security == 0 AND major <= 2` and `REJECT: critical > 0 OR security > 0`; `templates/shared/spec-review-judge.md.tmpl` says any accepted critical or major finding requires REVISE and reserves REJECT for a fundamental redesign; `pkg/spec/blocking.go` blocks every open finding that is not advisory, where advisory is a suggestion severity outside the impact categories or a style, suggestion, or improvement category below major.
- `internal/cli/spec_review_loop_merge.go` `reconcileJudgePassAcceptance` defers every accepted finding that `IsHardBlockingFinding` does not cover after a judge PASS; that predicate omits the correctness, contract, and data_loss categories, so a judge PASS waives a minor correctness finding the no-judge path blocks.
- `pkg/spec/reviewer_structured.go` `normalizeStructuredCategory` turns `contract` and `data_loss` into `completeness`, so a structured reviewer's suggestion-severity data-loss finding is advisory while the same finding from the text parser blocks.
- `pkg/spec/quality_preflight.go` `ValidateSpecSet` runs only the lean section floor for every `profile: lean` SPEC, eligible or not.
- `internal/cli/spec_review_lean.go` `applyLeanReviewProfile` returns the gate unchanged when `--providers` is set.
- The structured runner calls `backend.Execute` for each reviewer, for one JSON re-prompt per malformed reviewer (`spec_review_reprompt.go`), and for the judge; `runSpecReviewWithOptions` re-runs the whole loop with the next provider when a lean reviewer is unavailable. `review-receipt.json` carries round and provider durations only.

## Outcome Boundary

- Outcome Lock: one Go policy defines, per profile `compact`, `lean`, and `full`, the required documents and sections, the sections a profile may omit, the checklist items an omitted section makes not applicable, the mandatory gates, and the review execution shape; a high-risk or otherwise ineligible lean declaration gets the full policy everywhere; the reviewer prompt, the judge template, and the runtime classify the same finding set to the same verdict, a judge PASS never waives a blocking finding, and lean-allowed omissions are never raised as blocking; `auto spec review` resolves the review profile separately from the reviewer and judge selection with a documented precedence, warns when an explicit setting no longer means what it used to, rejects an invalid combination before any model call, and prints the effective plan (profile, risk class, reviewers, fallbacks, judge and why, quorum, rounds, expected and upper-bound calls, session budget) before running; every model call of a review reserves from and settles against one session budget persisted next to `review-receipt.json`, shared by reviewers, the judge, JSON repair, lean fallback, every round, and later invocations, and the receipt lists each call with its identifiers, role, reason, provider, model and effort or unknown, usage or unknown, duration, and end reason.
- Mandatory requirements: REQ-RP-SCOPE-01 through REQ-RP-VERIFY-01 below.
- Explicit non-goals: the conditional judge, result reuse across invocations, and delta verify (brief R8, R11, R9; this SPEC only leaves call roles, reasons, and a judge mode slot for them); default budget values (left unset until measured); token or cost estimation by ADK; changes under `pkg/worker`, `pkg/pipeline`, `internal/cli/status*`, `internal/cli/disk*`; edits under any generated root; real model calls in tests.
- Completion evidence: acceptance S1 through S16 pass with fake backends, the full `pkg/spec`, `pkg/config`, `templates`, `pkg/content`, `pkg/harneval`, and `internal/cli` packages pass, `go run ./cmd/source-lines --max 300 --ext .go` passes over every changed Go file, regenerated templates leave no drift, and `auto spec validate .autopus/specs/SPEC-REVIEWPOLICY-001` reports no error.

## Definitions

- Profile: `compact` (a `change.md` contract against an existing SPEC), `lean` (the reduced four-document set of a new low-risk SPEC), `full` (the complete set). Effective profile of a SPEC: `lean` when `SpecDocument.LeanEligible()` holds, otherwise `full`.
- Blocking finding: an open or regressed finding whose category is correctness, contract, data_loss, or security at any severity, or that came through the critical/security escape hatch, or whose severity is not `suggestion` outside the style, suggestion, and improvement categories, or whose severity is critical or major inside those categories. Every other finding is advisory.
- Verdict-scoped blocker: a provider REJECT, a judge REJECT, a failing checklist item of a REVISE vote, no usable review, or an exhausted budget.
- Model call: one `ExecutionBackend.Execute` issued by the SPEC review for a reviewer, a JSON repair, or the judge. Readiness probes and capability probes are not model calls.
- Review session: the logical review of one SPEC, persisted at `{SPEC_DIR}/review-session.json`, spanning CLI invocations (attempts) until it converges, is superseded, or is exhausted.
- Call reason: `review` (round 0 reviewer), `verify` (later-round reviewer), `json_repair`, `provider_fallback` (reviewer of a lean fallback attempt), `adjudication` (judge).

## Requirements

### Scope

**REQ-RP-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL change only `internal/cli/spec_review*.go`, `pkg/spec/**`, the review budget keys of `pkg/config`, the spec writing, review, and plan sources under `templates/**` and `content/**`, the outputs `go run ./cmd/generate-templates` produces from them, `docs/` for spec review, and this SPEC directory, and SHALL NOT edit `pkg/worker/**`, `pkg/pipeline/**`, `internal/cli/status*`, `internal/cli/disk*`, or any generated root.

### Review Policy (R6)

**REQ-RP-POL-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL define in `pkg/spec` one review policy per profile `compact`, `lean`, and `full` that names the required documents and sections, the sections the profile may omit, the checklist items that become not applicable when their section is omitted, the gates that are never not applicable (`security`, `validation`, `data_loss`, `deterministic_oracle`), whether the Risk-First Integration Probe and the Self-Verify Summary are required, and the review execution shape (reviewer count, default judge, quorum).

**REQ-RP-POL-02**
Priority: Must
Type: Unwanted
IF a SPEC declares `profile: lean` but its change class is missing or high risk, its surface is missing, or its surface classifies above its class, THEN THE SYSTEM SHALL apply the full policy to it in the validator, the reviewer prompt, the checklist handling, and the review plan, and SHALL keep reporting the profile error.

**REQ-RP-POL-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL classify findings with one disposition rule set (the blocking-finding definition above), SHALL write the same rule lines into the default reviewer prompt and the judge template, SHALL resolve PASS only when no finding blocks and no verdict-scoped blocker exists, and SHALL reserve REJECT for a reviewer or judge that states a fundamental redesign.

**REQ-RP-POL-04**
Priority: Must
Type: Event-driven
WHEN a judge returns PASS while accepting a blocking finding, THEN THE SYSTEM SHALL keep that finding open and resolve REVISE with a blocking reason naming it, and SHALL defer only advisory findings.

**REQ-RP-POL-05**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep the categories `contract` and `data_loss` (including the spellings `data-loss` and `data loss`) when normalizing structured reviewer and judge findings instead of mapping them to `completeness`.

**REQ-RP-POL-06**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL state in the reviewer prompt that the review answers whether implementation can start, that a requirement conflict, an undecided choice that changes what gets built, an acceptance criterion that cannot be verified, and a security or data-loss risk block and are reported as major or above or under an impact category, that wording, structure, and implementer-delegated choices are reported as `suggestion`, and that requirements and evidence are judged rather than heading names; SHALL list for an eligible lean SPEC its allowed omissions and its not-applicable checklist items; and SHALL append a configured `pass_criteria` after the canonical rules as additional project criteria that cannot replace them.

**REQ-RP-POL-07**
Priority: Must
Type: Event-driven
WHEN a reviewer reports FAIL for a checklist item that the effective policy marks not applicable because of an omitted section, and that section is absent from the SPEC documents, THEN THE SYSTEM SHALL record the item as N/A with a reason naming the profile and the section, and the item SHALL NOT produce a `reviewer_checklist_fail` blocker.

**REQ-RP-POL-08**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL take the sections `ValidateSpecSet` requires from the effective policy, so an eligible lean SPEC is held to the lean floor and every other SPEC, including an ineligible lean one, to the full section set, the Self-Verify Summary, and the Risk-First Integration Probe.

**REQ-RP-POL-09**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL state the lean floor and the lean allowed omissions, exactly as the policy lists them, in the spec-writer source, the spec-review skill, the quality checklist, and the four auto-plan surfaces (Claude workflows, Codex skill, Codex prompt, Gemini skill), SHALL make no lean surface require an omitted section, and SHALL regenerate the Codex and Gemini outputs of the spec-writer and spec-review sources.

**REQ-RP-POL-10**
Priority: Must
Type: Unwanted
IF an eligible lean SPEC receives a finding in an impact category, an escape-hatch finding, or a major finding about a missing acceptance criterion, or a checklist FAIL whose section is present, THEN THE SYSTEM SHALL block on it exactly as it does for a full SPEC.

### Review Plan (R7)

**REQ-RP-PLAN-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL accept `--profile auto|full|lean` (default `auto`, the SPEC's effective profile) as the only profile selector, `--providers` as the reviewer selector, and `--judge <provider|none>` as the judge selector of `auto spec review`.

**REQ-RP-PLAN-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL resolve the plan with this precedence: profile from `--profile full`, else the effective profile; on `full`, reviewers from `--providers`, else configuration and `--multi` discovery, and quorum from `min_providers` or the majority default; on `lean`, exactly one reviewer, the first `--providers` name with the remaining names as ordered fallbacks and never a provider the operator did not name, or without `--providers` the configured order with the configured judge first and the rest as fallbacks, and quorum 1; the judge from `--judge`, else none on `lean`, else `spec.review_gate.judge`.

**REQ-RP-PLAN-03**
Priority: Must
Type: Event-driven
WHEN `--providers` names more than one provider or `--multi` is set for a lean review, `--judge` adds a judge to a lean review, or `--profile full` raises a lean SPEC, THEN THE SYSTEM SHALL print, before any model call, one warning line naming the setting, what it did before this SPEC, and what it does now.

**REQ-RP-PLAN-04**
Priority: Must
Type: Unwanted
IF `--profile` has a value other than `auto`, `full`, or `lean`, `--profile lean` is requested for a SPEC whose effective profile is not lean, `--judge` is empty, or `--new-session` and `--extend-budget` are both set, THEN THE SYSTEM SHALL fail before any readiness probe or model call with an error naming the setting and the remedy.

**REQ-RP-PLAN-05**
Priority: Must
Type: Event-driven
WHEN a review attempt has assembled its reviewers and judge, THEN THE SYSTEM SHALL print one plan block before its first model call naming the effective profile and its reason, the declared and effective risk class and tier, the reviewers and their selection source, the fallbacks, the judge or `none` and the reason, the quorum, the maximum rounds, the expected calls per round, the per-round upper bound including JSON repair, the attempt upper bound including fallback, and the session budget with its limits, used and remaining values, and a note for every token or cost limit it cannot guarantee.

**REQ-RP-PLAN-06**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL call exactly the reviewers and judge the printed plan names, and SHALL never issue more model calls in an attempt than the printed attempt upper bound.

### Review Budget (R10)

**REQ-RP-BUD-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL read optional `spec.review_gate.budget` keys `max_calls`, `max_minutes`, `max_tokens`, and `max_cost_usd` (absent or 0 meaning no limit for that dimension, all `omitempty`), SHALL accept `--max-calls` and `--max-minutes` as per-invocation overrides, and SHALL fail configuration load with an error naming the dotted key when a value is negative or the cost is not finite.

**REQ-RP-BUD-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep one review session per SPEC in `{SPEC_DIR}/review-session.json` with schema `spec_review_session.v1`, its limits fixed when the session starts, and SHALL meter against it every reviewer call, judge call, JSON repair, lean fallback call, every round, and every later invocation while the session is open.

**REQ-RP-BUD-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL reserve one call slot before each model call starts, under a lock shared by parallel calls, SHALL settle the call when it returns with its duration, usage, model, effort, and end reason, and SHALL refuse to start a call that cannot reserve, recording it as blocked without counting it as used.

**REQ-RP-BUD-04**
Priority: Must
Type: Event-driven
WHEN the remaining calls cannot cover the next round's reviewers or the session's wall-clock budget is spent, THEN THE SYSTEM SHALL start no further round, end the loop with loop status `budget_exhausted`, keep the last findings and blocking reasons, add the verdict-scoped blocking reason `budget_exhausted`, never resolve PASS because of the exhaustion, and mark the session `budget_exhausted`.

**REQ-RP-BUD-05**
Priority: Must
Type: Event-driven
WHEN the wall-clock budget runs out while calls are in flight, THEN THE SYSTEM SHALL cancel them through their context, record them with end reason `budget_exhausted`, keep the outputs of calls that completed, and add the degraded reason `budget_exhausted` to any PASS of a round in which a call was blocked or cancelled by the budget.

**REQ-RP-BUD-06**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record usage as `actual` when the provider reported it, `estimated` when the provider reported an estimate, and `unknown` otherwise, SHALL write unknown token and cost values as JSON null and never as 0, SHALL enforce token and cost limits only against known usage before starting a call, and SHALL state in the plan and the receipt when an unknown usage makes a token or cost limit unenforceable.

**REQ-RP-BUD-07**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL add to `review-receipt.json` an optional `session` object carrying the session id, attempt id and sequence, session state, attempt end reason, limits, used and remaining values, and one row per call of the attempt with call id, round, role, reason, provider, model and effort or `unknown`, usage, start time, duration, and end reason, SHALL write no prompt text, output text, or secret into it, and SHALL keep every existing receipt field and the loading of receipts without the object.

**REQ-RP-BUD-08**
Priority: Must
Type: Event-driven
WHEN an invocation finds an open session, THEN THE SYSTEM SHALL continue it with cumulative counts; WHEN it finds a session ended by `budget_exhausted`, THEN THE SYSTEM SHALL make no model call and report `budget_exhausted` with the last verdict and blockers unless `--extend-budget` re-applies the current limits to that session as a recorded extension or `--new-session` starts a session linked to it; WHEN the session converged, THEN THE SYSTEM SHALL start a new linked session.

**REQ-RP-BUD-09**
Priority: Must
Type: Unwanted
IF `review-session.json` cannot be parsed, carries an unknown schema, or another invocation holds the session lock, THEN THE SYSTEM SHALL fail before any model call with an error naming the file and the remedy.

**REQ-RP-BUD-10**
Priority: Must
Type: Unwanted
IF a previous process ended with a call still in flight, THEN THE SYSTEM SHALL record that call as `interrupted` on the next load and count it as used.

**REQ-RP-BUD-11**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL merge call rows from the session file and receipts by `(session_id, call_id)`, counting a repeated identical row once and reporting a conflicting duplicate instead of summing it.

**REQ-RP-BUD-12**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL print after every review attempt one line per model call of the attempt with its role, reason, provider, duration, end reason, and usage status, so a single-pass review shows every reviewer, repair, fallback, and judge call it made.

### Verification

**REQ-RP-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL ship a regression test for every reproduced defect that fails on origin/main 109c7ea4, SHALL use only fake backends, and SHALL keep every changed Go file within 300 counted lines.

## Generated And Changed Surfaces

| Surface | Change |
|---------|--------|
| `pkg/spec/review_policy*.go` [NEW], `profile.go`, `quality_preflight.go`, `prompt.go`, `blocking.go`, `reviewer_structured.go` | policy object, validator and prompt consumption, disposition rules, category normalization |
| `internal/cli/spec_review_plan*.go` [NEW], `spec_review_lean.go`, `spec_review.go` | plan resolution, precedence, warnings, printed plan, new flags |
| `internal/cli/spec_review_session*.go`, `spec_review_ledger*.go`, `spec_review_budget*.go` [NEW], `spec_review_loop.go`, `spec_review_structured.go`, `spec_review_reprompt.go`, `spec_review_receipt.go`, `spec_review_loop_merge.go` | session file, ledger, metered backend, round pre-check, receipt session block, judge reconciliation |
| `pkg/config/schema_spec.go`, `schema.go` | `budget` keys and validation |
| `templates/shared/spec-review-judge.md.tmpl`, `content/agents/spec-writer.md`, `content/skills/spec-review.md`, `content/rules/spec-quality.md`, `templates/{claude,codex,gemini}` auto-plan sources | canonical rules and lean allowances; regenerated Codex and Gemini agent and skill templates |
| `docs/spec-review-policy.md` [NEW] | operator documentation of the policy, the plan, and the budget |

## Related SPECs

None as siblings. Reused contracts: the lean profile of the compact-change work (`pkg/spec/profile.go`, `pkg/spec/gates`), the blocking matrix and loop status of issue #187 (`pkg/spec/blocking.go`), the read-only review assembly and readiness preflight of `SPEC-REVIEWRO-001`, and the observation-integrity promotion gate of `SPEC-ADK-REVIEW-INTEGRITY-001`, none of which is reopened beyond the requirements above.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-RP-SCOPE-01 | T1-T12 | S16 | INV-RP-12 |
| REQ-RP-POL-01 | T1 | S1 | INV-RP-01 |
| REQ-RP-POL-02 | T1, T2 | S2 | INV-RP-02 |
| REQ-RP-POL-03 | T3 | S3 | INV-RP-03 |
| REQ-RP-POL-04 | T3 | S3 | INV-RP-03 |
| REQ-RP-POL-05 | T3 | S3 | INV-RP-03 |
| REQ-RP-POL-06 | T3 | S4 | INV-RP-04 |
| REQ-RP-POL-07 | T3 | S5 | INV-RP-05 |
| REQ-RP-POL-08 | T2 | S2 | INV-RP-02 |
| REQ-RP-POL-09 | T4 | S6 | INV-RP-06 |
| REQ-RP-POL-10 | T3 | S5 | INV-RP-05 |
| REQ-RP-PLAN-01 | T5 | S7 | INV-RP-07 |
| REQ-RP-PLAN-02 | T5 | S7 | INV-RP-07 |
| REQ-RP-PLAN-03 | T5 | S7 | INV-RP-07 |
| REQ-RP-PLAN-04 | T5 | S8 | INV-RP-07 |
| REQ-RP-PLAN-05 | T6 | S7 | INV-RP-08 |
| REQ-RP-PLAN-06 | T6 | S7 | INV-RP-08 |
| REQ-RP-BUD-01 | T7 | S9 | INV-RP-09 |
| REQ-RP-BUD-02 | T8, T9 | S10 | INV-RP-09 |
| REQ-RP-BUD-03 | T9 | S11 | INV-RP-10 |
| REQ-RP-BUD-04 | T10 | S12 | INV-RP-11 |
| REQ-RP-BUD-05 | T10 | S13 | INV-RP-11 |
| REQ-RP-BUD-06 | T9 | S14 | INV-RP-10 |
| REQ-RP-BUD-07 | T11 | S15 | INV-RP-10 |
| REQ-RP-BUD-08 | T8 | S10, S12 | INV-RP-09 |
| REQ-RP-BUD-09 | T8 | S10 | INV-RP-09 |
| REQ-RP-BUD-10 | T8 | S10 | INV-RP-09 |
| REQ-RP-BUD-11 | T11 | S15 | INV-RP-10 |
| REQ-RP-BUD-12 | T11 | S15 | INV-RP-10 |
| REQ-RP-VERIFY-01 | T12 | S16 | INV-RP-12 |
