# SPEC-STATUSEVIDENCE-001 Acceptance Criteria

Every scenario uses a temporary project and git repository, fixture checkpoints and receipts written through the real `pipeline` types, a `PATH` of dummy binaries, a stubbed Desktop helper resolution, and a temporary `HOME`; none touches the real home, `PATH`, or `.autopus` state.

## Test Scenarios

### S1: AC-SEV-001 - A declared done SPEC without evidence is not verified
Priority: Must
Given a project whose only SPEC declares `**Status**: done` and has no pipeline checkpoint
When `auto status` and `auto status --json` run
Then the text row shows `declared done` and `evidence: documented_only` and the summary line reads `1 documented done · 0 verified`
And the JSON spec has `status` `done`, `evidence.label` `documented_only`, `evidence.verification.state` `none`, and `summary.verified` 0 (test `TestStatusEvidence_DocumentedDoneWithoutReceiptIsNotVerified`).

### S2: AC-SEV-002 - Verification goes stale when code changes, not when the verified tree is committed
Given a SPEC whose completed run recorded a verification baseline over an edited working tree
When status runs, then again after the verified tree is committed, after a later edit, and after a SPEC document edit
Then the state is `fresh`, then `fresh` with HEAD moved, then `stale` with reason `working tree changed since verification`, then `stale` with reason `SPEC documents changed since verification`
And a completed run with no baseline reports `unknown` with reason `no verification baseline`, never `fresh` (tests `TestStatusEvidence_VerificationGoesStaleWhenCodeChanges`, `TestPipelineBaseline_RecordedOnlyForCompletedRuns`).

### S3: AC-SEV-003 - A failed run is resumable until HEAD moves
Given a checkpoint whose run ended `blocked` with a last failure decision `circuit_break_no_progress`, attempt 2 after a resume that restored plan and test_scaffold, 7 cumulative dispatches, and `base_head` equal to HEAD
When status runs
Then it reports terminal `blocked`, stop reason `circuit_break_no_progress`, attempt 2, 3 executed, 2 restored, 1 failed, 1 pending phases, the blocker, `resumable: yes`, and the command `auto pipeline run <SPEC> --continue`
And after a new commit it reports `resumable: no` with reason `HEAD moved since the run; --continue refuses a stale checkpoint`
And a newer `<SPEC>.blocked.yaml` adds `last resume refused` with its blocker (tests `TestStatusEvidence_FailedRunIsResumableUntilHEADMoves`, `TestStatusEvidence_RefusedResumeIsReported`).

### S4: AC-SEV-004 - Work without a SPEC is offered beside planning
Given a project with no SPEC and the platforms `claude-code` and `codex` configured
When status runs
Then the next actions include a small change without a SPEC as `/auto fix "<problem>"` and `@auto fix "<problem>"`, and new work as `/auto plan "<outcome>"` and `@auto plan "<outcome>"`, each with `runs_in: agent_session`, and no line says a SPEC is required (test `TestStatusNext_NoSpecOffersWorkWithoutASpec`).

### S5: AC-SEV-005 - JSON and text mean the same thing
Given the S3 project plus a declared done SPEC and an approved SPEC
When `auto status` and `auto status --json` run
Then every label, count, reason, and next command in the JSON appears with the same value in the text, and every next action has `auto_run: false` (test `TestStatusEvidence_JSONAndTextAgree`).

### S6: AC-SEV-006 - Restored phases are never counted as executed
Given a resumed run whose receipt restored plan and test_scaffold and dispatched implement, validate, and review once each
When `auto pipeline run` prints its result and status reads the receipt
Then the result line is exactly `Pipeline complete: 3 phases executed, 2 restored from checkpoint, 3 dispatches` and status reports executed 3 and restored 2 (tests `TestPipelineRunSummary_SeparatesExecutedAndRestored`, `TestStatusEvidence_FailedRunIsResumableUntilHEADMoves`).

### S7: AC-SEV-007 - Desktop is optional, and absent Desktop still yields valid commands
Given no worker config and the Desktop helper not installed
When status runs
Then it reports `Desktop: not installed (optional; the ADK works standalone)`, offers no Desktop command, and still lists valid next actions
Given the helper installed and no worker config
Then the Desktop connection appears only as an optional action `autopus-desktop-runtime connect` with `external_calls` naming the Autopus server
Given a worker config
Then it reports `Desktop: connected` and offers no connect action (test `TestStatusNext_DesktopIsOptional`).

### S8: AC-SEV-008 - Several installed providers and the selection rule
Given `claude`, `codex`, and `agy` on `PATH` and only `codex` configured
When status runs
Then it reports installed `claude, codex, agy`, configured `codex`, `auto pipeline run` platform `claude` with reason `first installed of claude → codex → agy`, a note that `claude` is not a configured platform with the hint `--platform codex`, and pipeline next actions carry provider `claude`
Given nothing installed
Then the reason is `no AI CLI installed; falls back to claude` and the next actions add `auto doctor` (test `TestStatusNext_SeveralProvidersInstalled`).

### S9: AC-SEV-009 - Review evidence and the review session
Given a review receipt with verdict `REVISE`, gate `blocked`, loop `revisions_exhausted`, finished at a fixed time
When status runs before and after `plan.md` is modified past that time
Then the review reports that verdict, time, and `fresh`, then `stale` with reason `SPEC documents changed after the review`
And a `review-session.json` of schema `spec_review_session.v1` with state `budget_exhausted`, 2 attempts, and 5 calls is reported as such, while one holding `not json` or schema `spec_review_session.v9` reports `unknown` with the reason and status still exits 0 (test `TestStatusEvidence_ReviewReceiptAndSession`).

### S10: AC-SEV-010 - Init leads with standalone use
Given `auto init --yes` with platforms `claude-code` and `codex` and no worker config
When it prints its summary
Then the next lines name `/auto plan` and `@auto plan` for new work and `/auto fix` and `@auto fix` for a small change, followed by an `Optional:` line that names the Desktop connection as not required, and no line tells the user to connect Desktop first (tests `TestInitGuidance_StandaloneFirst`, `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`).

### S11: AC-SEV-011 - The pipeline names its platform and why
Given `--platform codex`, then only `codex` and `agy` on `PATH`, then nothing on `PATH`
When the platform is resolved
Then the platforms and reasons are `codex` with `--platform`, `codex` with `first installed of claude → codex → agy`, and `claude` with `no AI CLI installed; falls back to claude` (test `TestPipelinePlatformSelection_Reasons`).

### S12: AC-SEV-012 - Scope, size, and no side effects
Given the branch diff and a status run in a project fixture
When the paths are listed, the source-lines gate runs, and the fixture tree is listed before and after status
Then every path is in the module ownership of `spec.md`, no file is over 300 counted lines, and status left the fixture tree byte-identical (test `TestStatusEvidence_StatusWritesNothing`).

## Oracle Acceptance Notes

Every oracle is a fixed string, count, label, or reason; none is recomputed from the implementation's helpers. S2's commit-invariance rests on probe B1. S5 compares the decoded JSON with the rendered text field by field.
