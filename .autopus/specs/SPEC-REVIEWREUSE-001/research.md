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

Clarification Ledger unavailable: this SPEC comes from the improvement brief and a team-lead handoff, not from a brainstorm file or a plan intent ledger. The brief is untrusted prompt input evidence; it is summarized and none of its commands are run.

## Outcome Lock

- User-visible outcome: with `judge_mode: on_disagreement` or `on_escalation` a round whose reviewers agree makes no judge call; no mode drops a minority critical, security, or data-loss claim; re-running `auto spec review` on unchanged input returns the previous PASS, REJECT, or awaiting-changes result with zero calls and says so, while any input change or damaged evidence triggers a paid review and names why; with `verify_input: delta` a local fix is verified from its changes and linked context, and anything the delta cannot account for falls back to the full input.
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

## Trust Boundary

Provider output stays untrusted, as in SPEC-REVIEWPOLICY-001. The judge decision reads parsed reviews only to decide whether to spend a call; it never clears a blocker, and a skipped or failed judge leaves the deterministic merge, which keeps every escalation claim. Reuse trusts nothing it cannot recompute: the receipt is parsed with its schema, every input is re-hashed, the evidence files must match their recorded digests, and a damaged or partial record means a paid review, never an approval. The receipt and the findings sidecar are local files; a user able to forge a matching pair can already edit the SPEC status directly, so local forgery is outside the threat model, and reuse never changes the status. The delta prompt carries document text the full prompt already carries; the baseline stores SPEC document text in the ignored `review-findings.json` and no prompt, provider output, or secret.

## Design Decisions

- **Conditional judge is opt-in; the merge fix is not.** `always` stays the default so existing behaviour and cost are unchanged until an operator chooses a mode. The supermajority merge dropping a minority critical or data-loss claim is a defect against its own documented rule, so the fix applies everywhere.
- **Decide on parsed reviews, call through the existing judge path.** The alternative, a new orchestra strategy, would touch `pkg/orchestra`; the chosen two-phase round keeps the judge prompt, validation, metering, and receipt projection unchanged.
- **Escalation is narrower than blocking.** `on_escalation` arbitrates only contested REJECT, critical, security, data-loss, and escape-hatch claims; ordinary disagreements resolve by the existing supermajority, which never drops an escalation claim after this SPEC.
- **Fingerprint by component, contract by probe.** Per-component digests let the refusal line name the change; digesting prompts built for a fixed probe SPEC versions the prompt and schema text automatically.
- **Reuse blocking results freely, PASS only with pinned models.** Returning a REJECT or awaiting-changes result is safe because it approves nothing; a PASS is an approval, so it is reused only when every model selector names a version. The 168-hour limit bounds how long any result stands in for a review.
- **Normalize the status line, exclude review artifacts.** The status promotion and the review files change with every run; hashing them would make every result unreusable.
- **Delta verify is opt-in, ordered, and fail-closed.** Every eligibility condition has a named fallback to the unchanged full builder; the manifest check proves each required source was either sent or is byte-identical to what a usable round reviewed.
- **Baseline inside the findings sidecar.** The baseline describes what the open findings were raised against; storing it there needs no new SPEC-directory file, ignore pattern, or status classification.

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
| INV-RR-12 | every required context included or linked to a verified baseline; full path unchanged | integrity | validation, prompts | S17, S18 |
| INV-RR-13 | verification and scope | conservation | commands, paths, sizes | S19, S20 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| Judge mode setting, decision, two-phase round, records | T1 to T3, T5, S1 to S4, S6, S7 | covered |
| Deterministic merge with provenance | T4, S5 | covered |
| Fingerprint, reuse, invalidation, force | T6 to T8, S8 to S12 | covered |
| Verify input setting, baseline, delta eligibility, prompt, integrity, round records | T9 to T13, S13 to S18 | covered |
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
| `[NEW] pkg/config/schema_review_modes.go`; `[NEW] internal/cli/spec_review_judge_mode*.go`, `spec_review_fingerprint*.go`, `spec_review_reuse*.go`, `spec_review_delta*.go`; `[NEW] pkg/spec/review_merge*.go`, `review_fingerprint*.go`, `review_baseline*.go`, `review_delta*.go`; `[NEW]` flags `--judge-mode`, `--force-review`, `--verify-input`; `[NEW]` keys `judge_mode`, `verify_input`; `[NEW]` receipt `fingerprint`, `judge.mode`, `judge.decisions`; `[NEW]` sidecar `baseline` | planned addition | do not exist yet |

## Reviewer Brief

- Intended scope: R8, R11, and R9 of the brief as a conditional judge, cross-invocation reuse, and delta verify for `auto spec review`.
- Explicit non-goals: reviewer-scoped verification, shared caches, default budgets, generated roots, anything outside the owned paths.
- Self-verified: Traceability Matrix over 23 requirements, thirteen invariants each mapped to a Must oracle, three executed probes, Reference Discipline separating existing and `[NEW]` references.
- Reviewer should focus on: whether any skip, reuse, or delta path can produce a PASS that a full `always` review would block; whether the fingerprint misses an input a reviewer reads; whether the delta eligibility order and the manifest check leave a required source unaccounted for; whether the merge change silently alters verdicts beyond escalation claims.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 1 | files: research.md, spec.md | reason: every existing path and symbol was read at 48607ed2; probe results recorded
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md, research.md | reason: new files, flags, keys, and receipt fields carry `[NEW]`
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Event-driven requirements read `WHEN ..., THEN`, Unwanted `IF ..., THEN`, State-driven `WHILE`; scenarios use `### S<N>: AC-...`
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing references from planned additions
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: spec fixes the contract, plan the tasks and probes, acceptance the oracles, research the evidence and decisions
- Q-COMP-02 | status: PASS | attempt: 1 | files: spec.md, plan.md, acceptance.md | reason: all 23 requirements appear in the matrix with a task and a scenario
- Q-COMP-03 | status: PASS | attempt: 1 | files: spec.md | reason: each requirement names its observable: call count, judge status, verdict, printed line, JSON field, chosen input, or error text
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Outcome Lock covers R8, R11, R9 end to end; reviewer-scoped verification is a named non-goal
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, spec.md, plan.md, acceptance.md | reason: INV-RR-01 to INV-RR-13 each map to a requirement, a task, and a Must oracle
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix complete; Reviewer Brief names four focus areas and the non-goals
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt empty; Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 1 | files: plan.md, research.md | reason: A1 to A3 PASS with evidence refs; the findings-sidecar compatibility of the new `baseline` key is covered by the T10 test
- Q-FEAS-01 | status: PASS | attempt: 1 | files: plan.md | reason: settings in `pkg/config`, merge, fingerprint, baseline, and delta in `pkg/spec`, decisions and integration in `internal/cli`
- Q-FEAS-02 | status: PASS | attempt: 1 | files: plan.md | reason: no template or generated file changes; the judge prompt is built by the existing renderer
- Q-FEAS-03 | status: PASS | attempt: 1 | files: plan.md, acceptance.md | reason: every scenario runs with fake backends in temporary directories and repositories
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: no should, might, could, possibly, maybe, perhaps in requirement text
