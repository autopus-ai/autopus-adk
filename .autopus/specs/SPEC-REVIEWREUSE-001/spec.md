# SPEC-REVIEWREUSE-001: Conditional Judge, Cross-Invocation Review Reuse, and Delta Verify

**Status**: draft
**Created**: 2026-11-02
**Domain**: REVIEWREUSE
**Priority**: HIGH
**Change Class**: feature (new review flags and config keys, a receipt fingerprint, a findings-sidecar baseline, a delta verify input contract)
**Source**: requirements R8, R11, and R9 of the ADK improvement brief dated 2026-11-01, re-verified on feat/spec-review-policy-budget 48607ed2 (SPEC-REVIEWPOLICY-001)
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `.autopus/specs/SPEC-REVIEWREUSE-001/**`; `internal/cli/spec_review*.go` and their tests; `internal/cli/testdata/panerm_s16/spec-review-receipt.json` and the S16 normalizer in `internal/cli/panerm_output_parity_test.go`; `pkg/spec/findings*.go`, `pkg/spec/types.go`, `pkg/spec/judge.go`, `pkg/spec/checklist.go`, `pkg/spec/metadata_status.go`, `[NEW] pkg/spec/review_merge*.go`, `[NEW] pkg/spec/review_fingerprint*.go`, `[NEW] pkg/spec/review_delta*.go`, `[NEW] pkg/spec/review_baseline*.go` and their tests; `pkg/config/schema_spec.go`, `pkg/config/schema.go` (validation call), `[NEW] pkg/config/schema_review_modes*.go`; `docs/spec-review-policy.md`, `docs/README.ko.md`
**Depends on**: SPEC-REVIEWPOLICY-001 (review policy, review plan, session budget, call ledger); SPEC-STATUSEVIDENCE-001 (the `specDocumentsFingerprint` helper in `internal/cli/spec_fingerprint.go`, called and not changed)

## Purpose

A SPEC review pays for model calls that cannot change its outcome. The judge re-adjudicates rounds in which every reviewer already agrees. A new `auto spec review` invocation re-reviews documents it reviewed minutes earlier, byte for byte. Every verify round resends the whole document set even when the author changed three lines. This SPEC removes those calls without weakening the gate:

- the judge runs only when reviewers disagree or a safety claim is contested, if the operator opts in; without a judge the reviewer findings merge deterministically and never drop a minority critical, security, or data-loss claim;
- a fingerprint of every review input lets a later invocation return the previous PASS, REJECT, or awaiting-changes result with zero calls when nothing changed, and any change or corruption invalidates it;
- an opt-in delta verify input sends a request that is complete in itself, because every review request starts a fresh provider session: the changes, every section they touch or that names an id of the open findings or the changes, the boundary sections, and every other required source in full, in the same sanitized representation the complete input delivers; it leaves out only SPEC documents that are unchanged since a baseline round that delivered them whole and that the request does not need, and falls back to the full input whenever it cannot prove that.

## Background

Observed on 48607ed2:

- `runStructuredSpecReviewOrchestra` (`internal/cli/spec_review_structured.go`) calls the judge after the parallel reviewers whenever a judge is configured and one reviewer response is usable. Agreement, findings, and reviewer count never skip it.
- Without a valid judge, reviewer findings merge through `spec.MergeSupermajority` (`pkg/spec/findings.go`): a group of findings with the same category, scope, and description prefix survives only when `count/total + 0.005 >= threshold`, except category `security`. Its doc comment says critical findings bypass the threshold too; the code does not. A REVISE vote keeps the round at REVISE, and when no group reaches the threshold every finding is kept; but when every reviewer votes PASS, or when another group reaches the threshold, a minority critical or `data_loss` finding is dropped. With three reviewers and threshold 0.67, a reviewer that votes PASS while reporting a critical correctness finding resolves PASS with no finding, and a critical correctness finding raised by one reviewer beside a major finding raised by the other two disappears from the result. The survivor of a group is its first member: its severity wins and the other reviewers that raised it are not recorded.
- Every review request starts a fresh provider process (`subprocessBackend.Execute`): the reviewer policy runs Claude with `--no-session-persistence` and Codex with `--ephemeral` (`internal/cli/orchestra_readonly_policy.go`), and `orchestra.ProviderRequest` has no field that restores a conversation. A reviewer knows only what its request carries.
- The complete-input path, used only when every reviewer is a GPT provider, delivers each SPEC document and context document through `promptlayer.SanitizeContent` (secrets redacted, injection markers neutralized) and records the source digest and the prompt digest apart (`pkg/spec/prompt_documents.go`, `pkg/promptlayer/context_delivery.go`). The inline path sends spec.md whole and compacts the auxiliary documents beyond the auxiliary line budget (`pkg/spec/doc_compaction.go`).
- The same-input tracker (`internal/cli/spec_review_loop_repeat.go`) lives inside one `runSpecReviewLoop` call. A new invocation starts with an empty tracker, so re-running `auto spec review` on unchanged documents pays for a full first round again.
- A verify round builds the discover prompt (full SPEC, auxiliary documents, code context, checklist) and appends the prior findings (`pkg/spec/prompt.go` `BuildReviewPrompt`). The GPT/Codex path checks that core, architecture, required references, and the four SPEC documents are delivered complete and hash-verified (`pkg/spec/prompt_context_delivery*.go`); cutting that input without a new contract would break its guarantee.

## Outcome Boundary

- In scope: the `judge_mode` setting, the conditional judge decision, the deterministic merge with provenance, and the judge decision record (R8); the review input fingerprint, the `pinned_models` declaration, the reuse decision and its output, `--force-review`, and the receipt's `spec_fingerprint` (R11); the review baseline, the delta verify input contract, its integrity check, and the full fallback (R9); documentation of all three.
- Out of scope: verifying a finding only with the reviewer that raised it, sharing results across SPECs or machines, default budget numbers, R1 to R5 of the brief, and any path outside the owned list.
- Defaults: `judge_mode: always` and `verify_input: full` keep today's prompts, calls, and verdicts. Only the deterministic merge rule (REQ-RR-JDG-05) changes behaviour in every mode, because it closes the dropped-minority-claim defect above.

## Definitions

- Usable review: a reviewer response that executed, produced output, and parsed, after the checklist policy of SPEC-REVIEWPOLICY-001 applied.
- Item key: what makes two reviews hold the same item: a finding's supermajority group key (category, normalized scope, normalized description prefix), a checklist FAIL's item id, and the REJECT vote itself. Severity is not part of a finding's key.
- Blocking item of a review: an active blocking finding under the canonical rules, a non-waived checklist FAIL, or a REJECT vote.
- Escalation item of a review: a REJECT vote, or a finding with severity `critical`, category `security` or `data_loss`, or the escape hatch.
- Contested: an item whose key at least one usable review of the round holds and at least one other does not; two reviews that each hold a different item contest both.
- Pinned model selector: a provider's model selector (its `model` field, else the value after `--model`, `-m`, or `--model=` in its arguments) that names one immutable model: it ends in a calendar-dated snapshot suffix (`-YYYY-MM-DD`, `-YYYYMMDD`, or `@YYYYMMDD` with a valid month and day), or it is listed in `spec.review_gate.pinned_models`. A digit is not a version: `gpt-4o` and `claude-opus-5-5` are rolling aliases.
- Carried document: a SPEC document the full review input carries: spec.md, plan.md, research.md, and acceptance.md, when present. Other files in the SPEC directory are not part of what a reviewer reads.
- Required source: anything the full verify input of a round carries: each carried document, the checklist, the code context when one is injected, and each context-delivery source (core, architecture, required reference) when the round's prompt is built from a verified context delivery, which happens only when every reviewer is a GPT provider.
- Delivered representation: the text a request carries for a carried document: its source passed through `promptlayer.SanitizeContent` with `Required` and `PreserveInjectionEvidence`, as the complete input does (secrets replaced by `[REDACTED_SECRET]`, injection markers neutralized in place). Its SHA-256 is the prompt digest; the SHA-256 of the file is the source digest.
- Delivered scope: for each carried document of a round, `complete` when the request the round actually sent carried the document's whole delivered representation, and `partial` otherwise. A full input from a verified context delivery carries every document complete; an inline full input carries spec.md complete and each auxiliary document as its injection coverage says; a delta request carries none complete, because it sends a changed document as a diff and sections and a linked document not at all.
- Risk-boundary region: the lines of a document before its first `## ` heading; spec.md `## Outcome Boundary`, `## Definitions`, `## Generated And Changed Surfaces`, `## Related SPECs`; research.md `## Trust Boundary` and `## Outcome Lock`; and any added or removed line that defines a requirement id (`**REQ-...**`) or an acceptance scenario heading (`### S<n>`).
- Scope closure of a finding: the requirement, acceptance, and scenario ids and the document or heading names in its `scope_ref`, plus every id that shares a spec.md traceability-matrix row with one of them.
- Attribution unit: the unit a changed line belongs to: the requirement block that holds it (from a `**REQ-...**` line to the next requirement line or heading), else the acceptance scenario section (from a `### S<n>` heading to the next heading), else the line itself within its nearest H2 or H3 heading section. Sibling blocks under one heading are separate units.
- Delta scope ids: the scope closure of every unresolved finding plus every id a changed line names, removed or added.
- Boundary section: a carried document's header (the lines before its first unit) and every unit of its risk-boundary sections.
- Scope section: a unit of a carried document whose label or text names a delta scope id, or that lies in a document or under a heading an unresolved finding's scope names; spec.md's traceability matrix contributes only its rows that name a delta scope id.

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
WHEN a round's reviewers have returned under `on_escalation`, THEN THE SYSTEM SHALL run the judge only if there are at least two usable reviews and an escalation item is contested, comparing items by key so that two reviews holding different escalation items run the judge; otherwise it SHALL make no judge call and record judge status `skipped` with reason `single_reviewer`, `no_escalation` (no review holds an escalation item), or `unanimous_escalation` (every review holds the same escalation items).

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
WHEN an invocation ends after at least one round ran, THEN THE SYSTEM SHALL write into `review-receipt.json` a `fingerprint` object (schema `spec_review_fingerprint.v1`) of the final round's inputs: `documents` (SHA-256 of every `*.md` in the SPEC directory except `review.md`, spec.md hashed with its status value normalized); `references` (SHA-256 of every context-delivery source and every `--required-document`); `code` (git `HEAD`, a digest of the status, path, and content digest of every changed or untracked file outside the SPEC directory, and the SHA-256 of the injected code context); `checklist` (SHA-256 of the checklist body); `policy` (effective profile, judge mode, verify input, strategy, verdict threshold, minimum providers, failed-provider denominator flag, maximum revisions, SHA-256 of `pass_criteria`, `doc_context_max_lines`, conditional profiles, and `pinned_models`); `config` (SHA-256 of the effective configuration that was actually loaded, wherever it lives, an external `--config` included, after migration, inheritance, and flag overrides); `delivery` (the effective auxiliary line budget and, for each auxiliary document, the SHA-256 of the excerpt the full input injects under it); `contract` (SHA-256, computed for every fingerprint, over every request text built for fixed probe data: the discover and verify prompts with inline and with complete sanitized documents and the delta input, each through the production structured response wrapper with and without the inline reviewer schema, the judge prompt in both modes, and the JSON repair request); `providers` (per reviewer and judge: name, role, backend, binary, model selector, SHA-256 of the arguments, model policy, pinned); `evidence` (SHA-256 of `review-findings.json` and `review.md` as the invocation left them); and `complete`, false with one reason per component that failed to compute; and it SHALL write `spec_fingerprint`, the value `specDocumentsFingerprint` (SPEC-STATUSEVIDENCE-001) returns for the SPEC documents the final round read.

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
IF a reviewer or the judge recorded in the previous fingerprint has a model selector that is not pinned, THEN THE SYSTEM SHALL NOT reuse a PASS, and SHALL still reuse a reusable REJECT or awaiting-changes result; the optional key `spec.review_gate.pinned_models` (omitted writes no key) declares selectors the operator vouches are immutable, and configuration load SHALL fail naming `spec.review_gate.pinned_models[<index>]` for an empty value, `default`, or a value ending in `latest`.

**REQ-RR-REUSE-05**
Priority: Must
Type: Event-driven
WHEN a result is reused, THEN THE SYSTEM SHALL make no readiness probe and no model call, SHALL print the reuse with the previous run id and finish time, the verdict and loop status, the finding summary, `모델 호출 0회`, and the `--force-review` remedy, SHALL leave `review-receipt.json`, `review-findings.json`, `review.md`, and the SPEC status unchanged, and SHALL record the invocation in the review session as an attempt with end reason `reused` and no calls.

**REQ-RR-REUSE-06**
Priority: Must
Type: Unwanted
IF a previous receipt exists and its result is not reused, THEN THE SYSTEM SHALL print one line `리뷰 재사용 안 함:` naming every reason (each changed component with the changed document, reference, policy field, delivery item, or provider name, `config`, and `contract`; a corrupt or unknown-schema receipt; an incomplete fingerprint; changed evidence files; age over 168 hours; a non-reusable result; a PASS with an unpinned model) and SHALL run the review as without reuse.

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
WHEN a round produced at least one usable review and the budget did not cut it, THEN THE SYSTEM SHALL store in `review-findings.json` a `baseline` object (schema `spec_review_baseline.v1`) with the run id, revision, and time; for every carried document of the round its `source_sha256`, its delivered representation as `content`, and that representation's `prompt_sha256`, never the raw source; the delivered scope of the request the round actually sent as `delivered`, so the baseline a delta round stores records every document `partial`; the SHA-256 of every other required source; the code, policy, configuration, and contract digests of REQ-RR-REUSE-01 and a digest of the SPEC-directory markdown files that are not carried documents; and the ids of the findings left open or regressed.

**REQ-RR-DELTA-03**
Priority: Must
Type: Event-driven
WHEN a verify round starts under `verify_input: delta`, THEN THE SYSTEM SHALL use the delta input only if, checked in this order: a baseline with a known schema exists (else `missing_baseline`); every stored representation hashes to its recorded `prompt_sha256` and the baseline's open ids equal the prior findings' open ids (else `baseline_hash_mismatch`); the baseline records every document it holds as delivered `complete` (else `incomplete_baseline`); every non-document required source, the code digests, the checklist, the policy, the configuration, the contract, and the digest of the markdown files that are not carried documents equal the baseline (else `context_changed`); no carried document was added or removed (else `interface_change`); every document whose source digest changed also changed its delivered representation, and no representation changed without its source (else `integrity_gap`); no change touches a risk-boundary region (else `interface_change`); every unresolved finding's scope closure names text present in the current documents (else `unclear_impact`); every changed line's attribution unit is in scope: a requirement block or a scenario section only when its own id is in some unresolved finding's scope closure, a line of a heading section when the line or its heading names such an id, and any line of a document or heading such a scope names (else `out_of_scope_change`); at least one document changed (else `no_change`); and the delta input is shorter than the full verify input (else `no_saving`). The first failed condition SHALL select the full verify input with that reason.

**REQ-RR-DELTA-04**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL build the delta input (contract `spec_review_delta.v1`) as a request complete in itself, because no reviewer can recall a baseline or an earlier conversation: the review purpose, the `## SPEC:` header, the baseline run and revision, a statement that the request is complete and that a finding whose status depends on text not in it stays open naming the source, the delta scope ids, a manifest naming every required source with its source digest, its prompt digest, and its delivery (`included`, `changed`, or `linked` with its reason), a unified diff of the delivered representation of each changed document, the full current text of every attribution unit a change touches, of every boundary section, and of every scope section, the traceability rows naming a delta scope id, the full text of every non-document required source, per unresolved finding the changes in its scope closure or the line `no change touches this finding`, the review policy section, and the verify instructions with the prior findings; a `linked` source SHALL be a carried document whose source and prompt digests equal the baseline's, which the baseline round delivered `complete`, and all of whose boundary and scope sections the request carries.

**REQ-RR-DELTA-05**
Priority: Must
Type: Unwanted
IF the delta manifest omits a required source, declares a digest other than its current source or prompt digest, links a source that is not a carried document, differs from the baseline, or was not delivered `complete` by the baseline round, marks a source changed without carrying its diff, leaves out a touched, boundary, or scope section or a traceability row of a carried document, or marks a source included without carrying its full delivered text, THEN THE SYSTEM SHALL discard the delta input and use the full verify input with reason `integrity_gap`.

**REQ-RR-DELTA-06**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record for every round its input (`delta` or `full`) and the reason (`discover`, `configured_full`, `delta`, or the fallback reason) in the receipt's `rounds` entries, SHALL print it before the round's calls when `verify_input` is `delta`, and SHALL reserve every call of a fallback round from the same session budget.

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
- `autopus.yaml` gains the optional keys `spec.review_gate.judge_mode`, `spec.review_gate.verify_input`, and `spec.review_gate.pinned_models`; the default configuration writes none of them.
- `review-receipt.json` (`spec_review_promotion_receipt.v1`, additive and omitted when empty) gains `judge.mode`, `judge.decisions`, `fingerprint` (with `config` and `delivery`), `spec_fingerprint`, `rounds[].input`, `rounds[].input_reason`, and `session.forced_review`.
- `review-findings.json` (additive) gains `baseline` with `delivered` and per-document `source_sha256`, `prompt_sha256`, and sanitized `content`; each finding gains `Sources`.
- `review-session.json` attempts gain the end reason `reused` and `forced_review`.
- `docs/spec-review-policy.md` and the `docs/README.ko.md` rows describe the three features. No template, generated root, or new SPEC-directory file changes.

## Related SPECs

- SPEC-REVIEWPOLICY-001: the policy, plan, session budget, call reasons (`adjudication`, `verify`), and receipt this SPEC extends.
- SPEC-ADK-REVIEW-INTEGRITY-001: the full-input integrity contract the delta input must not weaken.
- SPEC-SPECREV-001: the supermajority merge this SPEC amends for escalation items.
- SPEC-STATUSEVIDENCE-001: the review-freshness reader of `spec_fingerprint` and the helper that computes it.

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
| REQ-RR-REUSE-01 | T6, T7, T15 | S8, S10 | INV-RR-07, INV-RR-16 |
| REQ-RR-REUSE-02 | T8 | S9, S10 | INV-RR-08 |
| REQ-RR-REUSE-03 | T8 | S11 | INV-RR-08 |
| REQ-RR-REUSE-04 | T6, T8 | S10, S11 | INV-RR-08 |
| REQ-RR-REUSE-05 | T8 | S9 | INV-RR-09 |
| REQ-RR-REUSE-06 | T8 | S10 | INV-RR-08 |
| REQ-RR-REUSE-07 | T8 | S12 | INV-RR-09 |
| REQ-RR-DELTA-01 | T9 | S13 | INV-RR-10 |
| REQ-RR-DELTA-02 | T10 | S14, S16, S22 | INV-RR-10, INV-RR-12, INV-RR-15 |
| REQ-RR-DELTA-03 | T11 | S15, S16, S22 | INV-RR-11 |
| REQ-RR-DELTA-04 | T12 | S14, S21 | INV-RR-11, INV-RR-14 |
| REQ-RR-DELTA-05 | T12 | S17 | INV-RR-12 |
| REQ-RR-DELTA-06 | T13 | S16 | INV-RR-11 |
| REQ-RR-DELTA-07 | T12, T13 | S18 | INV-RR-12 |
| REQ-RR-VERIFY-01 | T14 | S19 | INV-RR-13 |
