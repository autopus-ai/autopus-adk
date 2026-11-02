# SPEC-REVIEWREUSE-001: Conditional Judge, Cross-Invocation Review Reuse, and Delta Verify

**Status**: draft
**Created**: 2026-11-02
**Domain**: REVIEWREUSE
**Priority**: HIGH
**Change Class**: feature (new review flags and config keys, a receipt fingerprint, a findings-sidecar baseline, a delta verify input contract)
**Source**: requirements R8, R11, and R9 of the ADK improvement brief dated 2026-11-01, re-verified on feat/spec-review-policy-budget 48607ed2 (SPEC-REVIEWPOLICY-001)
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `.autopus/specs/SPEC-REVIEWREUSE-001/**`; `internal/cli/spec_review*.go` and their tests; `internal/cli/testdata/panerm_s16/spec-review-receipt.json`; `pkg/spec/findings*.go`, `pkg/spec/types.go`, `pkg/spec/judge.go`, `pkg/spec/checklist.go`, `pkg/spec/metadata_status.go`, `[NEW] pkg/spec/review_merge*.go`, `[NEW] pkg/spec/review_fingerprint*.go`, `[NEW] pkg/spec/review_delta*.go`, `[NEW] pkg/spec/review_baseline*.go` and their tests; `pkg/config/schema_spec.go`, `pkg/config/schema.go` (validation call), `[NEW] pkg/config/schema_review_modes*.go`; `docs/spec-review-policy.md`, `docs/README.ko.md`
**Depends on**: SPEC-REVIEWPOLICY-001 (review policy, review plan, session budget, call ledger)

## Purpose

A SPEC review pays for model calls that cannot change its outcome. The judge re-adjudicates rounds in which every reviewer already agrees. A new `auto spec review` invocation re-reviews documents it reviewed minutes earlier, byte for byte. Every verify round resends the whole document set even when the author changed three lines. This SPEC removes those calls without weakening the gate:

- the judge runs only when reviewers disagree or a safety claim is contested, if the operator opts in; without a judge the reviewer findings merge deterministically and never drop a minority critical, security, or data-loss claim;
- a fingerprint of every review input lets a later invocation return the previous PASS, REJECT, or awaiting-changes result with zero calls when nothing changed, and any change or corruption invalidates it;
- an opt-in delta verify input sends the changes, the affected requirements and scenarios, and the evidence for each unresolved finding, links unchanged material to a hash-verified baseline, and falls back to the full input whenever the delta cannot prove it carries every required context.

## Background

Observed on 48607ed2:

- `runStructuredSpecReviewOrchestra` (`internal/cli/spec_review_structured.go`) calls the judge after the parallel reviewers whenever a judge is configured and one reviewer response is usable. Agreement, findings, and reviewer count never skip it.
- Without a valid judge, reviewer findings merge through `spec.MergeSupermajority` (`pkg/spec/findings.go`): a group of findings with the same category, scope, and description prefix survives only when `count/total + 0.005 >= threshold`, except category `security`. Its doc comment says critical findings bypass the threshold too; the code does not. A REVISE vote keeps the round at REVISE, and when no group reaches the threshold every finding is kept; but when every reviewer votes PASS, or when another group reaches the threshold, a minority critical or `data_loss` finding is dropped. With three reviewers and threshold 0.67, a reviewer that votes PASS while reporting a critical correctness finding resolves PASS with no finding, and a critical correctness finding raised by one reviewer beside a major finding raised by the other two disappears from the result. The survivor of a group is its first member: its severity wins and the other reviewers that raised it are not recorded.
- The same-input tracker (`internal/cli/spec_review_loop_repeat.go`) lives inside one `runSpecReviewLoop` call. A new invocation starts with an empty tracker, so re-running `auto spec review` on unchanged documents pays for a full first round again.
- A verify round builds the discover prompt (full SPEC, auxiliary documents, code context, checklist) and appends the prior findings (`pkg/spec/prompt.go` `BuildReviewPrompt`). The GPT/Codex path checks that core, architecture, required references, and the four SPEC documents are delivered complete and hash-verified (`pkg/spec/prompt_context_delivery*.go`); cutting that input without a new contract would break its guarantee.

## Outcome Boundary

- In scope: the `judge_mode` setting, the conditional judge decision, the deterministic merge with provenance, and the judge decision record (R8); the review input fingerprint, the reuse decision and its output, and `--force-review` (R11); the review baseline, the delta verify input contract, its integrity check, and the full fallback (R9); documentation of all three.
- Out of scope: verifying a finding only with the reviewer that raised it, sharing results across SPECs or machines, default budget numbers, R1 to R5 of the brief, and any path outside the owned list.
- Defaults: `judge_mode: always` and `verify_input: full` keep today's prompts, calls, and verdicts. Only the deterministic merge rule (REQ-RR-JDG-05) changes behaviour in every mode, because it closes the dropped-minority-claim defect above.

## Definitions

- Usable review: a reviewer response that executed, produced output, and parsed, after the checklist policy of SPEC-REVIEWPOLICY-001 applied.
- Blocking item of a review: an active blocking finding under the canonical rules, a non-waived checklist FAIL, or a REJECT vote.
- Escalation item of a review: a REJECT vote, or a finding with severity `critical`, category `security` or `data_loss`, or the escape hatch.
- Contested: present in at least one usable review of the round and absent from at least one other.
- Pinned model selector: a provider's model selector (its `model` field, else the value after `--model` or `-m` in its arguments) that is non-empty, contains a digit, is not `default`, and does not end in `latest`.
- Required source: anything the full verify input of a round carries: each SPEC document, each context-delivery source (core, architecture, required reference), the checklist, and the code context when one is injected.
- Risk-boundary region: the lines of a document before its first `## ` heading; spec.md `## Outcome Boundary`, `## Definitions`, `## Generated And Changed Surfaces`, `## Related SPECs`; research.md `## Trust Boundary` and `## Outcome Lock`; and any added or removed line that defines a requirement id (`**REQ-...**`) or an acceptance scenario heading (`### S<n>`).
- Scope closure of a finding: the requirement, acceptance, and scenario ids and the document or heading names in its `scope_ref`, plus every id that shares a traceability-matrix row with one of them.

## Requirements

### Scope

**REQ-RR-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL implement this SPEC only in the owned paths listed in the header, SHALL keep every Go source file at or below 300 counted lines, SHALL add no file under a SPEC directory other than fields inside existing review artifacts, and SHALL leave every generated root untouched.

### Conditional Judge (R8)

**REQ-RR-JDG-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL read the optional key `spec.review_gate.judge_mode` with values `always`, `on_disagreement`, and `on_escalation` (omitted meaning `always`, written only when set), SHALL accept `--judge-mode <mode>` as the invocation's override, SHALL fail configuration load with an error naming the dotted key and the allowed values for any other value, SHALL refuse an invalid flag value before any readiness probe or model call, and SHALL print the effective mode and its run condition in the plan block whenever a judge is planned.

**REQ-RR-JDG-02**
Priority: Must
Type: State-driven
WHILE the effective judge mode is `always`, THE SYSTEM SHALL run the judge exactly as before: once per round with at least one usable review, through the same call path, prompt, call reason, and verdict handling.

**REQ-RR-JDG-03**
Priority: Must
Type: Event-driven
WHEN a round's reviewers have returned under `on_disagreement`, THEN THE SYSTEM SHALL run the judge only if there are at least two usable reviews and their verdicts differ or a blocking item or an escalation item is contested; otherwise it SHALL make no judge call and record judge status `skipped` with reason `single_reviewer`, `consensus_pass`, or `consensus_block`.

**REQ-RR-JDG-04**
Priority: Must
Type: Event-driven
WHEN a round's reviewers have returned under `on_escalation`, THEN THE SYSTEM SHALL run the judge only if there are at least two usable reviews and an escalation item is contested; otherwise it SHALL make no judge call and record judge status `skipped` with reason `single_reviewer`, `no_escalation`, or `unanimous_escalation`.

**REQ-RR-JDG-05**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL merge reviewer findings that share a group key (in the supermajority merge and in duplicate removal) into one finding that carries the most severe member severity, the escape hatch when any member has it, and `Sources` naming each reviewer that raised it once in reviewer order, and SHALL keep a group whose members include severity `critical`, category `security` or `data_loss`, or the escape hatch regardless of the supermajority threshold; every other group SHALL follow the existing threshold.

**REQ-RR-JDG-06**
Priority: Must
Type: Unwanted
IF the judge that the effective mode calls for fails, times out, returns invalid output, or is refused by the session budget, THEN THE SYSTEM SHALL resolve the round from the deterministic merge of REQ-RR-JDG-05 and the verdict-scoped blockers of SPEC-REVIEWPOLICY-001, SHALL record the judge status, and SHALL NOT resolve PASS while an escalation item of a usable review is open.

**REQ-RR-JDG-07**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record in `review-receipt.json` the judge `mode`, a `skipped` status with its reason, and, for a judge that ran, one `decisions` entry per judged finding with its id, decision, reviewers (provider names), scope_ref, reason truncated to 500 runes, and `merge_into`, and SHALL count the judge as zero or one call per round in the plan's expected calls under a conditional mode while keeping it in the upper bound.

### Review Reuse (R11)

**REQ-RR-REUSE-01**
Priority: Must
Type: Event-driven
WHEN an invocation ends after at least one round ran, THEN THE SYSTEM SHALL write into `review-receipt.json` a `fingerprint` object (schema `spec_review_fingerprint.v1`) of the final round's inputs: `documents` (SHA-256 of every `*.md` in the SPEC directory except `review.md`, spec.md hashed with its status value normalized); `references` (SHA-256 of every context-delivery source and every `--required-document`); `code` (git `HEAD`, a digest of the status, path, and content digest of every changed or untracked file outside the SPEC directory, and the SHA-256 of the injected code context); `checklist` (SHA-256 of the checklist body); `policy` (effective profile, judge mode, verify input, strategy, verdict threshold, minimum providers, failed-provider denominator flag, maximum revisions, SHA-256 of `pass_criteria`); `contract` (SHA-256 over the discover and verify prompts built for a fixed probe SPEC, the reviewer JSON schema, and the judge prompt built for fixed data); `providers` (per reviewer and judge: name, role, backend, binary, model selector, SHA-256 of the arguments, model policy, pinned); `evidence` (SHA-256 of `review-findings.json` and `review.md` as the invocation left them); and `complete`, false with one reason per component that failed to compute.

**REQ-RR-REUSE-02**
Priority: Must
Type: Event-driven
WHEN an invocation starts without `--force-review`, THEN THE SYSTEM SHALL compute the current fingerprint before any readiness probe or model call and SHALL reuse the previous result only if the receipt parses with its known schema and carries a complete `spec_review_fingerprint.v1`, the current fingerprint is complete and equal to it in every component except `evidence`, `review-findings.json` and `review.md` match the `evidence` digests, the receipt's `finished_at` is at most 168 hours old, and the result is reusable under REQ-RR-REUSE-03 and REQ-RR-REUSE-04.

**REQ-RR-REUSE-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL treat as reusable exactly a PASS with `gate_status` `passed`, `loop_status` `converged`, and no degraded reason; a REJECT with no degraded reason and a `loop_status` other than `budget_exhausted` and `provider_unavailable`; and a REVISE with no degraded reason, `loop_status` `awaiting_changes` or `revisions_exhausted`, and no blocking reason `no_usable_review` or `budget_exhausted`, which it reports as `awaiting_changes`.

**REQ-RR-REUSE-04**
Priority: Must
Type: Unwanted
IF a reviewer or the judge recorded in the previous fingerprint has a model selector that is not pinned, THEN THE SYSTEM SHALL NOT reuse a PASS, and SHALL still reuse a reusable REJECT or awaiting-changes result.

**REQ-RR-REUSE-05**
Priority: Must
Type: Event-driven
WHEN a result is reused, THEN THE SYSTEM SHALL make no readiness probe and no model call, SHALL print the reuse with the previous run id and finish time, the verdict and loop status, the finding summary, `모델 호출 0회`, and the `--force-review` remedy, SHALL leave `review-receipt.json`, `review-findings.json`, `review.md`, and the SPEC status unchanged, and SHALL record the invocation in the review session as an attempt with end reason `reused` and no calls.

**REQ-RR-REUSE-06**
Priority: Must
Type: Unwanted
IF a previous receipt exists and its result is not reused, THEN THE SYSTEM SHALL print one line `리뷰 재사용 안 함:` naming every reason (each changed component with the changed document, reference, or provider name; a corrupt or unknown-schema receipt; an incomplete fingerprint; changed evidence files; age over 168 hours; a non-reusable result; a PASS with an unpinned model) and SHALL run the review as without reuse.

**REQ-RR-REUSE-07**
Priority: Must
Type: Event-driven
WHEN `--force-review` is passed, THEN THE SYSTEM SHALL skip reuse, run the review against the same session budget, and record `forced_review: true` on the session attempt and in the receipt's `session` object.

### Delta Verify (R9)

**REQ-RR-DELTA-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL read the optional key `spec.review_gate.verify_input` with values `full` and `delta` (omitted meaning `full`, written only when set), SHALL accept `--verify-input <full|delta>` as the invocation's override, SHALL fail configuration load with an error naming the dotted key for any other value, and SHALL refuse an invalid flag value before any readiness probe or model call.

**REQ-RR-DELTA-02**
Priority: Must
Type: Event-driven
WHEN a round produced at least one usable review and the budget did not cut it, THEN THE SYSTEM SHALL store in `review-findings.json` a `baseline` object (schema `spec_review_baseline.v1`) with the run id, revision, time, the content and SHA-256 of every SPEC document of the round, the SHA-256 of every other required source, the code, policy, and contract digests of REQ-RR-REUSE-01, and the ids of the findings left open or regressed.

**REQ-RR-DELTA-03**
Priority: Must
Type: Event-driven
WHEN a verify round starts under `verify_input: delta`, THEN THE SYSTEM SHALL use the delta input only if, checked in this order: a baseline with a known schema exists (else `missing_baseline`); every stored document hashes to its recorded digest and the baseline's open ids equal the prior findings' open ids (else `baseline_hash_mismatch`); every non-document required source, the code digests, the checklist, the policy, and the contract equal the baseline (else `context_changed`); no SPEC document was added or removed and no change touches a risk-boundary region (else `interface_change`); every unresolved finding's scope closure names text present in the current documents (else `unclear_impact`); every change lies in a section that names an id or name of some unresolved finding's scope closure (else `out_of_scope_change`); at least one document changed (else `no_change`); and the delta input is shorter than the full verify input (else `no_saving`). The first failed condition SHALL select the full verify input with that reason.

**REQ-RR-DELTA-04**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL build the delta input (contract `spec_review_delta.v1`) from the review purpose, the `## SPEC:` header, the baseline run and revision, a manifest naming every required source with its current digest and delivery (`included`, `changed`, or `linked` to the baseline), a unified diff of each changed document, the full current text of every section a change touches, the full current text of every requirement block and scenario section in the scope closure of the unresolved findings with the traceability rows that mention them, per unresolved finding the changes in its scope closure or the line `no change touches this finding`, the full checklist, the review policy section, and the verify instructions with the prior findings.

**REQ-RR-DELTA-05**
Priority: Must
Type: Unwanted
IF the delta manifest omits a required source, links a source whose current digest differs from the baseline, or marks a source changed without carrying its diff, THEN THE SYSTEM SHALL discard the delta input and use the full verify input with reason `integrity_gap`.

**REQ-RR-DELTA-06**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record for every round its input (`delta` or `full`) and the reason (`discover`, `configured_full`, `delta`, or the fallback reason) in the receipt's `rounds` entries, SHALL print it before the round's calls, and SHALL reserve every call of a fallback round from the same session budget.

**REQ-RR-DELTA-07**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL let a delta round report new critical or security findings and newly broken behaviour exactly as a full verify round does, and SHALL leave the full verify input, its integrity checks, and their tests unchanged under `verify_input: full`.

### Verification

**REQ-RR-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL pass the commands in `plan.md` `## Verification` with fake backends only, including a race run over the judge decision and the reuse code and the existing full-input integrity tests unchanged.

## Generated And Changed Surfaces

- `auto spec review` gains `--judge-mode`, `--force-review`, and `--verify-input`.
- `autopus.yaml` gains the optional keys `spec.review_gate.judge_mode` and `spec.review_gate.verify_input`; the default configuration writes neither.
- `review-receipt.json` (`spec_review_promotion_receipt.v1`, additive and omitted when empty) gains `judge.mode`, `judge.decisions`, `fingerprint`, `rounds[].input`, `rounds[].input_reason`, and `session.forced_review`.
- `review-findings.json` (additive) gains `baseline`; each finding gains `Sources`.
- `review-session.json` attempts gain the end reason `reused` and `forced_review`.
- `docs/spec-review-policy.md` and the `docs/README.ko.md` rows describe the three features. No template, generated root, or new SPEC-directory file changes.

## Related SPECs

- SPEC-REVIEWPOLICY-001: the policy, plan, session budget, call reasons (`adjudication`, `verify`), and receipt this SPEC extends.
- SPEC-ADK-REVIEW-INTEGRITY-001: the full-input integrity contract the delta input must not weaken.
- SPEC-SPECREV-001: the supermajority merge this SPEC amends for escalation items.

## Traceability Matrix

| Requirement | Task | Scenario | Invariant |
|-------------|------|----------|-----------|
| REQ-RR-SCOPE-01 | T14 | S20 | INV-RR-13 |
| REQ-RR-JDG-01 | T1 | S1 | INV-RR-01 |
| REQ-RR-JDG-02 | T3 | S2 | INV-RR-02 |
| REQ-RR-JDG-03 | T2, T3 | S3 | INV-RR-03 |
| REQ-RR-JDG-04 | T2, T3 | S4 | INV-RR-03 |
| REQ-RR-JDG-05 | T4 | S5 | INV-RR-04 |
| REQ-RR-JDG-06 | T3 | S6 | INV-RR-05 |
| REQ-RR-JDG-07 | T5 | S1, S3, S7 | INV-RR-06 |
| REQ-RR-REUSE-01 | T6, T7 | S8 | INV-RR-07 |
| REQ-RR-REUSE-02 | T8 | S9, S10 | INV-RR-08 |
| REQ-RR-REUSE-03 | T8 | S11 | INV-RR-08 |
| REQ-RR-REUSE-04 | T8 | S10, S11 | INV-RR-08 |
| REQ-RR-REUSE-05 | T8 | S9 | INV-RR-09 |
| REQ-RR-REUSE-06 | T8 | S10 | INV-RR-08 |
| REQ-RR-REUSE-07 | T8 | S12 | INV-RR-09 |
| REQ-RR-DELTA-01 | T9 | S13 | INV-RR-10 |
| REQ-RR-DELTA-02 | T10 | S14 | INV-RR-10 |
| REQ-RR-DELTA-03 | T11 | S15 | INV-RR-11 |
| REQ-RR-DELTA-04 | T12 | S14 | INV-RR-11 |
| REQ-RR-DELTA-05 | T12 | S17 | INV-RR-12 |
| REQ-RR-DELTA-06 | T13 | S16 | INV-RR-11 |
| REQ-RR-DELTA-07 | T12, T13 | S18 | INV-RR-12 |
| REQ-RR-VERIFY-01 | T14 | S19 | INV-RR-13 |
