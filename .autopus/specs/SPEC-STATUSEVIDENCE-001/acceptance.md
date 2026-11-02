# SPEC-STATUSEVIDENCE-001 Acceptance Criteria

Every scenario uses a temporary project and git repository and fixture checkpoints and receipts written through the real `pipeline` types. Scenarios that depend on installed CLIs, the Desktop helper, or the home directory replace the status probe; none touches the real home, `PATH`, or `.autopus` state. S1, S3, S4, and S12 use only the status command and fail on the code before this SPEC.

## Test Scenarios

### S1: AC-SEV-001 - A declared done SPEC without evidence is not verified
Priority: Must
Given a project whose only SPEC declares `**Status**: done` and has no pipeline checkpoint
When `auto status` and `auto status --json` run
Then the text row reads `SPEC-DONE-001  ✓ done  documented_only`, the declared line still reads `전체: 1/1 완료`, and the evidence line reads `Evidence: 0 verified · 1 documented_only`
And the JSON spec has `status` `done`, `evidence.label` `documented_only`, `evidence.verification.state` `none`, no `evidence.execution`, `summary.verified` 0, and `summary.evidence.documented_only` 1 (test `TestStatusEvidence_DocumentedDoneWithoutReceiptIsNotVerified`).

### S2: AC-SEV-002 - Verification goes stale when code changes, not when the verified tree is committed
Given a SPEC whose completed run recorded a verification baseline over an edited working tree
When status runs, then after the verified tree is committed, after a later code edit, after that edit is undone, after the `spec.md` status line changes to `completed`, and after an acceptance criterion is added
Then the label and state are `verified`/`fresh`, `verified`/`fresh` with HEAD moved, `stale`/`stale` with reason `working tree changed since verification: pkg/app/app.go`, `verified`/`fresh`, `verified`/`fresh`, and `stale`/`stale` with reason `SPEC documents changed since verification`
And without the baseline the completed run reads `unverified`/`unknown` with reason `no verification baseline`, never `fresh`
And only a completed run writes the baseline, mode 0600, naming `Pipeline verification baseline: .autopus/pipeline-state/<SPEC>.verification.json (HEAD <short>)`, while outside git the run prints `Pipeline verification baseline not recorded:` and completes (tests `TestStatusEvidence_VerificationGoesStaleWhenCodeChanges`, `TestPipelineBaseline_RecordedOnlyForCompletedRuns`, `TestPipelineBaseline_FingerprintIgnoresCommitsAndCatchesEdits`, `TestPipelineBaseline_StatusLinesDoNotChangeTheSpecFingerprint`).

### S3: AC-SEV-003 - A failed run is resumable until HEAD moves
Given a checkpoint on platform `codex` whose run ended `blocked` with a last failure decision `circuit_break_no_progress`, attempt 2 after a resume that restored plan and test_scaffold, implement done, validate failed, review pending, 4 dispatches and 7 cumulative, and a git hash equal to HEAD
When status runs
Then it reports `run: run-2 · attempt 2 · blocked (circuit_break_no_progress)`, `phases: 1 executed · 2 restored · 1 failed · 1 pending · 4 dispatches (7 cumulative)`, the blocker, and `resumable: yes — auto pipeline run SPEC-RUN-001 --continue --platform codex`, with the same values in the JSON execution
And after a new commit it reports `resumable: no — HEAD moved since the run; --continue refuses a stale checkpoint` and offers `auto pipeline run SPEC-RUN-001` as a new run
And a `<SPEC>.blocked.yaml` newer than the checkpoint adds `last attempt blocked before dispatch:` with its blocker (tests `TestStatusEvidence_FailedRunIsResumableUntilHEADMoves`, `TestStatusEvidence_RefusedResumeIsReported`).

### S4: AC-SEV-004 - Work without a SPEC is offered beside planning
Given a project with no SPEC and the platforms `claude-code` and `codex` configured
When status runs
Then the next actions include `/auto fix "<problem>" (claude-code) · $codex-auto-fix "<problem>" (codex)` for a small change without a SPEC and `/auto plan "<outcome>" (claude-code) · $codex-auto-plan "<outcome>" (codex)` for new work, each with `runs_in` `agent_session` and `auto_run` false, and the old `/auto plan으로 시작하세요` line is gone (test `TestStatusNext_NoSpecOffersWorkWithoutASpec`).

### S5: AC-SEV-005 - JSON and text mean the same thing
Given the S3 project plus a declared done SPEC and an approved SPEC
When `auto status` and `auto status --json` run
Then every label, count, reason, environment fact, and next command in the JSON appears with the same value in the text, every next action has `auto_run` false, and the first next action is the resume (test `TestStatusEvidence_JSONAndTextAgree`).

### S6: AC-SEV-006 - Restored phases are never counted as executed
Given a resumed run whose receipt restored plan and test_scaffold and dispatched implement, validate, and review once each
When `auto pipeline run` prints its result and status reads the receipt
Then the result line is exactly `Pipeline complete: 3 phases executed, 2 restored from checkpoint, 3 dispatches` and status reports executed 3 and restored 2 (tests `TestPipelineRunSummary_SeparatesExecutedAndRestored`, `TestStatusEvidence_FailedRunIsResumableUntilHEADMoves`).

### S7: AC-SEV-007 - Desktop is optional, and absent Desktop still yields valid commands
Given no worker config and the Desktop helper not installed
When status runs
Then it reports `Desktop: not installed (optional; the ADK works standalone)`, offers no Desktop command, and still lists `/auto fix "<problem>" (claude-code)`
Given the helper installed and no worker config
Then the Desktop connection appears only as the optional action `auto connect`, `optional · runs in terminal · provider: none`, with `external_calls` naming the Autopus server
Given a worker config
Then it reports `Desktop: connected (worker config found)` and offers no connect action (test `TestStatusNext_DesktopIsOptional`).

### S8: AC-SEV-008 - Several installed providers and the selection rule
Given `claude`, `codex`, and `agy` on `PATH` and only `codex` configured
When status runs
Then it reports `AI CLIs on PATH: claude, codex, agy`, `Configured platforms: codex`, `auto pipeline run platform: claude — first installed of claude → codex → agy`, and `note: claude is not a configured platform here; pass --platform codex to use codex`, and the new run it offers for a stopped SPEC carries provider `claude`
Given nothing installed
Then the reason is `no AI CLI installed; falls back to claude`, the note says the run cannot start, and the first next action is `auto doctor` (test `TestStatusNext_SeveralProvidersInstalled`).

### S9: AC-SEV-009 - Review evidence and the review session
Given a review receipt with verdict `REVISE`, gate `blocked`, loop `revisions_exhausted`, finished at a fixed time
When status runs before and after `plan.md` is modified past that time
Then the review reads `review: REVISE (gate blocked, loop revisions_exhausted) at 2026-11-01T10:00:00Z — fresh`, then `stale` with reason `SPEC documents changed after the review: plan.md`
And a `review-session.json` of schema `spec_review_session.v1` with state `budget_exhausted`, 2 attempts, and 5 distinct call IDs reads `review session: budget_exhausted · 2 attempts · 5 calls`, while one holding `not json` or schema `spec_review_session.v9`, or a receipt of another schema, reads `unknown` with the reason and status still exits 0 (test `TestStatusEvidence_ReviewReceiptAndSession`).

### S10: AC-SEV-010 - Init leads with standalone use
Given `auto init --yes` with platforms `claude-code` and `codex` and no worker config
When it prints its summary
Then the next lines name `/auto plan` and `$codex-auto-plan` for new work and `/auto fix` and `$codex-auto-fix` for a small change, followed by an `Optional:` line that says the ADK works without Desktop, and no line before it names Desktop (tests `TestInitGuidance_StandaloneFirst`, `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`).

### S11: AC-SEV-011 - The pipeline names its platform and why
Given `--platform codex`, then only `codex` and `agy` on `PATH`, then nothing on `PATH`
When the platform is resolved
Then the platforms and reasons are `codex` with `--platform`, `codex` with `first installed of claude → codex → agy`, and `claude` with `no AI CLI installed; falls back to claude`, agy is named `gemini (agy)`, and a dry run prints `Pipeline platform: codex — --platform` and records no baseline (tests `TestPipelinePlatformSelection_Reasons`, `TestPipelineRunCmd_ReportsPlatformAndDryRunRecordsNoBaseline`).

### S12: AC-SEV-012 - Scope, size, and no side effects
Given the branch diff and a status run in a project fixture
When the paths are listed, the source-lines gate runs, and the fixture tree is listed before and after status
Then every path is in the module ownership of `spec.md`, no file is over 300 counted lines, and status left every fixture file outside `.git` with the same mode, content, and modification time (test `TestStatusEvidence_StatusWritesNothing`).

## Oracle Acceptance Notes

Every oracle is a fixed string, count, label, or reason; none is recomputed from the implementation's helpers. S2's commit-invariance rests on probe B1, which `TestPipelineBaseline_FingerprintIgnoresCommitsAndCatchesEdits` repeats in the repository. S5 compares the decoded JSON with the rendered text field by field.
