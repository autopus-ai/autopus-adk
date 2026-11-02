# SPEC-PIPELINE-RESUME-001: Pipeline Resume Outputs, Failure Feedback, and One Runner Response Contract

**Status**: draft
**Created**: 2026-11-02
**Domain**: PIPELINE
**Priority**: HIGH
**Change Class**: security_or_data (persisted checkpoint schema, redaction of persisted and forwarded failure evidence)
**Source**: ADK improvement brief of 2026-11-01, requirements R2, R3, and R4 (untrusted requirements input, summarized here)
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `pkg/pipeline/**`, `.autopus/specs/SPEC-PIPELINE-RESUME-001/**`; one resume-message helper under `internal/cli/pipeline_run*.go`
**Depends on**: SPEC-ORCH-024 (orchestration run receipt, checkpoint identity and dependency closure), SPEC-OMP-004 (sealed OMP execution view)

## Purpose

`auto pipeline run` drives the canonical dependency-ordered engine (`SubprocessEngine`), and the package also exports a `SequentialRunner` and a `ParallelRunner`. Three defects were reproduced on base `109c7ea4` with fake backends:

1. R4 — the exported runners parse a verdict from any response. A response with `TimedOut: true`, `ExitCode: 42`, and output `VERDICT: PASS` passed its validation gate in both runners and the dependent child phase ran. Only the canonical engine applies `validatePhaseResponse`.
2. R3 — the canonical retry loop builds the prompt once, before the loop. A validation phase that failed with a unique failing command was re-dispatched four times with byte-identical prompts, so the failure never reached the next attempt, and the fix never reached the code: the validator was asked to judge the same input again.
3. R2 — resume restores phase statuses but no phase outputs. With plan and test_scaffold done and implement interrupted, the resumed implement prompt carried neither the plan nor the scaffold output (the first prompt carried both), the checkpoint file held neither, and the resumed run reported `completed`. A new run ID replaced the old one with no link between them.

This SPEC makes every runner share one response contract, turns a failed gate into a classified, redacted failure that is routed to the phase that can fix it followed by re-verification, and persists the outputs later phases need together with the run lineage, so a resume either restores exactly what the first run produced or says why it cannot.

## Background

- The canonical engine is the only engine the CLI uses; the exported runners are library surface. The defect in R4 is therefore not a defect of the default CLI path.
- `PhaseResponse.FailureClass` is set by the CLI provider backend (`timeout`, `empty_output`, `execution_error`); the engine cannot tell from a Go error whether a backend acted before failing.
- The workflow remediation rule in `pkg/workflow/remediation.go` (`RunGateRemediation`) aborts a fix loop on two consecutive identical signatures (`circuit_break_no_progress`) and on budget (`budget_exhausted`). The pipeline reuses that rule and its reason codes over failure fingerprints.
- The agent-pipeline phase reference routes a validation FAIL back to the implementation owner and re-validates; review is read-only and its fixes go back to the owner.
- `promptlayer.SanitizeContent` redacts secret patterns to `[REDACTED_SECRET]` and drops or neutralizes prompt-injection lines; applied twice it returns its first result (probe B1).
- The checkpoint is `pipeline_checkpoint.v1` YAML with `version`, `route_version`, `spec_id`, `snapshot_hash`, `git_commit_hash`, `task_status`, and the run receipt; the receipt schema `orchestration_run_receipt.v1` is shared with orchestra and generated skills, so it only gains optional fields.

## Outcome Boundary

- Outcome Lock: the canonical engine, `SequentialRunner`, and `ParallelRunner` reject a Go error, nil response, timeout, non-zero exit, or empty output before any verdict is parsed and never run a dependent of the rejected phase; a failed validation or review gate becomes a classified failure whose redacted evidence (failing command, expected and actual result, log excerpt, or finding IDs, evidence, and fix conditions) reaches the implementation phase in a rebuilt prompt, after which every verification downstream of the fix runs again; identical or non-progressing failures stop at the circuit breaker and an exhausted budget ends as `failed_budget`, both keeping outputs and evidence; only a backend failure explicitly marked as a retryable transport failure is re-sent, with bounded backoff; a checkpoint persists each completed phase's dynamic output with schema, policy, producing phase, content hash, and input fingerprint, and a resume restores those outputs into byte-identical prompts, re-dispatches an unverifiable output only when no affected phase writes the working tree, and blocks otherwise with a message naming the phase and the reason; every run carries a task ID that survives resumes, its own run ID, `resumed_from`, and the prior attempts' failures and dispatch usage.
- Mandatory requirements: REQ-PRR-SCOPE-01 through REQ-PRR-VERIFY-01 below.
- Explicit non-goals: token or cost accounting (R10); changing `auto pipeline run` result output or status surfaces (R5); keyed integrity (HMAC) of checkpoint files; persisting the remediation budget or the circuit-breaker memory across resumes; making `StrategyParallel` available to the canonical route; retrying a timeout, a non-zero exit, or an unmarked Go error.
- Completion evidence: the tests named in `acceptance.md` pass with fake backends, `go test ./pkg/pipeline/... ./internal/cli/...` pass, the source-lines gate passes on the changed Go files, and each reproduction above is a test that failed on `109c7ea4`.

## Definitions

- Admission: the shared check `admitPhaseResponse` applies to one dispatch: Go error, nil response, `TimedOut`, non-zero `ExitCode`, whitespace-only output, in that order.
- Failure kinds: `transport_failure`, `timeout`, `test_failure` (validation output with exactly one `VERDICT: FAIL`), `review_rejection` (review output with exactly one `VERDICT: REQUEST_CHANGES`), `contract_violation` (nil or empty response, invalid worker receipt, compaction blocker, or a missing, duplicate, or malformed verdict), and `cancelled`.
- Retryable transport failure: a Go error that wraps `pipeline.ErrRetryableTransport`; a backend wraps it only when the request never reached the model.
- Fix phase: the `implement` phase when it precedes the failed gate on the route, else the gate phase itself.
- Failure fingerprint: `sha256:` over the kind, the phase, and the sorted signatures — one per failing command (`command`, `expected`, `actual`) or per finding ID, else the normalized redacted excerpt.
- No progress: the new failure of a gate has every signature of that gate's previous failure in this run (identical fingerprints included).
- Phase output record: `{schema: pipeline_phase_output.v1, phase_id, policy: pipeline_phase_output_policy.v1, attempt, run_id, input_fingerprint, content_hash, content}` where content is the sanitized, compacted output injected into later prompts.
- Input fingerprint: `sha256:` over schema, policy, SPEC ID, snapshot hash, git commit hash, route version, normalized route, phase ID, gate, sorted dependencies, and the phase ID and content hash of every restorable completed phase before it on the route.
- External-write phase: a phase whose dispatch writes the working tree; `test_scaffold` and `implement` (`Phase.ExternalWrites`).

## Requirements

### Scope

**REQ-PRR-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL confine changes to `pkg/pipeline/`, `.autopus/specs/SPEC-PIPELINE-RESUME-001/`, and a resume-message helper in `internal/cli/pipeline_run*.go`, SHALL keep `orchestration_run_receipt.v1` and add only optional receipt fields, and SHALL NOT edit generated harness roots.

### Shared Response Contract (R4)

**REQ-PRR-RESP-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL apply one admission function in the canonical engine, `SequentialRunner`, and `ParallelRunner` before any gate verdict is parsed, failing a dispatch with `phase <id>: <go error>`, `backend returned nil response`, `backend timed out`, `backend exited with code <n>`, or `backend returned empty output`.

**REQ-PRR-RESP-02**
Priority: Must
Type: Unwanted
IF a dispatch fails admission, THEN THE SYSTEM SHALL return an error for the run, SHALL NOT retry it in the exported runners, SHALL NOT dispatch any phase that depends on it, and SHALL NOT let a `VERDICT: PASS` line in its output change that outcome.

**REQ-PRR-RESP-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep `ParallelRunner` admission order, slot cap, independent gate-failure results, and cancellation propagation unchanged, so a cancelled context still yields an error that wraps `context.Canceled`.

### Failure Feedback (R3)

**REQ-PRR-FAIL-01**
Priority: Must
Type: Event-driven
WHEN a canonical dispatch or gate fails, THEN THE SYSTEM SHALL classify it as exactly one failure kind and append a failure record with phase, attempt, kind, fingerprint, decision, the routed fix phase, a redacted summary, the parsed evidence items, and a redacted excerpt to the run receipt.

**REQ-PRR-FAIL-02**
Priority: Must
Type: Event-driven
WHEN a dispatch fails with a retryable transport failure, THEN THE SYSTEM SHALL re-send the identical request at most two more times after waiting 1 s and then 2 s (doubling, capped at 8 s, cut short by cancellation), and a failure of any kind other than `test_failure` or `review_rejection` SHALL stop the run with no further dispatch.

**REQ-PRR-FAIL-03**
Priority: Must
Type: Event-driven
WHEN the validation gate fails with `test_failure` and budget remains, THEN THE SYSTEM SHALL dispatch the fix phase with a prompt rebuilt from its own inputs plus a `## Remediation Feedback` section carrying the failure kind, phase, attempt, fingerprint, each failing command with its expected and actual result, and the last 40 lines (at most 2,000 bytes) of the redacted output.

**REQ-PRR-FAIL-04**
Priority: Must
Type: Event-driven
WHEN the review gate fails with `review_rejection` and budget remains, THEN THE SYSTEM SHALL dispatch the fix phase with the `## Remediation Feedback` section carrying every finding ID with its evidence and fix condition parsed from `FINDING <ID>: <evidence> | FIX: <condition>` lines, and the redacted excerpt.

**REQ-PRR-FAIL-05**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL redact secret patterns to `[REDACTED_SECRET]` and neutralize injection markers in all failure evidence before it is forwarded or persisted, while keeping identifiers, commands, expected and actual values, and finding text.

**REQ-PRR-FAIL-06**
Priority: Must
Type: Event-driven
WHEN a fix is routed, THEN THE SYSTEM SHALL reset the fix phase, the failed gate, and every phase between them to pending, discard their outputs, and dispatch each of them again in route order, so no verdict produced before the fix is reused.

**REQ-PRR-FAIL-07**
Priority: Must
Type: Unwanted
IF a gate fails again with no progress against its previous failure in the run, THEN THE SYSTEM SHALL stop with terminal `blocked` and blocker `phase <id>: circuit_break_no_progress: ...`, keeping the completed phases' outputs, the failed output, and every failure record.

**REQ-PRR-FAIL-08**
Priority: Must
Type: Unwanted
IF a gate fails after it has spent `Phase.MaxRetries` remediation cycles in the run, THEN THE SYSTEM SHALL stop with terminal `failed_budget` and blocker `phase <id>: gate <gate> failed after <n> attempt(s); remediation budget exhausted`, and SHALL never report exhaustion as completion.

**REQ-PRR-FAIL-09**
Priority: Must
Type: Event-driven
WHEN the validation or review gate is dispatched, THEN THE SYSTEM SHALL ask, before the existing final verdict instruction, for `FAILED_COMMAND:`, `EXPECTED:`, and `ACTUAL:` lines (validation) or `FINDING <ID>: <evidence> | FIX: <condition>` lines (review).

### Phase Outputs and Lineage (R2)

**REQ-PRR-OUT-01**
Priority: Must
Type: Event-driven
WHEN a canonical phase completes, THEN THE SYSTEM SHALL store its phase output record in the checkpoint `phase_outputs` list of schema `pipeline_checkpoint.v2`, and SHALL use the record content as that phase's dynamic output in this and every resumed run.

**REQ-PRR-OUT-02**
Priority: Must
Type: Event-driven
WHEN a run resumes a checkpoint that passed the SPEC, route, snapshot, and dependency checks, THEN THE SYSTEM SHALL verify each completed phase's record (schema, policy, a single record per phase, content hash, and input fingerprint over the verified records before it) in route order and restore every verified record into the phase prompts and the OMP completed history.

**REQ-PRR-OUT-03**
Priority: Must
Type: Unwanted
IF a completed phase's record is missing, duplicated, or fails verification, THEN THE SYSTEM SHALL treat that phase and every completed phase after it as unrestorable, re-dispatch them when none of them is an external-write phase, and otherwise block before any dispatch with `pipeline: resume blocked:` naming the phase, the reason, and the fact that it writes the working tree.

**REQ-PRR-OUT-04**
Priority: Must
Type: Unwanted
IF the checkpoint version is `pipeline_checkpoint.v1`, THEN THE SYSTEM SHALL load it, apply REQ-PRR-OUT-03 with reason `checkpoint pipeline_checkpoint.v1 predates persisted phase outputs`, and record the decision in the receipt `resume` notes that the CLI prints as `Pipeline resume: <note>` lines.

**REQ-PRR-OUT-05**
Priority: Must
Type: Event-driven
WHEN a resumed checkpoint's last routed fix had not completed, THEN THE SYSTEM SHALL attach that failure's feedback to the fix phase again, so the resumed fix prompt equals the interrupted one.

**REQ-PRR-LIN-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL give each run a new `run_id`, keep one `task_id` across every resume of a task (a legacy prior receipt without one lends its `run_id`), set `resumed_from` to the prior run ID, append the prior run's ID, terminal state, blocker, dispatch count, provider receipts, failures, and times to `prior_attempts` without dropping older entries, and report `cumulative_dispatch_count`.

**REQ-PRR-LIN-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL never re-dispatch a completed external-write phase on resume, whether it is restored or unrestorable.

### Verification

**REQ-PRR-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL prove every Must requirement with tests that use fake backends and temporary directories only, and SHALL keep every changed Go file within 300 counted lines.

## Generated And Changed Surfaces

- `[NEW] pkg/pipeline/response_contract.go`: `admitPhaseResponse`, `validatePhaseResponse` (moved from `engine_prompt.go`).
- `pkg/pipeline/runner.go`, `parallel_dispatch.go`: admission before verdict parsing.
- `[NEW] pkg/pipeline/phase_failure.go`, `[NEW] phase_failure_report.go`, `[NEW] engine_remediation.go`: classification, evidence, feedback, breaker, transport retry.
- `pkg/pipeline/engine_run.go`, `engine_prompt.go`, `engine.go`, `omp_execution_view.go`, `phase.go`, `receipt.go`: the remediation loop, gate report request, `ExternalWrites`, optional receipt fields.
- `[NEW] pkg/pipeline/phase_output.go`, `[NEW] checkpoint_restore.go`, `[NEW] receipt_lineage.go`; `checkpoint.go`, `types.go`, `engine_persist.go`: records, resume plan, lineage, checkpoint v2.
- `[NEW] internal/cli/pipeline_run_resume.go`: `Pipeline resume:` lines.

## Related SPECs

- SPEC-ORCH-024: receipt and checkpoint identity this SPEC extends.
- SPEC-OMP-004: sealed views whose completed history the restore feeds.
- SPEC-CONTEXT-COMPRESS-001: compaction whose output the record persists.

## Traceability Matrix

| Requirement | Plan task | Acceptance | Invariants |
|---|---|---|---|
| REQ-PRR-SCOPE-01 | T8 | S11 | INV-P11 |
| REQ-PRR-RESP-01, RESP-02 | T1 | S1 | INV-P01, INV-P02 |
| REQ-PRR-RESP-03 | T1 | S2 | INV-P02 |
| REQ-PRR-FAIL-01, FAIL-03, FAIL-05, FAIL-09 | T2, T3 | S3 | INV-P03, INV-P11 |
| REQ-PRR-FAIL-04, FAIL-06 | T3 | S4 | INV-P03, INV-P05 |
| REQ-PRR-FAIL-07, FAIL-08 | T3 | S5 | INV-P06 |
| REQ-PRR-FAIL-02 | T3 | S6 | INV-P04 |
| REQ-PRR-OUT-01, OUT-02 | T4, T5 | S7 | INV-P07 |
| REQ-PRR-OUT-03 | T5 | S8 | INV-P08, INV-P09 |
| REQ-PRR-OUT-04 | T5, T7 | S9 | INV-P08 |
| REQ-PRR-OUT-05, LIN-01, LIN-02 | T6 | S10 | INV-P09, INV-P10 |
| REQ-PRR-VERIFY-01 | T8 | S11 | INV-P11 |
