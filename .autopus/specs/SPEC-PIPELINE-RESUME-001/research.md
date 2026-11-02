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
- Records hold the sanitized, compacted text that prompts read (probe B1), so restore needs no re-derivation and resumed prompts are byte-identical; `previous` feeds both prompt paths through one `priorPhaseInputs` table, so the plain prompt carries the same prior outputs as the frozen one.
- The checkpoint version moves to v2 because a v1 file cannot prove its outputs; v1 stays readable as legacy, and an older binary refuses v2 (probe B3).
- Unverifiable read-only phases are re-dispatched; any unverifiable phase that writes the working tree blocks, because only the operator can tell whether its earlier writes are still wanted.
- The remediation budget and breaker memory reset on an explicit resume, which is a user decision; the receipt's `resume.remediation_resets` records each reset gate with the cycles spent before and the fingerprint the breaker last held, and prior failures stay in `prior_attempts`.

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
| INV-P10 | lineage is append-only across resumes, and a resume's reset of gate budgets is itself recorded | history conservation | task ID, prior attempts, counts, remediation resets | AC-PRR-010, AC-PRR-012 |
| INV-P11 | secrets never reach a stored, forwarded, or returned failure string | redaction | prompts, receipt, checkpoint, sidecar, lineage, returned error | AC-PRR-003, AC-PRR-011, AC-PRR-013 |
| INV-P12 | a finding is identified by what it says, not by its reviewer-assigned ID | identity rule | fingerprints, breaker decision | AC-PRR-015 |
| INV-P13 | a display bound never hides a cut nor merges two different failures | bound and marker | feedback text, fingerprints | AC-PRR-014 |
| INV-P14 | prompt-state changes are visible in the layer manifest without content | manifest observability | manifest entries and changes | AC-PRR-016 |

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
| Budget reset on explicit resume | T6, S12 | covered |
| Redaction of every stored and returned string | T2, T3, T6, S13 | covered |
| Display bounds, finding identity, manifest layers | T2, T3, S14, S15, S16 | covered |
| OMP view restore | T5, T6, S17 | covered |
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
| `pkg/workflow/remediation.go`, `pkg/promptlayer/context_scan.go`, `pkg/promptlayer/layer.go` | existing, reused rule, sanitizer, and manifest renderer | read on 2026-11-02; sanitizer probed (B1) |
| `content/rules/spec-quality.md` | existing rule this SPEC answers (prompt-state manifest, full checklist) | read on 2026-11-02 |
| every `[NEW]` file in `spec.md` | planned addition on branch `fix/pipeline-resume-retry-contract` | `go test ./pkg/pipeline/... ./internal/cli/...` |

## Reviewer Brief

- Intended scope: R4, R3, R2 of the brief for `pkg/pipeline`, plus one CLI message helper.
- Explicit non-goals: see the Outcome Lock.
- Self-verified: three reproductions on the base commit, three design probes, traceability over 25 requirements, and the codex review's ten findings answered below.
- Reviewer should focus on: whether any non-transport failure can still reach a second dispatch; whether invalidation can leave a stale PASS; whether a resume can restore an output it did not verify or re-dispatch a completed working-tree phase; whether evidence can carry a secret into a prompt or the checkpoint.

## Residual Risks

- Content hashes are unkeyed: a consistent edit of a single record whose phase has no completed successor restores. Records are local state written by the same user.
- A backend that wraps `ErrRetryableTransport` after its model acted would cause a repeated side effect; the sentinel's contract says it must not.
- Checkpoints keep their 0644 mode and now carry redacted phase outputs.

## Review Response (codex, 2026-11-02, judge REVISE, 10 accepted)

| Finding | Resolution |
|---|---|
| F-PRR-01 raw error text in blocker, transitions, lineage | FAIL-05 covers every stored, forwarded, and returned string; the receipt redacts reasons and blocker itself, lineage copies are redacted, returned errors keep identity; S13 |
| F-PRR-02 admission versus transport retry | RESP-01/02 make the canonical engine set aside a retryable transport failure before admission; S6 asserts the dependents run after the re-sent phase passes |
| F-PRR-03 plain prompt carried only the previous output | one `priorPhaseInputs` table for both prompt paths; S7 checks both markers on both paths |
| F-PRR-04 finding identity by ID | FAIL-12 and the Signature definition; S15 with a reused and a renumbered ID |
| F-PRR-05 silent truncation | FAIL-11 states the bounds, marks each cut, and keeps signatures untruncated; S14 |
| F-PRR-06 missing oracles | S8 adds duplicate and schema; S17 opens the OMP views |
| F-PRR-07 prompt layer manifest | `## Prompt Layer Manifest Contract` in spec.md; FAIL-10 makes feedback a manifest layer; S16 |
| F-PRR-08 identical request | FAIL-02 names the payload that stays and the attempt that advances; S6 checks attempts 1, 2, 3 |
| F-PRR-09 checklist record | every Q item below |
| F-PRR-10 probe class | B3 is an `implementation_assumption` |

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: every cited symbol and behaviour was read or reproduced on 2026-11-02, including the manifest renderer
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: new files carry `[NEW]` in Generated And Changed Surfaces and the tasks
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Event-driven requirements use `WHEN ..., THEN`, Unwanted use `IF ..., THEN`, and every scenario is Given/When/Then
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing references from `[NEW]` additions with the verification used
- Q-COMP-01 | status: PASS | attempt: 2 | files: all four | reason: contract, manifest classification, tasks and probes, oracles, and evidence each live in their file
- Q-COMP-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: 25 requirements map to S1–S17, including OMP restore, duplicate and schema rejection, and both prompt paths
- Q-COMP-03 | status: PASS | attempt: 1 | files: plan.md | reason: tasks follow the commit order R4, R3, R2 with one verification task
- Q-COMP-04 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: Outcome Lock adds redaction of every failure string and marked display bounds; S13 and S14 close them
- Q-COMP-05 | status: PASS | attempt: 2 | files: research.md, spec.md, acceptance.md | reason: INV-P01 to INV-P14 map to requirements, tasks, and Must oracles with exact values, including review identity and request identity
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix carries the invariant column; Reviewer Brief names the focus areas
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt is empty and Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md | reason: three probe rows executed on the base commit with PASS status and evidence, each classed `implementation_assumption`
- Q-FEAS-01 | status: PASS | attempt: 2 | files: plan.md, spec.md | reason: runtime changes in pkg/pipeline carry every promise; the plain prompt path now reads the shared prior-input table T5 names
- Q-FEAS-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: content/ and pkg/ are the sources; generated roots are not edited and goldens are re-recorded by the writer
- Q-FEAS-03 | status: PASS | attempt: 1 | files: plan.md | reason: go test, golangci-lint, and source-lines commands exist and ran on 2026-11-02
- Q-STYLE-01 | status: PASS | attempt: 2 | files: spec.md | reason: FAIL-02 separates the payload that stays from the attempt that advances; no ambiguous modal remains
- Q-STYLE-02 | status: PASS | attempt: 1 | files: spec.md | reason: every requirement has Priority Must on its own line apart from the EARS type
- Q-STYLE-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: complete sentences and bare Given/When/Then/And steps that the parser reads
- Q-SEC-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: model output, review text, and the brief are untrusted; injection markers are neutralized and the retry sentinel's trust condition is stated
- Q-SEC-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: FAIL-05 redacts every stored, forwarded, and returned failure string while keeping error identity; S13 plants a secret in a backend error and in a stored receipt
- Q-SEC-03 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: checkpoint, sidecar, and lineage hold redacted, bounded evidence only; manifest entries hold hashes, never content
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md | reason: admission, remediation, and resume form one run-and-resume lifecycle of the canonical engine
- Q-COH-02 | status: PASS | attempt: 1 | files: research.md | reason: R5 and R10 stay explicit non-goals; nothing required is deferred
- Q-COH-03 | status: PASS | attempt: 1 | files: research.md | reason: no sibling SPEC
