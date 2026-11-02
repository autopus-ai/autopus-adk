# SPEC-REVIEWREUSE-001 Research

## Existing Code Analysis

Read on feat/spec-review-policy-budget 48607ed2 on 2026-11-02.

| Path | Role | Finding |
|------|------|---------|
| `internal/cli/spec_review_structured.go` `runStructuredSpecReviewOrchestra` | round runner | creates the backend once (`specReviewBackendFactory`), meters it (`meterSpecReviewBackend`), runs the reviewers in parallel, then calls `runStructuredSpecReviewJudge` whenever `JudgeProvider` is set, `NoJudge` is false, and one reviewer response is usable |
| `internal/cli/spec_review_judge.go` `runStructuredSpecReviewJudge` | judge call | anonymizes the reviewers, builds the judge prompt from the reviewer prompt's mode and prior findings, executes with role `judge`, validates sources and accepted-finding rules; returns a response or a `FailedProvider` |
| `internal/cli/spec_review_loop.go` `runSpecReviewLoop` | revision loop | per round: budget pre-check, prompt, `specReviewRunOrchestra`, parse, checklist policy, supermajority verdict and finding merge, `applySpecReviewJudge`, repeat and scope lock, deterministic findings, lean omissions, `resolveReviewVerdict`, persistence |
| `internal/cli/spec_review_loop_findings.go` `mergeDiscoverFindings`; `spec_review_loop_merge.go` `mergeVerifyFindings` | merge without a judge | both call `spec.MergeSupermajority` then `spec.DeduplicateFindings`; discover falls back to all findings only when nothing survived and the merged verdict is REVISE |
| `pkg/spec/findings.go` `MergeSupermajority`, `DeduplicateFindings`; `pkg/spec/merge.go` `verdictFromCounts` | merge rules | groups by category, normalized scope, description prefix; keeps `group[0]`; bypass for category `security` only, although the comment promises critical too; no provenance; any REVISE vote keeps the merged verdict at REVISE |
| `internal/cli/spec_review_loop_repeat.go` | same-input tracker | SHA-256 of the SPEC directory's `*.md` except `review.md`, per loop call only |
| `internal/cli/spec_review.go` `runSpecReviewWithOptions` | invocation | resolves the plan, collects code context, loads prior findings, opens the session, runs attempts, writes the receipt and promotes the status |
| `internal/cli/spec_review_receipt.go`, `spec_review_runtime.go` | receipt | `spec_review_promotion_receipt.v1`, additive `omitempty` fields; judge fields projected one by one |
| `pkg/spec/prompt.go` `BuildReviewPrompt`, `buildVerifyInstructions` | prompt | verify mode = full discover body + prior findings checklist; markers `## SPEC:`, `Instructions (Verify Mode)`, `#### Prior Findings Checklist` are read by the judge builder |
| `pkg/spec/prompt_context_delivery*.go`, `prompt_documents.go` | GPT path | complete documents from a verified `promptlayer` delivery; admission limit; hash checks per source |
| `pkg/spec/checklist.go` `loadChecklistForPrompt` | checklist | embedded `content/rules/spec-quality.md`, disk fallback |
| `pkg/spec/metadata_status.go` `UpdateStatus`, `rewriteSpecStatus` | status promotion | rewrites the frontmatter `status:` or the legacy `**Status**:` line of spec.md after a PASS |
| `pkg/spec/findings.go` `PersistFindingsWithCoverage`, `LoadFindingsWithCoverage` | findings sidecar | object `{findings, doc_coverages}`; legacy array accepted; `encoding/json` ignores unknown keys |
| `internal/cli/spec_review_plan.go`, `spec_review_plan_print.go` | plan | `JudgeMode` is `none` or `always`; call bounds count one judge call per round |
| `internal/cli/spec_review_session.go`, `spec_review_ledger*.go` | session and ledger | attempts with end reasons; every call reserved through the metered backend; reasons `review`, `verify`, `json_repair`, `provider_fallback`, `adjudication` |
| `pkg/orchestra/schema_types.go` `JudgedFinding` | judge output | `id`, `decision` (`accept`, `reject`, `merge`), `sources`, `reason`, `merge_into` |
| `pkg/orchestra/types.go` `ProviderConfig` | provider identity | `Name`, `Backend`, `Model`, `Binary`, `Args`, `ModelPolicy` |
| `pkg/orchestra/backend.go` `ProviderRequest`; `internal/cli/orchestra_readonly_policy.go` | request boundary | a request carries provider, prompt, schema path, role, round, timeout, and config, and nothing that restores a conversation; reviewers run Claude with `--no-session-persistence` and Codex with `--ephemeral`, so every request is a fresh session (re-read on 27297e2a, codex round 1 F-001) |
| `pkg/promptlayer/context_scan.go` `SanitizeContent`; `pkg/spec/prompt_documents.go` | sanitization | the complete input redacts secrets to `[REDACTED_SECRET]`, neutralizes injection markers in place, and keeps source and prompt digests apart (codex round 1 F-002) |
| `internal/cli/spec_review_context_scope.go` `requireCompleteGPTReviewDocuments` | input path | the complete, context-delivery input is used only when every reviewer is a GPT provider; otherwise the inline prompt compacts auxiliary documents beyond the budget |
| `internal/cli/spec_fingerprint.go` `specDocumentsFingerprint` (SPEC-STATUSEVIDENCE-001, #253) | SPEC fingerprint | the one rule status compares a receipt's `spec_fingerprint` with; the review writer calls it |

Clarification Ledger unavailable: this SPEC comes from the improvement brief and a team-lead handoff, not from a brainstorm file or a plan intent ledger. The brief is untrusted prompt input evidence; it is summarized and none of its commands are run.

## Outcome Lock

- User-visible outcome: with `judge_mode: on_disagreement` or `on_escalation` a round whose reviewers agree on every item makes no judge call; no mode drops a minority critical, security, or data-loss claim; re-running `auto spec review` on unchanged input returns the previous PASS (only from dated or declared models), REJECT, or awaiting-changes result with zero calls and says so, while any input, configuration, delivery, or contract change or damaged evidence triggers a paid review and names why; with `verify_input: delta` a local fix is verified from a sanitized request complete in itself, and anything the delta cannot account for falls back to the full input; the receipt records the `spec_fingerprint` status reads.
- Mandatory requirements: REQ-RR-SCOPE-01 through REQ-RR-VERIFY-01 in `spec.md`.
- Explicit non-goals: reviewer-scoped verification, cross-SPEC or remote caches, default budget numbers, changes outside the owned paths.
- Completion evidence: S1 to S20 of `acceptance.md` and the verification table in `plan.md`.

## Visual Planning Brief

`plan.md` carries the Mermaid flow. In prose: the plan fixes the judge mode and verify input; the session opens; the reuse decision compares fingerprints before anything is probed; a paid run chooses delta or full input per verify round, calls the reviewers, decides whether the judge runs, merges deterministically when it does not, and writes the findings with a baseline and the receipt with a fingerprint.

## Probe Evidence (2026-11-02)

- A1, PASS. `rg -n` for `runStructuredSpecReviewJudge(`, `meterSpecReviewBackend(`, and `specReviewBackendFactory(` in `internal/cli` without tests printed the definitions plus `spec_review_structured.go:42` (factory), `:49` (meter), `:76` (judge call): one production call site each, so a second caller can create and meter the backend the same way.
- A2, PASS. In a temporary repository with `.gitignore` listing `ignored.txt`, a committed `tracked.go` then modified, a new `new.go`, and `ignored.txt`: `git rev-parse HEAD` printed the commit, and `git status --porcelain=v1 -z --untracked-files=all` printed ` M tracked.go` and `?? new.go` NUL-separated, without the ignored file (git 2.50.1).
- Merge baseline, observed. With reviewers claude, codex, gemini, no judge, and threshold 0.67 on the fake backend: a gemini PASS vote carrying `[critical][correctness]` or `[minor][data_loss]` beside two clean PASS votes resolved PASS with no finding; claude and codex raising `[major][completeness] REQ-002` beside a gemini `[critical][correctness] REQ-001` resolved REVISE with only the REQ-002 finding. A gemini REVISE vote carrying a lone finding kept it, because any REVISE vote keeps the round at REVISE and the empty merge falls back to every finding.
- A3, PASS. A scratch module with a `replace` directive to this worktree loaded SPEC-REVIEWPOLICY-001 and built its discover prompt twice: equal within the process, 109146 bytes, and SHA-256 `87b5909c...` in both of two separate processes.
- A4, PASS (codex round 1). `ProviderRequest` has no conversation or session field, and a fake run's codex request argv holds `--ephemeral` (Claude's holds `--no-session-persistence`); the pre-fix delta request, run in a scratch worktree, lacked the context documents, the code context, and every section of a linked document, so a linked source never reached the reviewer.

## Trust Boundary

Provider output stays untrusted, as in SPEC-REVIEWPOLICY-001. The judge decision reads parsed reviews only to decide whether to spend a call; it never clears a blocker, and a skipped or failed judge leaves the deterministic merge, which keeps every escalation claim. Reuse trusts nothing it cannot recompute: the receipt is parsed with its schema, every input is re-hashed, the evidence files must match their recorded digests, and a damaged or partial record means a paid review, never an approval. The receipt and the findings sidecar are local files; a user able to forge a matching pair can already edit the SPEC status directly, so local forgery is outside the threat model, and reuse never changes the status. Being an ignored file does not protect a secret, so the baseline stores only the delivered representation (`SanitizeContent`, as the complete input): a secret is `[REDACTED_SECRET]` in the baseline and in any diff line that removes it, and an injection marker stays neutralized; the source digest proves integrity without storing the source. A delta request never asks a reviewer to recall anything: each request is a fresh session.

## Design Decisions

- **Conditional judge is opt-in; the merge fix is not.** `always` stays the default so existing behaviour and cost are unchanged until an operator chooses a mode. The supermajority merge dropping a minority critical or data-loss claim is a defect against its own documented rule, so the fix applies everywhere.
- **Decide on parsed reviews, call through the existing judge path.** The alternative, a new orchestra strategy, would touch `pkg/orchestra`; the chosen two-phase round keeps the judge prompt, validation, metering, and receipt projection unchanged.
- **Escalation is narrower than blocking.** `on_escalation` arbitrates only contested REJECT, critical, security, data-loss, and escape-hatch claims; ordinary disagreements resolve by the existing supermajority, which never drops an escalation claim after this SPEC.
- **Fingerprint by component, contract by probe.** Per-component digests let the refusal line name the change; digesting prompts built for a fixed probe SPEC versions the prompt and schema text automatically.
- **Reuse blocking results freely, PASS only with pinned models.** Returning a REJECT or awaiting-changes result is safe because it approves nothing; a PASS is an approval, so it is reused only when every model selector names one immutable model: a dated snapshot, or a selector the operator declares in `pinned_models`. A digit is not a version (`gpt-4o` is an alias of a dated snapshot). The 168-hour limit bounds how long any result stands in for a review.
- **Fingerprint what is delivered, not what is configured in one place.** The effective configuration is digested whole because a file outside the repository never reaches the git component; the excerpts the full input injects are digested because a probe SPEC cannot show where the actual SPEC is compacted; the contract runs every prompt variant through the production wrapper.
- **Normalize the status line, exclude review artifacts.** The status promotion and the review files change with every run; hashing them would make every result unreusable.
- **Delta verify is opt-in, ordered, fail-closed, and self-contained.** Every eligibility condition has a named fallback to the unchanged full builder. Because no reviewer keeps a session, "linked" can only mean "not needed": an unchanged carried document that the baseline round delivered whole and whose sections naming a delta scope id, with its boundary sections, are in the request; every other required source is sent in full, and the manifest check proves each claim. Attribution is per requirement block or scenario, so a sibling block never rides on a neighbour's id. "Delivered whole" is read from the request a round actually sent, so a delta round's baseline is all `partial` and delta rounds never chain: the verify round after one reviews the full input.
- **Baseline inside the findings sidecar, sanitized.** The baseline describes what the open findings were raised against; storing it there needs no new SPEC-directory file, ignore pattern, or status classification, and it holds the representation a request delivers, never the raw source.

## Minimality Decision Matrix

| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | brief R8, R9, R11; judge on consensus, dropped minority critical claim, repeated paid rounds, full-text verify reproduced on 48607ed2 | proceed | conditional judge, reuse, delta verify |
| existing code/helper/pattern | `runStructuredSpecReviewJudge`, metered backend, `MergeSupermajority`, repeat-tracker digest, receipt `omitempty` pattern, findings sidecar, verify instructions | reuse | no new judge path, ledger, or receipt schema version |
| stdlib/native | `crypto/sha256`, `encoding/json`, `os/exec` for git, `strings` line diff | use | no diff library |
| existing dependency | `cobra` flags, `yaml.v3`, `testify` | reuse | no dependency added |
| new dependency or new abstraction | judge decision, fingerprint, reuse decision, baseline, delta builder; nothing owns these today | accepted | new files listed in `spec.md` ownership |
| minimum sufficient verification | S1 to S20 with fake backends, race run, unchanged full-path tests, package runs | required checks | security, validation, data-loss, deterministic-oracle gates unchanged |

## Semantic Invariant Inventory

Source clauses summarize the brief, which is untrusted input.

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-RR-01 | explicit judge policies, compatible default | configuration contract | config, flag, plan lines | S1 |
| INV-RR-02 | `always` keeps existing behaviour | equality with baseline | calls, verdict, receipt | S2 |
| INV-RR-03 | consensus without blockers merges deterministically; conflicts are arbitrated per policy | decision table | judge calls, status, reason | S3, S4 |
| INV-RR-04 | minority safety claims never removed by majority PASS; provenance kept | conservation | merged findings, verdict | S5 |
| INV-RR-05 | judge absence, failure, or budget never turns risk into PASS | fail-closed | verdict, blockers | S6 |
| INV-RR-06 | judge reasons linked to original findings | record completeness | receipt decisions | S7 |
| INV-RR-07 | fingerprint covers documents, code, references, checklist, policy, prompt and schema, providers, settings | coverage | receipt fingerprint | S8 |
| INV-RR-08 | reuse only on identical, intact, reusable input; any change invalidates | equivalence | reuse decision, refusal line | S10, S11 |
| INV-RR-09 | reuse makes no call and is never shown as a new review; forced review charged | zero-call and accounting | calls, files, session | S9, S12 |
| INV-RR-10 | baseline hashes, policy, unresolved ids preserved | record completeness | findings sidecar | S13, S14 |
| INV-RR-11 | delta carries diff, affected requirements and scenarios, fix evidence; falls back on named conditions | decision table | prompt, input, reason | S14, S15, S16 |
| INV-RR-12 | every required context included or linked to a verified baseline; full path unchanged | integrity | validation, prompts | S17, S18, S22 |
| INV-RR-13 | verification and scope | conservation | commands, paths, sizes | S19, S20 |
| INV-RR-14 | a stateless reviewer judges a delta request from its text alone | self-containment | delta prompt, manifest | S14, S21 |
| INV-RR-15 | the baseline and the delta never carry what the complete input withholds | redaction | baseline, diff, sections | S14, S16 |
| INV-RR-16 | status reads the SPEC fingerprint the review recorded | record completeness | receipt `spec_fingerprint` | S8 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| Judge mode setting, decision, two-phase round, records | T1 to T3, T5, S1 to S4, S6, S7 | covered |
| Deterministic merge with provenance | T4, S5 | covered |
| Fingerprint, reuse, invalidation, force, pinned models, spec fingerprint | T6 to T8, T15, S8 to S12 | covered |
| Verify input setting, baseline, delta eligibility, self-contained sanitized prompt, integrity, round records | T9 to T13, S13 to S18, S21 | covered |
| Reviewer-scoped verification | explicit non-goal | out of scope by Outcome Lock |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Verify a finding first with the reviewer that raised it | The brief asks to evaluate it after delta verify exists | Delta rounds show reviewers re-checking findings they did not raise |
| Make `on_disagreement` the default | The brief starts new judge policies as opt-in | Ledger data shows equal verdict quality with fewer calls |
| Configurable reuse age | 168 hours is a conservative fixed bound | A user needs a different window |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | One change story: stop paying for review calls that cannot change the outcome | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| every path and symbol in `## Existing Code Analysis` | existing | read on 2026-11-02 at 48607ed2; call sites confirmed with `rg` (A1) |
| `git rev-parse`, `git status --porcelain=v1 -z --untracked-files=all` | existing tool | executed in a temporary repository (A2) |
| `[NEW] pkg/config/schema_review_modes.go`; `[NEW] internal/cli/spec_review_judge_mode*.go`, `spec_review_fingerprint*.go`, `spec_review_reuse*.go`, `spec_review_delta*.go`; `[NEW] pkg/spec/review_merge*.go`, `review_fingerprint*.go`, `review_baseline*.go`, `review_delta*.go`; `[NEW]` flags `--judge-mode`, `--force-review`, `--verify-input`; `[NEW]` keys `judge_mode`, `verify_input`, `pinned_models`; `[NEW]` receipt `fingerprint` (`config`, `delivery`), `spec_fingerprint`, `judge.mode`, `judge.decisions`; `[NEW]` sidecar `baseline` (`delivered`, `source_sha256`, `prompt_sha256`) | planned addition | added on this branch |

## Reviewer Brief

- Intended scope: R8, R11, and R9 of the brief as a conditional judge, cross-invocation reuse, and delta verify for `auto spec review`.
- Explicit non-goals: reviewer-scoped verification, shared caches, default budgets, generated roots, anything outside the owned paths.
- Self-verified: Traceability Matrix over 23 requirements, thirteen invariants each mapped to a Must oracle, three executed probes, Reference Discipline separating existing and `[NEW]` references.
- Reviewer should focus on: whether any skip, reuse, or delta path can produce a PASS that a full `always` review would block; whether the fingerprint misses an input a reviewer reads; whether a delta request leaves out anything a fresh reviewer needs, or carries anything the complete input withholds; whether the merge change silently alters verdicts beyond escalation claims.

## Review Response (codex round 1)

| Finding | Severity | Decision | Change | Evidence |
|---------|----------|----------|--------|----------|
| F-001 linked context never reaches a stateless reviewer | major | accept | Definitions (required source, delivered scope, scope and boundary sections), REQ-RR-DELTA-02 to 05, S14, S16, S17, S21; the request carries every needed section and required source, `linked` means unneeded, `incomplete_baseline` | `TestRunSpecReview_DeltaRequestIsSelfContainedForAStatelessReviewer`, `TestRunSpecReview_IncompletelyDeliveredBaselineIsNeverLinked`, `TestBuildDeltaVerifyPrompt_IsSelfContainedForAStatelessReviewer` |
| F-002 baseline and diff bypass sanitization | major | accept | delivered representation and source/prompt digests in REQ-RR-DELTA-02 to 05, Trust Boundary, S14; `integrity_gap` for a change the redaction hides | `TestRunSpecReview_DeltaBaselineAndRequestStaySanitized`, `TestDeltaVerify_DeletedSecretAndInjectionStaySanitized` |
| F-003 contested compared per reviewer | major | accept | item keys in Definitions, REQ-RR-JDG-04, S3 and S4 rows, T2 | `TestDecideSpecReviewJudge_ContestedIsDecidedPerItem`, S3 and S4 tables |
| F-004 a digit taken for a version | major | accept | pinned = dated snapshot or `pinned_models`, REQ-RR-REUSE-04, S8 to S11 | `TestSpecReviewModelSelectorPinned`, `TestReviewGatePinnedModels_LoadAndValidate`, `TestRunSpecReview_UnpinnedModelBlocksOnlyAReusedPass` |
| F-005 fingerprint misses delivery settings and prompt variants | major | accept | `config`, `delivery`, and the full contract in REQ-RR-REUSE-01, S8, S10 | `TestSpecReviewContract_CoversEveryProductionPromptVariant`, `TestRunSpecReview_WrapperOnlyChangeInvalidates`, `TestRunSpecReview_ExternalConfigChangeInvalidates`, `TestSpecReviewFingerprint_RecordsTheDeliveredExcerpts` |
| F-006 section-level change attribution | major | accept | attribution unit in Definitions and REQ-RR-DELTA-03, S15 with REQ-001 to REQ-003 under one H2 | `TestPlanDeltaVerify_FallsBackOnTheFirstFailedCondition` (sibling block naming REQ-002) |
| F-007 call counts ignore the default judge | major | accept | every reuse and delta Given says "no judge"; the tests already ran without a judge, so no code changed | S9 to S12, S14 to S18, S21 Givens |

## Review Response (codex round 2)

| Finding | Severity | Decision | Change | Evidence |
|---------|----------|----------|--------|----------|
| F-001 a delta round's baseline claims a whole delivery | major | accept | delivered scope is read from the request a round actually sent; a delta request records every document `partial`, so the next verify reviews the full input (Definitions, REQ-RR-DELTA-02, T10, S22) | `TestRunSpecReview_DeltaRoundBaselineNeverClaimsAWholeDelivery` (round 1 recorded every document `complete` and round 2 linked spec.md before the fix) |
| F-003 S4 mixes in on_disagreement oracles | major | accept | S3 carries the ordinary blocking-item and `consensus_block` oracles; S4 states `no_escalation`, `unanimous_escalation`, and a contested escalation only; no code change, the decision already compared only escalation items under `on_escalation` | `TestDecideSpecReviewJudge_OnDisagreementComparesBlockingItems`, `TestDecideSpecReviewJudge_OnEscalationComparesOnlyEscalationItems` |

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 2 | files: research.md, spec.md | reason: every existing path and symbol was read at 48607ed2 and the request boundary, sanitization, and input-path rows re-read at 27297e2a; probe results recorded
- Q-CORR-02 | status: PASS | attempt: 2 | files: spec.md, plan.md, research.md | reason: new files, flags, keys, and receipt fields carry `[NEW]`, `pinned_models`, `config`, `delivery`, and `spec_fingerprint` included
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Event-driven requirements read `WHEN ..., THEN`, Unwanted `IF ..., THEN`, State-driven `WHILE`; scenarios use `### S<N>: AC-...`
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing references from planned additions
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: spec fixes the contract, plan the tasks and probes, acceptance the oracles, research the evidence and decisions
- Q-COMP-02 | status: PASS | attempt: 2 | files: spec.md, plan.md, acceptance.md | reason: all 23 requirements appear in the matrix with a task and a scenario, S21 and T15 included
- Q-COMP-03 | status: PASS | attempt: 1 | files: spec.md | reason: each requirement names its observable: call count, judge status, verdict, printed line, JSON field, chosen input, or error text
- Q-COMP-04 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: Outcome Lock covers R8, R11, R9 end to end, a self-contained sanitized delta, a baseline that never claims a whole delivery its request did not make, and pinned-only PASS reuse included; reviewer-scoped verification is a named non-goal
- Q-COMP-05 | status: PASS | attempt: 3 | files: research.md, spec.md, plan.md, acceptance.md | reason: INV-RR-01 to INV-RR-16 each map to a requirement, a task, and a Must oracle; contested per mode (S3 on_disagreement, S4 on_escalation), attribution, delivered scope across rounds (S22), and call counts agree with their oracles
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix complete; Reviewer Brief names four focus areas and the non-goals
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt empty; Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md, research.md | reason: A1, A2, and A4 PASS in the probe table with evidence refs, A3 recorded here and pinned by a test; the findings-sidecar compatibility of the new `baseline` key is covered by the T10 test
- Q-FEAS-01 | status: PASS | attempt: 2 | files: plan.md | reason: settings in `pkg/config`, merge, fingerprint, baseline, and delta in `pkg/spec`, decisions and integration in `internal/cli`; no reviewer state is assumed across requests
- Q-FEAS-02 | status: PASS | attempt: 1 | files: plan.md | reason: no template or generated file changes; the judge prompt is built by the existing renderer
- Q-FEAS-03 | status: PASS | attempt: 2 | files: plan.md, acceptance.md | reason: every scenario runs with fake backends in temporary directories and repositories, and its call count follows its stated judge
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: no should, might, could, possibly, maybe, perhaps in requirement text
