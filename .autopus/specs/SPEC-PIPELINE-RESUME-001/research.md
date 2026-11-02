# SPEC-PIPELINE-RESUME-001 Research

## Existing Code Analysis

Read on 2026-11-02 at base `109c7ea4` (unchanged since the brief's baseline `4a5fa659` for these files).

- `pkg/pipeline/engine_run.go`: `Run` skips `done` phases with a "restored from checkpoint" transition and never fills `state.previous`; `executePhase` builds the prompt before its retry loop and `continue`s on a failed gate with that prompt; a Go error or a failed admission stops the run (`cancelled` for context errors, else `blocked`).
- `pkg/pipeline/engine_prompt.go`: `validatePhaseResponse` (nil, timeout, exit code, empty output) and `appendGateContract`.
- `pkg/pipeline/runner.go`, `parallel_dispatch.go`: `SequentialRunner` checks only the Go error before `EvaluateGate`; `ParallelRunner` checks the Go error and nil. `ParallelRunner` blocks dependents of a gate failure and stops admission on an execution error.
- `pkg/pipeline/checkpoint.go`, `types.go`, `engine_persist.go`: checkpoint `pipeline_checkpoint.v1`, atomic save, `ValidateResume` (identity, SPEC, route version, snapshot, dependency closure), `ValidateResumeRoute`; `checkpointForState` writes statuses and the receipt only.
- `pkg/pipeline/receipt.go`: `orchestration_run_receipt.v1` with `run_id`, dispatch evidence, transitions, terminal state; `TerminalFailedBudget` exists and is unused.
- `pkg/pipeline/omp_execution_view.go`: `completedOMPHistory` reads `state.previous`; `newPhaseRequest` builds the active prompt without prior outputs.
- `pkg/pipeline/phase_prompt.go`: prior outputs reach prompts through `sanitizePromptContent`.
- `internal/cli/pipeline_run.go`, `pipeline.go`: `--continue` loads `.autopus/pipeline-state/<SPEC>.yaml`, refuses a checkpoint whose git hash differs from HEAD, and prints `Pipeline complete` after `Run`.
- `internal/cli/pipeline_backend.go`: provider failures carry `FailureClass` `timeout`, `empty_output`, or `execution_error`; nothing marks a failure as safe to re-send.
- `pkg/workflow/remediation.go`: `RunGateRemediation` stops a fix loop on consecutive identical signatures (`circuit_break_no_progress`) or budget (`budget_exhausted`).
- `content/skills/references/agent-pipeline/phases.md`, `review.md`: validation FAIL goes back to the implementation owner and re-validates; review is read-only; earlier PASS results are claims until re-run.

## Outcome Lock

- User-visible outcome: a pipeline run either completes with every gate judged on the final code, or stops with a classified, evidenced blocker; a resumed run restores the exact outputs its phases need or says which phase cannot be restored and why; all runners refuse execution failures before reading a verdict.
- Mandatory requirements: REQ-PRR-SCOPE-01 through REQ-PRR-VERIFY-01 in `spec.md`.
- Explicit non-goals: token and cost accounting, status and result-count output, keyed checkpoint integrity, cross-resume budgets, parallel strategy for the canonical route, re-sending timeouts or unmarked errors.
- Completion evidence: S1 to S11, the source-lines gate, and the base-commit reproductions recorded below.

## Visual Planning Brief

`plan.md` carries the Mermaid flow. In prose: resume planning verifies records before any state exists; the loop dispatches phases by index; admission is shared; a retryable transport failure waits and re-sends; a typed gate failure becomes evidence for `implement` and rewinds the loop to it; the breaker and budget end the loop without success; each completed phase leaves a hashed record that the next resume verifies.

## Reproductions (base `109c7ea4`, 2026-11-02, scratch tests deleted afterwards)

- R4: `ParallelRunner` and `SequentialRunner` with parent response `TimedOut: true, ExitCode: 42, Output: "VERDICT: PASS"` returned `err=<nil>`, calls `[parent child]`, 2 results.
- R3: compact route, validate always `FAILED_COMMAND: go test -run TestUnique4711` / `VERDICT: FAIL`: calls `[implement validate validate validate validate]`, the validate prompts were identical, and the second lacked `TestUnique4711`.
- R2: plan `PLAN_DYNAMIC_MARKER`, scaffold, implement cancelled: the checkpoint file did not contain the plan output; the resumed calls were `[implement validate review]` with terminal `completed`, and the resumed implement prompt carried neither marker while the first carried the plan marker.

## Design Decisions

- Admission is a function, not a wrapper backend, so the canonical engine keeps its own receipt evidence and the runners keep their learning hooks.
- Re-sending is opt-in through `ErrRetryableTransport`: the engine cannot know whether a failed subprocess already wrote files, so an unmarked error, a timeout, or a non-zero exit is not repeated.
- A malformed or missing verdict is a contract violation and stops the run instead of being re-asked; `--continue` re-dispatches the failed phase on request.
- The fix goes to `implement`, matching the phase reference; the gate only re-dispatches itself on a route without `implement`.
- The breaker reuses `RunGateRemediation`'s rule and reason codes over fingerprints instead of importing its exit-code type, which cannot carry a fingerprint; budget per gate is `Phase.MaxRetries` spent across the run.
- Records hold the sanitized, compacted text that prompts read (probe B1), so restore needs no re-derivation and resumed prompts are byte-identical; `previous` also feeds the legacy prompt path, which closes the same gap there.
- The checkpoint version moves to v2 because a v1 file cannot prove its outputs; v1 stays readable as legacy, and an older binary refuses v2 (probe B3).
- Unverifiable read-only phases are re-dispatched; any unverifiable phase that writes the working tree blocks, because only the operator can tell whether its earlier writes are still wanted.
- The remediation budget and breaker memory reset on an explicit resume; prior failures stay visible in `prior_attempts`.

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-P01 | an execution failure cannot be turned into a pass by its own output | precedence of admission over verdict | runner errors, results | AC-PRR-001 |
| INV-P02 | a failed parent never runs its children | dependency gating | dispatched phases | AC-PRR-001, AC-PRR-002 |
| INV-P03 | the fix attempt sees why the previous attempt failed | prompt content | fix prompt | AC-PRR-003, AC-PRR-004 |
| INV-P04 | identical requests are re-sent only for marked transport failures, at most twice | retry bound | dispatch count, waits | AC-PRR-006 |
| INV-P05 | a verdict is never reused after the code it judged changed | invalidation | re-dispatched gates | AC-PRR-004 |
| INV-P06 | no progress and exhaustion end without success and keep their evidence | terminal state | terminal, failures, checkpoint | AC-PRR-005 |
| INV-P07 | a resumed phase reads exactly what the interrupted run would have given it | equality of prompts | implement prompt | AC-PRR-007 |
| INV-P08 | an output that cannot be verified is never restored | integrity gate | resume decision, blocker | AC-PRR-008, AC-PRR-009 |
| INV-P09 | completed working-tree writes are never repeated automatically | side-effect safety | dispatched phases | AC-PRR-008, AC-PRR-010 |
| INV-P10 | lineage is append-only across resumes | history conservation | task ID, prior attempts, counts | AC-PRR-010 |
| INV-P11 | secrets never reach persisted or forwarded evidence | redaction | prompts, checkpoint file | AC-PRR-003, AC-PRR-011 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---|---|---|
| Shared response contract | T1, S1, S2 | covered |
| Failure classification and evidence | T2, S3, S6 | covered |
| Fix routing and re-verification | T3, S3, S4 | covered |
| Breaker and budget | T3, S5 | covered |
| Output records and restore | T4, T5, S7 | covered |
| Integrity rejection and legacy | T5, S8, S9 | covered |
| Lineage and external writes | T6, S10 | covered |
| Status output, usage accounting | R5, R10 work packages | out of scope by Outcome Lock |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Pass the open finding list to the re-review as a verify checklist | The fix prompt already carries it; re-review stays a fresh judgement | Re-reviews keep raising moved findings |
| Have the CLI provider backend mark pre-dispatch failures as retryable | Needs per-provider evidence that no model turn started | A provider exposes that signal |
| Keyed checkpoint integrity | Needs a per-user key store | Checkpoints start crossing trust boundaries |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `pkg/pipeline/engine_run.go`, `engine_prompt.go`, `engine_persist.go`, `checkpoint.go`, `types.go`, `receipt.go`, `runner.go`, `parallel_dispatch.go`, `omp_execution_view.go`, `phase_prompt.go`, `phase.go`, `phase_gate.go` | existing | read on 2026-11-02 |
| `internal/cli/pipeline_run.go`, `pipeline.go`, `pipeline_backend.go`, `pipeline_backend_omp.go` | existing | read on 2026-11-02 |
| `pkg/workflow/remediation.go`, `pkg/promptlayer/context_scan.go` | existing, reused rule and sanitizer | read on 2026-11-02; sanitizer probed (B1) |
| every `[NEW]` file in `spec.md` | planned addition on branch `fix/pipeline-resume-retry-contract` | `go test ./pkg/pipeline/... ./internal/cli/...` |

## Reviewer Brief

- Intended scope: R4, R3, R2 of the brief for `pkg/pipeline`, plus one CLI message helper.
- Explicit non-goals: see the Outcome Lock.
- Self-verified: three reproductions on the base commit, three design probes, traceability over 21 requirements.
- Reviewer should focus on: whether any non-transport failure can still reach a second dispatch; whether invalidation can leave a stale PASS; whether a resume can restore an output it did not verify or re-dispatch a completed working-tree phase; whether evidence can carry a secret into a prompt or the checkpoint.

## Residual Risks

- Content hashes are unkeyed: a consistent edit of a single record whose phase has no completed successor restores. Records are local state written by the same user.
- A backend that wraps `ErrRetryableTransport` after its model acted would cause a repeated side effect; the sentinel's contract says it must not.
- Checkpoints keep their 0644 mode and now carry redacted phase outputs.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: every cited symbol and behaviour was read or reproduced on 2026-11-02
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: new files carry `[NEW]` in Generated And Changed Surfaces and the tasks
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Event-driven requirements use `WHEN ..., THEN`, Unwanted use `IF ..., THEN`, and every scenario is Given/When/Then
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing references from `[NEW]` additions with the verification used
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: contract, tasks and probes, oracles, and evidence each live in their file
- Q-COMP-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: 21 requirements map to S1–S11 with failure, rejection, and legacy paths
- Q-COMP-03 | status: PASS | attempt: 1 | files: plan.md | reason: tasks follow the commit order R4, R3, R2 with one verification task
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Outcome Lock states admission, feedback, breaker, restore, and lineage; residual risks are stated
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, spec.md, acceptance.md | reason: INV-P01 to INV-P11 map to requirements, tasks, and Must oracles with exact values
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix carries the invariant column; Reviewer Brief names the focus areas
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt is empty and Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 1 | files: plan.md | reason: three probe rows executed on the base commit with PASS status and evidence
