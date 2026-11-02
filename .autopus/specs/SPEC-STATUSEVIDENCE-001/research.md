# SPEC-STATUSEVIDENCE-001 Research

## Existing Code Analysis

Read on 2026-11-02 on `feat/status-execution-evidence`, stacked on PR #251 (`bdd14afc`).

- `internal/cli/status.go`, `status_json.go`, `status_scan.go`: scan SPEC directories, count `done`, `completed`, and `implemented` as done, and sent an empty project to `/auto plan` only; JSON carries `summary`, `specs`, `findings`, `hygiene`.
- `internal/cli/pipeline_run.go`: printed `Pipeline complete: %d phases executed` with `len(result.PhaseResults)`, restored phases included; dry runs persisted to the canonical checkpoint path.
- `internal/cli/pipeline_run_flags.go`: `resolvePlatform` returns the flag, else the first of `claude`, `codex`, `agy` on `PATH`, else `claude`, even when none is installed.
- `internal/cli/init.go`: without `~/.config/autopus/worker.yaml` the only next step was the Desktop connection; with it, Codex users got the retired `$auto plan`.
- `internal/cli/pipeline.go`: `--continue` refuses a checkpoint whose git hash differs from HEAD.
- `pkg/pipeline/checkpoint_restore.go`, `phase_output.go`, `engine_run.go`: `planResume` validates identity, version, snapshot, dependency closure, and route, verifies output records, and refuses when a writing phase cannot be restored; it reads nothing beyond the checkpoint. Dispatches start with in-progress transitions of attempt one or more; restores record attempt zero.
- `internal/cli/pipeline_run_owner.go`: the owner sidecar is rewritten before admission; `pipeline_backend_orca.go` and `pipeline_backend_omp.go` record backends `orca` and `rpc-*` per dispatch.
- `internal/cli/spec_review_receipt.go`, `spec_review_runtime.go`, `spec_review.go`: the receipt has no content fingerprint, and a passing review promotes `spec.md` after `finished_at` is taken.
- `internal/cli/runtime_helper_delegate.go`: `resolveRuntimeHelper` finds the Desktop helper without `PATH`.

## Outcome Lock

- User-visible outcome: a user can tell from `auto status` which SPECs were verified by a run whose evidence still matches the code, which only claim completion, which stopped and whether `--continue` would take them, and what the last review said; dry runs no longer erase that evidence; the pipeline's result line separates restored from executed phases; the suggested next commands fit what is installed, lead with standalone use, carry only validated values, and state their directory, place, provider, calls, and writes.
- Mandatory requirements: REQ-SEV-SCOPE-01 through REQ-SEV-VERIFY-01 in `spec.md`.
- Explicit non-goals: a new dashboard or state store, changing platform selection, changing what `auto spec review` writes, Desktop detection in init, model or server calls from status.
- Completion evidence: S1 to S18, the TUIUX re-record, and the source-lines gate.

## Visual Planning Brief

`plan.md` carries the Mermaid flow. In prose: status picks the newest state root for each SPEC, reads its checkpoint, asks the engine whether `--continue` would be admitted, validates and compares the verification baseline, reads the review files, labels the evidence without merging it, and derives next actions from the labels plus installed CLIs, pipeline platforms, configured platforms, and Desktop facts.

## Design Decisions

- Resumability is the engine's own decision. `pipeline.AdmitResume` runs `planResume` without dispatching or writing, after the CLI's HEAD check, so output-record verification is part of the answer instead of deferred; probe B2 runs the real `--continue --dry-run` against eight conditions.
- Executed and restored phases are counted by what this run dispatched, from the receipt's transitions, because `resume.restored_phases` keeps a phase that a routed fix later dispatched again.
- The stop reason is the decision that ended the run; remediate and transport retries never end one, so a persistence failure after a routed fix reads `partial_preserved`.
- Freshness compares a working-tree fingerprint against the run's base commit, not HEAD alone, so committing the verified tree keeps the evidence fresh. Files git skips (assume-unchanged, skip-worktree) make the tree unverifiable rather than unchanged, and `core.fileMode` is forced on so executable-bit changes count.
- A baseline is evidence only when every field is present and well-formed and its tree paths add up to its fingerprint.
- Review freshness needs a recorded fingerprint. The rule lives in one helper, `specDocumentsFingerprint` in `internal/cli/spec_fingerprint.go`, which the verification baseline uses and the review receipt writer of R11 will call, so the writer and the reader cannot drift; a golden test pins its value. Until a receipt carries `spec_fingerprint`, status reports the review stale when a document was written after the receipt and unknown otherwise; mtime never proves a SPEC unchanged.
- Dry runs persist under `.autopus/pipeline-state/dry-run/`: the engine writes wherever the run's checkpoint directory points, so no engine change is needed.
- The execution owner comes from the run's dispatch backends because the sidecar is rewritten before admission.
- Commands carry only values that pass the pipeline's SPEC ID pattern, the platform allowlist, and the owner allowlist; directories are absolute and shell-quoted; strings from receipts lose control characters.
- Status sees the machine through an injected probe, so tests never depend on the developer's `PATH`, Desktop install, or home.

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-S01 | a document claim or a dry run is never execution evidence | label separation | evidence label, verified count, checkpoint | AC-SEV-001, AC-SEV-002, AC-SEV-015 |
| INV-S02 | evidence is stale or unknown once what it judged changed or cannot be compared | freshness rule | verification and review state | AC-SEV-002, AC-SEV-009, AC-SEV-014 |
| INV-S03 | restored phases are not executed phases | count separation | result line, phase counts | AC-SEV-003, AC-SEV-006 |
| INV-S04 | resumable means `--continue` admits it | prediction rule | resume admission, command | AC-SEV-003, AC-SEV-016 |
| INV-S05 | next commands fit installed CLIs, pipeline platforms, configured platforms, Desktop, and the evidence root | applicability | next actions | AC-SEV-004, AC-SEV-007, AC-SEV-008, AC-SEV-010, AC-SEV-017, AC-SEV-018 |
| INV-S06 | status writes nothing, runs nothing but read-only git, runs no recommended command | side-effect freedom | project tree, executions | AC-SEV-012 |
| INV-S07 | text and JSON carry the same meaning | parity | both outputs | AC-SEV-005 |
| INV-S08 | a command holds only validated values in an absolute, quoted directory | command safety | commands, shell lines | AC-SEV-013, AC-SEV-018 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---|---|---|
| Declared versus executed and verified | T1, T3, S1, S5 | covered |
| Verification baseline and staleness | T2, S2, S14 | covered |
| Stopped runs and real admission | T1, T5, S3, S16, S17 | covered |
| Review evidence | T1, S9 | covered except fresh reviews for unfingerprinted receipts (Completion Debt) |
| Pipeline counts, platform line, dry runs | T5, S6, S11, S15 | covered |
| Next commands, providers, Desktop, safety | T4, T6, S4, S7, S8, S10, S13, S18 | covered |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| Review receipts record no SPEC fingerprint | A `fresh` review state for receipts written before R11; status shows them `stale` or `unknown` | R11 makes the receipt writer record `spec_fingerprint` by calling `specDocumentsFingerprint`; the writer (`internal/cli/spec_review_*`) is outside this SPEC's ownership |

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Let platform selection prefer configured platforms | Changes pipeline behaviour, not its reporting | Users are routinely surprised by the first-installed rule |
| Hash flagged files directly instead of reporting unknown | Sparse checkouts are rare here | Sparse-checkout users need verification |
| A `--spec` filter for status | Not needed for correctness | Large monorepos |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `internal/cli/status.go`, `status_json.go`, `status_scan.go`, `pipeline_run.go`, `pipeline_run_flags.go`, `pipeline.go`, `init.go`, `runtime_helper_delegate.go`, `pipeline_run_owner.go`, `pipeline_backend_orca.go`, `pipeline_backend_omp.go`, `spec_review_receipt.go`, `spec_review_runtime.go` | existing | read on 2026-11-02 |
| `pkg/pipeline/checkpoint.go`, `checkpoint_restore.go`, `phase_output.go`, `engine_run.go`, `receipt.go`, `receipt_lineage.go` | existing | read on 2026-11-02 |
| PR #252 `internal/cli/spec_review_session.go` | external, not on base | read through `gh pr diff 252` on 2026-11-02 |
| every `[NEW]` file in `spec.md` | added on `feat/status-execution-evidence` | `go test ./internal/cli/... ./pkg/pipeline/...` |

## Reviewer Brief

- Intended scope: R5 of the brief for status, pipeline run output, dry-run persistence, and init guidance, plus a read-only admission entry in `pkg/pipeline`.
- Explicit non-goals: see the Outcome Lock.
- Self-verified: probes B1, B2, and B3 executed; traceability over 21 requirements; the 16 findings of review revision 0 answered.
- Reviewer should focus on: any path that reports verified, fresh, or admitted without the engine's or the fingerprint's agreement; any value reaching a command unvalidated; any status write or execution.

## Residual Risks

- A tree with a file flagged assume-unchanged or skip-worktree — every sparse checkout — never reads verified.
- Ignored files are outside the fingerprint, and inside a submodule only its checked-out commit counts.
- Admission covers what the engine checks before dispatch; the provider CLI and delegation limits are checked when the run starts.
- The newest state root is chosen by recorded run and baseline times, which a copied state directory carries along.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: every cited symbol, format, and message was read on 2026-11-02
- Q-CORR-02 | status: PASS | attempt: 2 | files: spec.md, plan.md | reason: new files carry `[NEW]`
- Q-CORR-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: EARS lines and bare Given/When/Then steps
- Q-CORR-04 | status: PASS | attempt: 2 | files: research.md | reason: existing, external, and added references are separated
- Q-COMP-01 | status: PASS | attempt: 2 | files: all four | reason: contract, tasks and probes, oracles, and decisions each live in their file
- Q-COMP-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: 21 requirements map to S1–S18, including init facts and every admission and baseline failure
- Q-COMP-03 | status: PASS | attempt: 2 | files: spec.md | reason: each requirement names its trigger and its output; the JSON contract names every field
- Q-COMP-04 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: the Outcome Lock covers every R5 clause; the one open slice is Completion Debt
- Q-COMP-05 | status: PASS | attempt: 2 | files: research.md, acceptance.md | reason: INV-S01 to INV-S08 map to oracles, hidden changes, invalid baselines, re-dispatched restores, and refused outputs included
- Q-COMP-06 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: Traceability Matrix with invariants; Reviewer Brief names the focus
- Q-COMP-07 | status: PASS | attempt: 2 | files: research.md | reason: Completion Debt names the review fingerprint writer honestly
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md | reason: B1 to B3 executed with test evidence; B2 runs the real `--continue`
- Q-FEAS-01 | status: PASS | attempt: 2 | files: plan.md | reason: every promise is runtime code in internal/cli and pkg/pipeline
- Q-FEAS-02 | status: PASS | attempt: 2 | files: spec.md | reason: ownership lists every touched path, the recapture section included
- Q-FEAS-03 | status: PASS | attempt: 2 | files: plan.md | reason: the listed commands exist
- Q-STYLE-01 | status: PASS | attempt: 2 | files: spec.md | reason: no ambiguous modal in a requirement
- Q-STYLE-02 | status: PASS | attempt: 2 | files: spec.md | reason: Priority on its own line apart from the EARS type
- Q-STYLE-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: complete sentences, bare steps
- Q-SEC-01 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: commands take only allowlisted or pattern-checked values in quoted absolute directories; S13 injects shell syntax
- Q-SEC-02 | status: PASS | attempt: 2 | files: spec.md | reason: status reads the worker config's presence only and strips control characters from shown strings
- Q-SEC-03 | status: PASS | attempt: 2 | files: spec.md | reason: one 0600 sidecar holding hashes and identifiers
- Q-COH-01 | status: PASS | attempt: 2 | files: spec.md | reason: one story: what status claims and what it tells the user to do next
- Q-COH-02 | status: PASS | attempt: 2 | files: research.md | reason: admission is decided, not deferred; the one deferral is Completion Debt
- Q-COH-03 | status: PASS | attempt: 2 | files: research.md | reason: no sibling SPEC
