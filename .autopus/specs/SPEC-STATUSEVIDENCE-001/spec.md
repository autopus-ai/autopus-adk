# SPEC-STATUSEVIDENCE-001: Status Separates Declared SPEC State from Execution Evidence and Names the Next Command

**Status**: draft
**Created**: 2026-11-02
**Domain**: STATUSEVIDENCE
**Priority**: HIGH
**Change Class**: feature (new `auto status --json` evidence fields are a machine-read contract, so the lean profile does not apply)
**Source**: ADK improvement brief of 2026-11-01, requirement R5 (untrusted requirements input, summarized here)
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `internal/cli/status*.go`, `internal/cli/init*.go` (guidance text), `internal/cli/pipeline_run*.go`, the `auto status` rows of `README.md` and `docs/README.ko.md`, `.autopus/specs/SPEC-STATUSEVIDENCE-001/**`, and the TUIUX goldens the init text changes
**Depends on**: SPEC-PIPELINE-RESUME-001 (pipeline receipt lineage, failure decisions, restored phases)

## Purpose

`auto status` counts a SPEC as complete when its `spec.md` says `done`, `completed`, or `implemented`, which is a document claim, not a verified run. `auto pipeline run` reports `Pipeline complete: N phases executed` with N including phases restored from a checkpoint. `auto init` tells every user without a worker config to connect Autopus Desktop first, although the ADK works on its own, and `auto status` with no SPEC sends every user to `/auto plan`, even for a small fix that needs no SPEC. The pipeline picks its platform from the first installed of `claude`, `codex`, `agy` without saying so or relating it to the project's configured platforms.

This SPEC makes status keep three kinds of evidence apart — the declared SPEC state, the last pipeline execution, and the last SPEC review — marks evidence stale when the code or SPEC it judged has changed, separates executed from restored phases, and names next commands that match what is installed and routed, each with where it runs, which provider it uses, and what it calls and writes. Nothing it recommends runs by itself.

## Background

- Pipeline runs persist `.autopus/pipeline-state/<SPEC>.yaml` (`pipeline_checkpoint.v2`) with the run receipt: run and task IDs, `resumed_from`, `prior_attempts`, `terminal_state`, `blocker`, failures with decisions such as `circuit_break_no_progress` and `budget_exhausted`, `resume.restored_phases`, `dispatch_count`, and the checkpoint's `git_commit_hash` and `snapshot_hash`. A refused resume writes `<SPEC>.blocked.yaml` instead. Pipeline run already writes per-SPEC sidecars there (`<SPEC>.execution-owner.json`, `<SPEC>.tier-integrity.json`).
- `auto pipeline run --continue` refuses a checkpoint whose git hash differs from HEAD.
- `auto spec review` writes `{SPEC_DIR}/review-receipt.json` (`spec_review_promotion_receipt.v1`: verdict, `gate_status`, `loop_status`, `finished_at`) with no code baseline; PR #252, not on this base, adds `{SPEC_DIR}/review-session.json` (`spec_review_session.v1`: state `open`, `converged`, `budget_exhausted`, `superseded`, attempts, calls).
- Codex invokes routes as `@auto <route>` with the local plugin or `$codex-auto-<route>`; Claude Code, Antigravity CLI, OpenCode, and Oh My Pi use `/auto <route>`. Small low-risk work without a SPEC goes through `/auto fix` or the router's `inline` triage.
- The Desktop runtime helper is resolved by `resolveRuntimeHelper` (`AUTOPUS_DESKTOP_RUNTIME_HELPER`, `PATH`, packaged locations); a Desktop connection writes `~/.config/autopus/worker.yaml`.

## Outcome Boundary

- Outcome Lock: `auto status` text and `--json` show, per SPEC, the declared status, the last pipeline execution (run and attempt, terminal and stop reason, blocker, executed, restored, failed, and pending phases, dispatches, resumable or not with the reason and command), the verification state of the last completed run (`fresh`, `stale` with reasons, `unknown`, `none`) against a recorded code baseline, and the last SPEC review (verdict, time, staleness, review session state or `unknown`), and never counts a declared-done SPEC as verified; `auto pipeline run` reports executed and restored phases and dispatches separately and names its platform and why; status and init name next commands that match the installed AI CLIs, configured platforms, and Desktop state, lead with standalone use, offer work without a SPEC beside `/auto plan`, and say for each command where it runs, which provider it uses, and whether it calls external services and writes files; no recommended command runs automatically.
- Mandatory requirements: REQ-SEV-SCOPE-01 through REQ-SEV-VERIFY-01 below.
- Explicit non-goals: a new dashboard or state store; changing pipeline platform selection itself; writing review receipts (owned by `auto spec review`); detecting Desktop installation in `auto init`; Windows-specific Desktop paths; probing provider authentication or making any model call from status.
- Completion evidence: the tests named in `acceptance.md`, `go test ./internal/cli/...`, the TUIUX goldens re-recorded with section 95 of the recapture log, and the source-lines gate on the changed Go files.

## Definitions

- Declared status: the normalized `spec.md` status. `done`, `completed`, and `implemented` are documented-done.
- Execution evidence: the receipt in `<state>/<SPEC>.yaml`, where `<state>` is `.autopus/pipeline-state` under the root that holds the SPEC's `.autopus/specs`.
- Executed phases: phases whose final status is `done` and that are not in `resume.restored_phases`. Restored phases: that list. Dispatches: `dispatch_count` (this run) and `cumulative_dispatch_count`.
- Stop reason: for a run that ended other than `completed`, the decision of its last failure (`circuit_break_no_progress`, `budget_exhausted`, `blocked`, `cancelled`), else the terminal state.
- Verification baseline: `<state>/<SPEC>.verification.json`, schema `pipeline_verification_baseline.v1`, written by a completed run: `spec_id`, `run_id`, `task_id`, `base_head` (the run's git hash), `snapshot_hash`, `tree_fingerprint`, `recorded_at`.
- Tree fingerprint: `sha256:` over every path that differs from `base_head` in the working tree (`git diff --name-only <base_head>`) or is untracked and not ignored, each with the SHA-256 of its content or `deleted`, sorted, excluding `.autopus/**` except SPEC documents (`spec.md`, `plan.md`, `acceptance.md`, `research.md`, `change.md` under `.autopus/specs/`). Committing the verified tree keeps it equal; any later content change alters it (probe B1).
- Next action: `{command, purpose, runs_in (terminal | agent_session), provider (when it dispatches models), external_calls, writes, auto_run: false}`.

## Requirements

### Scope

**REQ-SEV-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL build status evidence only from existing sources (SPEC documents, pipeline checkpoints and sidecars, review receipts and sessions, `autopus.yaml`, `PATH`, the Desktop runtime helper resolution, and the worker config's presence), SHALL add no state store beyond the verification baseline sidecar, SHALL make status write nothing and call no model or server, and SHALL never run a recommended command.

### Declared State and Execution Evidence

**REQ-SEV-DECL-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL report the declared status apart from evidence, label a documented-done SPEC without fresh verification `documented_only`, and count `verified` SPECs separately from documented-done ones in the text summary and `summary` JSON.

**REQ-SEV-EXEC-01**
Priority: Must
Type: Event-driven
WHEN a SPEC has execution evidence, THEN THE SYSTEM SHALL report its run ID, task ID, attempt number (prior attempts plus one), `resumed_from`, route, terminal state, stop reason, blocker, executed, restored, failed, and pending phase counts, dispatches and cumulative dispatches, `base_head`, and finish time.

**REQ-SEV-EXEC-02**
Priority: Must
Type: Event-driven
WHEN the last run ended other than `completed` or `dry_run`, THEN THE SYSTEM SHALL report whether `auto pipeline run <SPEC> --continue` would be admitted — no when HEAD differs from `base_head`, when the checkpoint fails its SPEC, version, route, snapshot, or dependency checks, or when git is unavailable — with the reason, and the command when it would.

**REQ-SEV-EXEC-03**
Priority: Must
Type: Unwanted
IF `<SPEC>.blocked.yaml` is newer than the canonical checkpoint, THEN THE SYSTEM SHALL report that the last resume attempt was refused, with its blocker.

### Verification Baseline

**REQ-SEV-BASE-01**
Priority: Must
Type: Event-driven
WHEN `auto pipeline run` ends `completed` in a git work tree, THEN THE SYSTEM SHALL write the verification baseline atomically with mode 0600, and SHALL report a write failure on stderr without failing the completed run.

**REQ-SEV-BASE-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL report verification `fresh` only when the tree fingerprint recomputed against `base_head` equals the recorded one and the SPEC snapshot is unchanged; `stale` with the reasons `working tree changed since verification` or `SPEC documents changed since verification`; `unknown` with the reason when git, `base_head`, or the baseline cannot be read; and `none` without a completed run; and SHALL never report a SPEC as verified on its declared status alone.

### Review Evidence

**REQ-SEV-REVIEW-01**
Priority: Must
Type: Event-driven
WHEN a SPEC directory holds `review-receipt.json`, THEN THE SYSTEM SHALL report its verdict, gate status, loop status, and finish time, and SHALL mark it stale when any SPEC document was modified after that time.

**REQ-SEV-REVIEW-02**
Priority: Must
Type: Unwanted
IF `review-session.json` is present but unparseable or of a schema other than `spec_review_session.v1`, or a review receipt cannot be parsed, THEN THE SYSTEM SHALL report that evidence as `unknown` with the reason and continue; a `spec_review_session.v1` file reports its state, attempt count, and call count.

### Pipeline Run Output

**REQ-SEV-RUN-01**
Priority: Must
Type: Event-driven
WHEN `auto pipeline run` completes, THEN THE SYSTEM SHALL print `Pipeline complete: <E> phases executed, <R> restored from checkpoint, <D> dispatches`, never counting a restored phase as executed.

**REQ-SEV-RUN-02**
Priority: Must
Type: Event-driven
WHEN `auto pipeline run` resolves its platform, THEN THE SYSTEM SHALL print on stderr `Pipeline platform: <platform> — <reason>`, the reason being `--platform`, `first installed of claude → codex → agy`, or `no AI CLI installed; falls back to claude`.

### Next Commands

**REQ-SEV-NEXT-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL list next actions computed from the evidence and install state — resume for resumable runs, implementation for approved SPECs, review for draft SPECs, and work without a SPEC beside planning — each naming its command in the syntax of the configured platforms, where it runs, the provider it dispatches with when it is a pipeline command, whether it calls external services, and what it writes, with `auto_run: false`.

**REQ-SEV-NEXT-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL report the installed AI CLIs (`claude`, `codex`, `agy`, `opencode`, `omp`), the configured platforms, and the platform `auto pipeline run` would select with its reason, noting when that platform is not configured for the project.

**REQ-SEV-NEXT-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL present the Autopus Desktop connection as optional: status reports whether the Desktop runtime helper is installed and a worker config exists, says the ADK works standalone, and offers the connect command only when the helper is installed; init leads with standalone use and lists the Desktop connection as optional.

**REQ-SEV-JSON-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL carry the same labels, counts, reasons, and next actions in `auto status --json` as in the text output.

### Verification

**REQ-SEV-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL prove every Must requirement with tests that use temporary repositories, fixture checkpoints and receipts, stubbed `PATH` and Desktop resolution, and a temporary home, and SHALL keep every changed Go file within 300 counted lines.

## Generated And Changed Surfaces

- `[NEW] internal/cli/status_evidence.go`, `[NEW] status_evidence_review.go`, `[NEW] status_evidence_render.go`: evidence readers and the text and JSON blocks.
- `[NEW] internal/cli/status_next.go`, `[NEW] status_next_env.go`: next actions, platform and Desktop facts.
- `[NEW] internal/cli/pipeline_run_baseline.go`: tree fingerprint and the baseline sidecar; `pipeline_run.go`, `pipeline_run_flags.go`: counts and the platform line.
- `internal/cli/status.go`, `status_json.go`, `init.go`: wiring and guidance; README rows; TUIUX goldens.

## Related SPECs

- SPEC-PIPELINE-RESUME-001: the receipt fields this SPEC reads.
- SPEC-TUIUX-001: the init goldens this SPEC re-records.

## Traceability Matrix

| Requirement | Plan task | Acceptance | Invariants |
|---|---|---|---|
| REQ-SEV-SCOPE-01 | T7 | S12 | INV-S06 |
| REQ-SEV-DECL-01 | T1, T3 | S1 | INV-S01 |
| REQ-SEV-EXEC-01, EXEC-03 | T1, T3 | S3, S6 | INV-S03 |
| REQ-SEV-EXEC-02 | T1 | S3 | INV-S04 |
| REQ-SEV-BASE-01, BASE-02 | T2 | S2 | INV-S01, INV-S02 |
| REQ-SEV-REVIEW-01, REVIEW-02 | T1 | S9 | INV-S02 |
| REQ-SEV-RUN-01, RUN-02 | T5 | S6, S11 | INV-S03 |
| REQ-SEV-NEXT-01 | T4 | S4, S7 | INV-S05, INV-S06 |
| REQ-SEV-NEXT-02 | T4 | S8 | INV-S05 |
| REQ-SEV-NEXT-03 | T4, T6 | S7, S10 | INV-S05 |
| REQ-SEV-JSON-01 | T3 | S5 | INV-S07 |
| REQ-SEV-VERIFY-01 | T7 | S12 | INV-S06 |
