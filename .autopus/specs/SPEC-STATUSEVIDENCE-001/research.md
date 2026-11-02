# SPEC-STATUSEVIDENCE-001 Research

## Existing Code Analysis

Read on 2026-11-02 at `222c9e9b` (PR #251 tip; this branch stacks on it).

- `internal/cli/status.go`, `status_json.go`, `status_scan.go`: scan SPEC directories, count `done`, `completed`, and `implemented` as done, and print `SPEC이 없습니다. /auto plan으로 시작하세요.` without SPECs; JSON carries `summary`, `specs`, `findings`, `hygiene`.
- `internal/cli/pipeline_run.go`: prints `Pipeline complete: %d phases executed` with `len(result.PhaseResults)`, restored phases included.
- `internal/cli/pipeline_run_flags.go`: `resolvePlatform` returns the flag, else the first of `claude`, `codex`, `agy` on `PATH`, else `claude`.
- `internal/cli/init.go`: without `~/.config/autopus/worker.yaml` the only next step is `open Autopus Desktop Connect or run 'autopus-desktop-runtime connect'`; with it, Codex users get the retired `$auto plan` instead of `$codex-auto-plan`.
- `internal/cli/pipeline.go`: `--continue` loads `.autopus/pipeline-state/<SPEC>.yaml` and refuses a checkpoint whose git hash differs from HEAD.
- `internal/cli/pipeline_run_owner.go`, `pipeline_run_integrity.go`: per-SPEC JSON sidecars in the pipeline state directory, mode 0600.
- `internal/cli/runtime_helper_delegate.go`: `resolveRuntimeHelper` finds the Desktop runtime helper.
- `internal/cli/spec_review_receipt.go`, `spec_review_runtime.go`, `spec_review.go`: `review-receipt.json` fields; a passing review promotes `spec.md` after `finished_at` is taken and before the receipt is written; PR #252 (`feat/spec-review-policy-budget`, read-only) defines `review-session.json` `spec_review_session.v1`.
- `pkg/pipeline/checkpoint.go`, `receipt.go`, `receipt_lineage.go`: checkpoint validation and the receipt fields status reads.

## Outcome Lock

- User-visible outcome: a user can tell from `auto status` which SPECs were verified by a run whose evidence still matches the code, which only claim completion, which failed and can be resumed, and what the last review said; the pipeline's result line separates restored from executed phases; the suggested next commands fit what is installed, lead with standalone use, and state where they run and what they call and write.
- Mandatory requirements: REQ-SEV-SCOPE-01 through REQ-SEV-VERIFY-01 in `spec.md`.
- Explicit non-goals: a new dashboard or state store, changing platform selection, writing review receipts, Desktop detection in init, model or server calls from status.
- Completion evidence: S1 to S12, the TUIUX re-record, and the source-lines gate.

## Visual Planning Brief

`plan.md` carries the Mermaid flow. In prose: status reads each SPEC's declared state, its checkpoint and sidecars, its verification baseline, and its review files, labels the evidence without merging them, and derives next actions from the labels plus installed CLIs, configured platforms, and Desktop facts; `auto pipeline run` writes the baseline and prints honest counts.

## Design Decisions

- Freshness compares a working-tree fingerprint against the run's base commit, not HEAD alone: committing exactly the verified tree keeps the evidence fresh, while any edit, new file, or pulled change makes it stale (probe B1). A HEAD-only rule would call every committed verification stale and miss uncommitted edits.
- The baseline is one sidecar next to the existing pipeline receipts, following `execution-owner.json` and `tier-integrity.json`; it is not a new store.
- The review receipt has no code baseline or content fingerprint, so review staleness compares SPEC document modification times with the receipt file's own time, not `finished_at`: the review's promotion of `spec.md` happens after `finished_at` and would otherwise make every passing review stale at once. It errs toward stale, a later status-only edit included.
- The SPEC fingerprint drops header status lines because review promotion and `/auto sync` rewrite them; without that, every synced SPEC would read stale.
- The baseline keeps per-path digests so a stale reason names what changed, and excludes everything under `.autopus`, which is harness state such as the run's own checkpoint.
- A resume command pins the run's platform, so a resume does not switch providers because `PATH` changed.
- Resumability is predicted from HEAD and the checkpoint checks the CLI and engine run first; output-record verification stays at resume time, where its refusal is already explicit.
- Init prints static standalone-first text so goldens stay deterministic across machines; status probes the Desktop helper through an injectable function.

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-S01 | a document claim is never execution evidence | label separation | evidence label, verified count | AC-SEV-001, AC-SEV-002 |
| INV-S02 | evidence is stale once what it judged changed | freshness rule | verification and review state | AC-SEV-002, AC-SEV-009 |
| INV-S03 | restored phases are not executed phases | count separation | result line, phase counts | AC-SEV-003, AC-SEV-006, AC-SEV-011 |
| INV-S04 | resumable means `--continue` would be admitted | prediction rule | resumable flag, reason | AC-SEV-003 |
| INV-S05 | next commands fit installed CLIs, configured platforms, and Desktop state | applicability | next actions | AC-SEV-004, AC-SEV-007, AC-SEV-008, AC-SEV-010 |
| INV-S06 | status writes nothing, calls nothing, runs nothing | side-effect freedom | project tree, auto_run | AC-SEV-012 |
| INV-S07 | text and JSON carry the same meaning | parity | both outputs | AC-SEV-005 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---|---|---|
| Declared versus executed and verified | T1, T3, S1, S5 | covered |
| Verification baseline and staleness | T2, S2 | covered |
| Failed, resumable, refused resume | T1, S3 | covered |
| Review evidence | T1, S9 | covered |
| Pipeline counts and platform line | T5, S6, S11 | covered |
| Next commands, providers, Desktop | T4, T6, S4, S7, S8, S10 | covered |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Record an input fingerprint in the review receipt | Owned by the review engine work package | That work lands |
| Let platform selection prefer configured platforms | Changes pipeline behaviour, not its reporting | Users are routinely surprised by the first-installed rule |
| A `--spec` filter for status | Not needed for correctness | Large monorepos |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `internal/cli/status.go`, `status_json.go`, `status_scan.go`, `pipeline_run.go`, `pipeline_run_flags.go`, `pipeline.go`, `init.go`, `runtime_helper_delegate.go`, `pipeline_run_owner.go`, `spec_review_receipt.go` | existing | read on 2026-11-02 |
| `pkg/pipeline/checkpoint.go`, `receipt.go`, `receipt_lineage.go` | existing | read on 2026-11-02 |
| PR #252 `internal/cli/spec_review_session.go` | external, not on base | read through `gh pr diff 252` on 2026-11-02 |
| every `[NEW]` file in `spec.md` | added on `feat/status-execution-evidence` | `go test ./internal/cli/...` |

## Reviewer Brief

- Intended scope: R5 of the brief for status, pipeline run output, and init guidance.
- Explicit non-goals: see the Outcome Lock.
- Self-verified: probe B1 executed, traceability over 16 requirements, existing receipt and sidecar formats read.
- Reviewer should focus on: whether any path can report a SPEC as verified without a fresh baseline; whether the fingerprint misses a content change; whether a recommended command could run or mislead about calls and writes; whether status could write anything.

## Residual Risks

- Review staleness by modification time can call a review stale after a checkout that only touched timestamps; it never calls a changed SPEC fresh.
- A tree with more than 5,000 changed or untracked files, or more than 1 GiB of their content, reports verification `unknown` instead of hashing them.
- Ignored files are outside the fingerprint, and inside a submodule only its checked-out commit counts, so uncommitted submodule edits do not make verification stale.
- Next commands are suggestions: the review action assumes `auto spec review` resolves the SPEC from the status directory, and `auto connect` assumes the Desktop helper status found is the one it delegates to.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: every cited symbol, file format, and message was read on 2026-11-02
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: new files carry `[NEW]`
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: EARS lines and bare Given/When/Then steps the parser reads
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing, external, and planned references
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: contract, tasks and probes, oracles, and decisions each live in their file
- Q-COMP-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: 16 requirements map to S1–S12
- Q-COMP-03 | status: PASS | attempt: 1 | files: spec.md | reason: each requirement names its trigger and the text or JSON field that shows it
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: the Outcome Lock covers every R5 clause and the required tests
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, acceptance.md | reason: INV-S01 to INV-S07 map to Must oracles with fixed strings and counts
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix with invariants; Reviewer Brief names the focus
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt empty; Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 1 | files: plan.md | reason: B1 and B3 executed with evidence, B2 from a code read
- Q-FEAS-01 | status: PASS | attempt: 1 | files: plan.md | reason: every promise is runtime code in internal/cli
- Q-FEAS-02 | status: PASS | attempt: 1 | files: spec.md | reason: owned CLI paths only; generated roots untouched; goldens re-recorded by the writer
- Q-FEAS-03 | status: PASS | attempt: 1 | files: plan.md | reason: the listed commands exist
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: no ambiguous modal in a requirement
- Q-STYLE-02 | status: PASS | attempt: 1 | files: spec.md | reason: Priority on its own line apart from the EARS type
- Q-STYLE-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: complete sentences, bare steps
- Q-SEC-01 | status: PASS | attempt: 1 | files: spec.md | reason: receipts and review files are read as untrusted data; unparseable files degrade to unknown
- Q-SEC-02 | status: PASS | attempt: 1 | files: spec.md | reason: status prints receipt blockers already redacted by the pipeline and reads only the worker config's presence, never its contents
- Q-SEC-03 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: one 0600 sidecar holding hashes and IDs only
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md | reason: one story: what status claims and what it tells the user to do next
- Q-COH-02 | status: PASS | attempt: 1 | files: research.md | reason: nothing required is deferred
- Q-COH-03 | status: PASS | attempt: 1 | files: research.md | reason: no sibling SPEC
