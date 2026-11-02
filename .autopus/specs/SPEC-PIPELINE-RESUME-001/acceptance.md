# SPEC-PIPELINE-RESUME-001 Acceptance Criteria

Every scenario runs against fake `PhaseBackend` implementations and `t.TempDir()` state; no model, no network, and no real `.autopus/pipeline-state`. "Calls" is the ordered list of phase IDs the fake backend received.

## Test Scenarios

### S1: AC-PRR-001 - One response contract for every runner
Priority: Must
Given three runners — the canonical engine on the full route, `SequentialRunner` over `parent` (validation gate, `MaxRetries: 2`) then `child`, and `ParallelRunner` over `parent` (validation gate) and `child` depending on it — and a backend whose first phase answers with one case of the table below while every other phase answers normally
When each runner runs each case

| case | first-phase response | expected error text |
|---|---|---|
| go error | output `VERDICT: PASS`, error `fixture transport down` | `fixture transport down` |
| nil | nil response, nil error | `backend returned nil response` |
| timeout | `TimedOut: true`, output `VERDICT: PASS` | `backend timed out` |
| exit 42 | `ExitCode: 42`, output `VERDICT: PASS` | `backend exited with code 42` |
| empty | output ` \n\t` | `backend returned empty output` |
| pass | output `VERDICT: PASS` | none |

Then every failing case returns an error containing the expected value from the table and the calls are exactly the first phase, once
And the pass case returns no error, with calls `plan, test_scaffold, implement, validate, review` for the canonical engine and `parent, child` for both exported runners
And on base `109c7ea4` the timeout and exit 42 cases returned no error from both exported runners and dispatched `child` (test `TestRunnerResponseContract_RejectsBeforeVerdict`).

### S2: AC-PRR-002 - Parallel behaviour and cancellation are preserved
Priority: Must
Given a backend that cancels the run context and returns `context.Canceled` from the first phase
When each of the three runners runs
Then each returns an error for which `errors.Is(err, context.Canceled)` is true and no dependent phase is dispatched
And the existing tests `TestParallelAdmissionCancellationAndEvidence`, `TestParallelFinalCompletionCannotHideCancellation`, `TestParallelIndependentGateFailureRetainsVerdict`, `TestParallelJoinIncludesEveryParent`, and `TestParallelRunner_EnforcesWorktreeSlotCapAtRuntime` (its fake now answers non-empty output) pass unchanged in their assertions.

### S3: AC-PRR-003 - A test failure reaches the fix step with its evidence
Priority: Must
Given the compact route and a backend answering implement `IMPL_V1`, then validate `FAILED_COMMAND: go test ./pkg/fixture -run TestUniqueMarker4711` / `EXPECTED: exit 0 with 3 items` / `ACTUAL: 4 items; OPENAI_API_KEY=sk-proj-abcdefghijklmnopqrstuvwxyz` / `VERDICT: FAIL`, then implement `IMPL_V2`, validate `VERDICT: PASS`, review `VERDICT: APPROVE`
When the engine runs
Then the calls are `implement, validate, implement, validate, review` and the terminal state is `completed`
And the third prompt contains `## Remediation Feedback`, `test_failure`, `TestUniqueMarker4711`, `exit 0 with 3 items`, `4 items`, and `[REDACTED_SECRET]`, does not contain `sk-proj-abcdefghijklmnopqrstuvwxyz`, and differs from the first prompt, which does not contain `TestUniqueMarker4711`
And the fourth prompt contains `IMPL_V2`
And the receipt holds one failure with kind `test_failure`, phase `validate`, decision `remediate`, `routed_to: implement`, and a fingerprint starting `sha256:`, and the checkpoint file does not contain the secret value
And on base `109c7ea4` the second validation prompt equalled the first and lacked `TestUniqueMarker4711` (test `TestSubprocessEngine_TestFailureFeedbackReachesTheFixStep`).

### S4: AC-PRR-004 - A review rejection is fixed and everything after the fix is re-verified
Priority: Must
Given the compact route and a backend answering implement `IMPL_V1`, validate `VERDICT: PASS`, review `FINDING F-UNIQ-9: handler drops ctx cancellation (engine.go:42) | FIX: return ctx.Err() before dispatch` / `VERDICT: REQUEST_CHANGES`, then implement `IMPL_V2`, validate `VERDICT: PASS`, review `VERDICT: APPROVE`
When the engine runs
Then the calls are `implement, validate, review, implement, validate, review` and the terminal state is `completed`
And the fourth prompt contains `review_rejection`, `F-UNIQ-9`, `handler drops ctx cancellation (engine.go:42)`, and `return ctx.Err() before dispatch`
And the fifth prompt (the second validation) contains `IMPL_V2`, so the first validation PASS was not reused
And the receipt's completed phase count is 3 and its failure record has decision `remediate` and `routed_to: implement` (test `TestSubprocessEngine_ReviewRejectionRoutesFixAndReverifies`).

### S5: AC-PRR-005 - No progress stops at the breaker; exhaustion is never success
Priority: Must
Given the compact route and a validate answer that is always `FAILED_COMMAND: go test ./x` / `EXPECTED: ok` / `ACTUAL: boom` / `VERDICT: FAIL`
When the engine runs with a checkpoint directory
Then the calls are `implement, validate, implement, validate`, the error contains `circuit_break_no_progress`, the terminal state is `blocked`, the receipt holds two failures with the same fingerprint whose last decision is `circuit_break_no_progress`, the persisted checkpoint has implement `done` with an output record and validate `failed`, and the validate result output contains `ACTUAL: boom`
When the validate answer instead ends `ACTUAL: boom #<n>` with n counting 1, 2, 3, 4
Then there are 8 calls (4 implement, 4 validate), the error contains `gate validation failed after 4 attempt(s); remediation budget exhausted`, the terminal state is `failed_budget`, and the last failure decision is `budget_exhausted` (tests `TestSubprocessEngine_IdenticalFailureTripsTheBreaker`, `TestSubprocessEngine_DistinctFailuresExhaustTheBudget`).

### S6: AC-PRR-006 - Only a marked transport failure is re-sent, with bounded backoff
Priority: Must
Given an engine whose sleep is recorded instead of waited
When implement fails twice with an error wrapping `ErrRetryableTransport` and then answers `IMPL`
Then implement is dispatched three times with identical prompts, the recorded waits are exactly `[1s 2s]`, the run completes, and two failures carry decision `retry_transport`
When implement always fails that way
Then there are exactly three implement calls, waits `[1s 2s]`, and the terminal state is `blocked`
When implement fails once with `connection reset` (unmarked), with `TimedOut: true`, or validate answers `looks fine` with no verdict line
Then there is no further dispatch after that failure, no wait, and the failure kind is `transport_failure`, `timeout`, or `contract_violation` respectively
When the context is cancelled during a wait
Then the terminal state is `cancelled` (tests `TestEngineTransportRetry_*`, `TestSubprocessEngine_NonRetryableFailuresStopWithoutAnotherCall`).

### S7: AC-PRR-007 - Resume restores the dynamic outputs into the same prompt
Priority: Must
Given a first run, with and without a SPEC directory, whose backend answers plan `PLAN_DYNAMIC_7f3a`, test_scaffold `SCAFFOLD_DYNAMIC_9c1e`, and cancels the context from implement
When the checkpoint it wrote is loaded and a second run resumes it
Then the checkpoint has `version: pipeline_checkpoint.v2` and two `phase_outputs` records (plan, test_scaffold) with schema `pipeline_phase_output.v1`, policy `pipeline_phase_output_policy.v1`, and `sha256:` content hashes and input fingerprints
And the second run's calls are `implement, validate, review`, its implement prompt equals the first run's implement prompt byte for byte and contains both markers, its `resume.restored_phases` is `[plan test_scaffold]`, and its terminal state is `completed`
And on base `109c7ea4` the resumed prompt carried neither marker (test `TestSubprocessEngine_ResumeRestoresDynamicOutputs`).

### S8: AC-PRR-008 - Unverifiable outputs are never restored silently
Priority: Must
Given the checkpoint of S7's first run
When it is resumed after one change each: the plan record removed; the plan content edited; the plan content edited with its content hash recomputed; the records replaced by those of another SPEC's run; the plan record's policy set to `pipeline_phase_output_policy.v0`
Then each resume makes no call, returns an error containing `pipeline: resume blocked:` and `test_scaffold`, plus respectively `no persisted output record`, `content hash mismatch`, `input fingerprint mismatch`, `input fingerprint mismatch`, and `output policy`, leaves the canonical checkpoint byte-identical, and writes a blocked sidecar whose terminal state is `blocked`
When a checkpoint whose only completed phase is plan has an edited plan content
Then plan is dispatched again (calls `plan, test_scaffold, implement, validate, review`), `resume.rerun_phases` is `[plan]`, and a note contains `content hash mismatch` (tests `TestSubprocessEngine_ResumeRejectsUnverifiableOutputs`, `TestSubprocessEngine_ResumeRedispatchesAnUnverifiableReadOnlyPhase`).

### S9: AC-PRR-009 - A legacy checkpoint is detected and explained
Priority: Must
Given a `pipeline_checkpoint.v1` checkpoint with plan and test_scaffold done and no outputs
When it is resumed
Then no call is made and the error contains `checkpoint pipeline_checkpoint.v1 predates persisted phase outputs` and `test_scaffold writes the working tree`
Given a `pipeline_checkpoint.v1` checkpoint with only plan done
When it is resumed
Then the calls are the five phases, `resume.rerun_phases` is `[plan]`, `resume.restored_phases` is empty, a note contains `predates persisted phase outputs`, and the CLI helper prints that note as a line starting `Pipeline resume: ` (tests `TestSubprocessEngine_Run_ResumeFromCheckpoint`, `TestSubprocessEngine_LegacyCheckpointWithWritesBlocks`, `TestReportPipelineResume_PrintsEveryNote`).

### S10: AC-PRR-010 - Lineage survives several resumes; completed writes never repeat
Priority: Must
Given run 1 cancelled at implement, run 2 resuming it and cancelled at validate, and run 3 resuming run 2
When run 3 completes
Then run 2's calls are `implement, validate` and run 3's calls are `validate, review`, so the completed implement phase is not dispatched again
And the three receipts share one `task_id`, run 2's `resumed_from` is run 1's `run_id`, run 3's is run 2's, run 3's `prior_attempts` run IDs are run 1 then run 2 with terminal states `cancelled` and `cancelled`, run 1's entry keeps its `cancelled` failure for implement, and run 3's `cumulative_dispatch_count` is 7 with its own dispatch count 2
Given a compact run whose validation failed with `TestUniqueMarker4711` and whose fix dispatch was cancelled
When it is resumed
Then the resumed first call is implement and its prompt equals the cancelled fix prompt (tests `TestSubprocessEngine_MultipleResumesKeepLineage`, `TestSubprocessEngine_ResumedFixKeepsItsFeedback`).

### S11: AC-PRR-011 - Scope and size
Priority: Must
Given the branch diff against `origin/main`
When its paths are listed and the source-lines gate runs on the changed Go files
Then every path is under `pkg/pipeline/`, `.autopus/specs/SPEC-PIPELINE-RESUME-001/`, or `internal/cli/pipeline_run*.go`, and the gate reports no file over 300 counted lines.

## Oracle Acceptance Notes

S1, S3, S4, S5, S7, and S10 oracles are fixed call lists, exact substrings, and exact prompt equality, never values recomputed from the implementation's own helpers. The S7 equality oracle is what probe B1 makes possible. S8's recomputed-hash case shows the limit of unkeyed hashes: a consistent edit of one record is caught only through the fingerprint of a later record, which is why a checkpoint whose single completed phase is edited consistently would restore (research.md, Residual Risks).
